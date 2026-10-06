package store

import (
	"database/sql"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/ledger"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/model"
)

func (s *Store) Observe(demo bool, vin string, reading ledger.Reading) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	book, err := s.ledgerBook(vin)
	if err != nil {
		return err
	}
	_, out := ledger.Apply(book, reading, NewID())
	if prev := ledger.TakePreviousCharge(out.Charge); prev != nil {
		if err := s.saveCharge(demo, vin, *prev); err != nil {
			return err
		}
	}
	if out.Charge != nil {
		if err := s.saveCharge(demo, vin, *out.Charge); err != nil {
			return err
		}
	}
	if prev := ledger.TakePreviousDrain(out.Drain); prev != nil {
		if err := s.saveDrain(demo, vin, *prev); err != nil {
			return err
		}
	}
	if out.Drain != nil {
		if err := s.saveDrain(demo, vin, *out.Drain); err != nil {
			return err
		}
	}
	if out.Battery != nil {
		if err := s.saveBattery(vin, *out.Battery); err != nil {
			return err
		}
	}
	if out.Software != "" {
		if err := s.seeSoftware(vin, out.Software, reading.At); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ledgerBook(vin string) (ledger.Book, error) {
	var book ledger.Book
	charge, err := s.openCharge(vin)
	if err != nil {
		return book, err
	}
	book.Charge = charge
	drain, err := s.openDrain(vin)
	if err != nil {
		return book, err
	}
	book.Drain = drain
	closed, err := s.lastClosedCharge(vin)
	if err != nil {
		return book, err
	}
	book.LastClosed = closed
	var odo sql.NullFloat64
	var at string
	err = s.db.QueryRow(`SELECT odometer, at FROM battery_points WHERE vin = ? ORDER BY at DESC LIMIT 1`, vin).Scan(&odo, &at)
	if err == sql.ErrNoRows {
		err = nil
	}
	if err != nil {
		return book, err
	}
	if odo.Valid {
		v := odo.Float64
		book.LastOdo = &v
		book.LastBattery, _ = parseStamp(at)
	}
	_ = s.db.QueryRow(`SELECT version FROM software WHERE vin = ? ORDER BY last_seen DESC LIMIT 1`, vin).Scan(&book.LastSoftware)
	return book, nil
}

func (s *Store) saveCharge(demo bool, vin string, c ledger.Charge) error {
	var ended any
	if c.EndedAt != nil {
		ended = stamp(*c.EndedAt)
	}
	_, err := s.db.Exec(`INSERT INTO charges
		(id, vin, demo, started_at, ended_at, last_seen, soc_start, soc_end, energy_kwh, kind, open, one_reading)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		ended_at = excluded.ended_at, last_seen = excluded.last_seen, soc_start = excluded.soc_start,
		soc_end = excluded.soc_end, energy_kwh = excluded.energy_kwh, kind = excluded.kind,
		open = excluded.open, one_reading = excluded.one_reading`,
		c.ID, vin, boolInt(demo), stamp(c.StartedAt), ended, stamp(c.LastSeen), nullFloat(c.SocStart), nullFloat(c.SocEnd),
		nullFloat(c.EnergyKwh), c.Kind, boolInt(c.Open), boolInt(c.OneReading))
	return err
}

func (s *Store) saveDrain(demo bool, vin string, d ledger.Drain) error {
	var ended any
	if d.EndedAt != nil {
		ended = stamp(*d.EndedAt)
	}
	_, err := s.db.Exec(`INSERT INTO drains
		(id, vin, demo, started_at, ended_at, last_seen, soc_start, soc_end, range_start, range_end, open)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		ended_at = excluded.ended_at, last_seen = excluded.last_seen, soc_end = excluded.soc_end,
		range_end = excluded.range_end, open = excluded.open`,
		d.ID, vin, boolInt(demo), stamp(d.StartedAt), ended, stamp(d.LastSeen), nullFloat(d.SocStart), nullFloat(d.SocEnd),
		nullFloat(d.RangeStart), nullFloat(d.RangeEnd), boolInt(d.Open))
	return err
}

func (s *Store) saveBattery(vin string, p ledger.BatteryPoint) error {
	_, err := s.db.Exec(`INSERT INTO battery_points (vin, at, odometer, range_mi, soc) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(vin, at) DO UPDATE SET odometer = excluded.odometer, range_mi = excluded.range_mi, soc = excluded.soc`,
		vin, stamp(p.At), p.Odometer, nullFloat(p.RangeMi), nullFloat(p.Soc))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM battery_points WHERE vin = ? AND at NOT IN (
		SELECT at FROM battery_points WHERE vin = ? ORDER BY at DESC LIMIT 500)`, vin, vin)
	return err
}

func (s *Store) seeSoftware(vin, version string, at time.Time) error {
	if version == "" {
		return nil
	}
	_, err := s.db.Exec(`INSERT INTO software (vin, version, first_seen, last_seen, notes) VALUES (?, ?, ?, ?, '')
		ON CONFLICT(vin, version) DO UPDATE SET last_seen = excluded.last_seen`,
		vin, version, stamp(at), stamp(at))
	return err
}

func (s *Store) SaveSoftwareNotes(vin, version, notes string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if version == "" {
		version = "Unknown version"
	}
	_, err := s.db.Exec(`INSERT INTO software (vin, version, first_seen, last_seen, notes) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(vin, version) DO UPDATE SET last_seen = excluded.last_seen, notes = excluded.notes`,
		vin, version, stamp(at), stamp(at), notes)
	return err
}

func (s *Store) openCharge(vin string) (*ledger.Charge, error) {
	row := s.db.QueryRow(`SELECT id FROM charges WHERE vin = ? AND open = 1 ORDER BY started_at DESC LIMIT 1`, vin)
	var id string
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	c, err := s.charge(id)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) lastClosedCharge(vin string) (*ledger.Charge, error) {
	row := s.db.QueryRow(`SELECT id FROM charges WHERE vin = ? AND open = 0 ORDER BY last_seen DESC LIMIT 1`, vin)
	var id string
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	c, err := s.charge(id)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) charge(id string) (ledger.Charge, error) {
	var c ledger.Charge
	var started, seen string
	var ended sql.NullString
	var socStart, socEnd, energy sql.NullFloat64
	var open, one int
	err := s.db.QueryRow(`SELECT id, started_at, ended_at, last_seen, soc_start, soc_end, energy_kwh, kind, open, one_reading FROM charges WHERE id = ?`, id).
		Scan(&c.ID, &started, &ended, &seen, &socStart, &socEnd, &energy, &c.Kind, &open, &one)
	if err != nil {
		return c, err
	}
	c.StartedAt, _ = parseStamp(started)
	c.LastSeen, _ = parseStamp(seen)
	if ended.Valid {
		t, err := parseStamp(ended.String)
		if err == nil {
			c.EndedAt = &t
		}
	}
	c.SocStart = floatPtr(socStart)
	c.SocEnd = floatPtr(socEnd)
	c.EnergyKwh = floatPtr(energy)
	c.Open = open == 1
	c.OneReading = one == 1
	return c, nil
}

func (s *Store) openDrain(vin string) (*ledger.Drain, error) {
	row := s.db.QueryRow(`SELECT id FROM drains WHERE vin = ? AND open = 1 ORDER BY started_at DESC LIMIT 1`, vin)
	var id string
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	d, err := s.drain(id)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) drain(id string) (ledger.Drain, error) {
	var d ledger.Drain
	var started, seen string
	var ended sql.NullString
	var socStart, socEnd, rangeStart, rangeEnd sql.NullFloat64
	var open int
	err := s.db.QueryRow(`SELECT id, started_at, ended_at, last_seen, soc_start, soc_end, range_start, range_end, open FROM drains WHERE id = ?`, id).
		Scan(&d.ID, &started, &ended, &seen, &socStart, &socEnd, &rangeStart, &rangeEnd, &open)
	if err != nil {
		return d, err
	}
	d.StartedAt, _ = parseStamp(started)
	d.LastSeen, _ = parseStamp(seen)
	if ended.Valid {
		t, err := parseStamp(ended.String)
		if err == nil {
			d.EndedAt = &t
		}
	}
	d.SocStart = floatPtr(socStart)
	d.SocEnd = floatPtr(socEnd)
	d.RangeStart = floatPtr(rangeStart)
	d.RangeEnd = floatPtr(rangeEnd)
	d.Open = open == 1
	return d, nil
}

func (s *Store) Charges(vin string) ([]model.ChargeView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id FROM charges WHERE vin = ? ORDER BY started_at DESC LIMIT 100`, vin)
	if err != nil {
		return nil, err
	}
	ids, err := scanIDs(rows)
	if err != nil {
		return nil, err
	}
	out := []model.ChargeView{}
	for _, id := range ids {
		c, err := s.charge(id)
		if err != nil {
			return nil, err
		}
		out = append(out, chargeView(c))
	}
	return out, nil
}

func (s *Store) Drains(vin string) ([]model.DrainView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id FROM drains WHERE vin = ? ORDER BY started_at DESC LIMIT 100`, vin)
	if err != nil {
		return nil, err
	}
	ids, err := scanIDs(rows)
	if err != nil {
		return nil, err
	}
	out := []model.DrainView{}
	for _, id := range ids {
		d, err := s.drain(id)
		if err != nil {
			return nil, err
		}
		if !d.Open && !drainWorthShowing(d) {
			continue
		}
		out = append(out, drainView(d))
	}
	return out, nil
}

func drainWorthShowing(d ledger.Drain) bool {
	end := d.LastSeen
	if d.EndedAt != nil {
		end = *d.EndedAt
	}
	if end.Sub(d.StartedAt) >= 10*time.Minute {
		return true
	}
	if d.SocStart != nil && d.SocEnd != nil && *d.SocStart-*d.SocEnd >= 1 {
		return true
	}
	if d.RangeStart != nil && d.RangeEnd != nil && *d.RangeStart-*d.RangeEnd >= 1 {
		return true
	}
	return false
}

func (s *Store) Battery(vin string) ([]model.BatteryView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT at, odometer, range_mi, soc FROM battery_points WHERE vin = ? ORDER BY at DESC LIMIT 120`, vin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.BatteryView
	for rows.Next() {
		var at string
		var odo float64
		var rng, soc sql.NullFloat64
		if err := rows.Scan(&at, &odo, &rng, &soc); err != nil {
			return nil, err
		}
		when, _ := parseStamp(at)
		out = append(out, model.BatteryView{At: model.APITime(when), Odometer: odo, RangeMi: floatPtr(rng), Soc: floatPtr(soc)})
	}
	if out == nil {
		out = []model.BatteryView{}
	}
	return out, rows.Err()
}

func (s *Store) Software(vin string) ([]model.SoftwareView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT version, first_seen, last_seen, notes FROM software WHERE vin = ? ORDER BY last_seen DESC`, vin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.SoftwareView{}
	for rows.Next() {
		var version, first, last, notes string
		if err := rows.Scan(&version, &first, &last, &notes); err != nil {
			return nil, err
		}
		a, _ := parseStamp(first)
		b, _ := parseStamp(last)
		out = append(out, model.SoftwareView{Version: version, FirstSeen: model.APITime(a), LastSeen: model.APITime(b), Notes: notes})
	}
	return out, rows.Err()
}

func (s *Store) LatestSoftware(vin string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var version string
	err := s.db.QueryRow(`SELECT version FROM software WHERE vin = ? ORDER BY last_seen DESC LIMIT 1`, vin).Scan(&version)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return version, err
}

func chargeView(c ledger.Charge) model.ChargeView {
	end := c.LastSeen
	if c.EndedAt != nil {
		end = *c.EndedAt
	}
	view := model.ChargeView{
		ID: c.ID, StartedAt: model.APITime(c.StartedAt), SocStart: c.SocStart, SocEnd: c.SocEnd,
		EnergyKwh: c.EnergyKwh, Minutes: minutes(c.StartedAt, end), Kind: c.Kind, Open: c.Open, OneReading: c.OneReading,
	}
	if c.EndedAt != nil {
		t := model.APITime(*c.EndedAt)
		view.EndedAt = &t
	}
	return view
}

func drainView(d ledger.Drain) model.DrainView {
	end := d.LastSeen
	if d.EndedAt != nil {
		end = *d.EndedAt
	}
	view := model.DrainView{
		ID: d.ID, StartedAt: model.APITime(d.StartedAt), SocStart: d.SocStart, SocEnd: d.SocEnd,
		RangeStart: d.RangeStart, RangeEnd: d.RangeEnd, Minutes: minutes(d.StartedAt, end), Open: d.Open,
	}
	if d.EndedAt != nil {
		t := model.APITime(*d.EndedAt)
		view.EndedAt = &t
	}
	return view
}

func minutes(start, end time.Time) int {
	if end.Before(start) {
		return 0
	}
	return int(end.Sub(start).Round(time.Minute).Minutes())
}

func scanIDs(rows *sql.Rows) ([]string, error) {
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
