package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/billing"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/model"
	_ "modernc.org/sqlite"
)

const timeLayout = "2006-01-02T15:04:05.000000000Z"

type Session struct {
	ID     string
	Demo   bool
	Expiry time.Time
}

type Store struct {
	db *sql.DB
	mu sync.Mutex
}

func Open(path string) (*Store, error) {
	dsn := path
	if path == ":memory:" {
		dsn = "file:" + NewID() + "?mode=memory&cache=shared"
	} else {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	if _, err := s.db.Exec(`ALTER TABLE snapshots ADD COLUMN detail TEXT NOT NULL DEFAULT ''`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	if _, err := s.db.Exec(`ALTER TABLE vehicles ADD COLUMN access TEXT NOT NULL DEFAULT ''`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	for _, stmt := range []string{
		`ALTER TABLE trips ADD COLUMN start_soc REAL`,
		`ALTER TABLE trips ADD COLUMN end_soc REAL`,
		`ALTER TABLE trips ADD COLUMN start_rated REAL`,
		`ALTER TABLE trips ADD COLUMN end_rated REAL`,
		`ALTER TABLE trips ADD COLUMN range_kind TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE trips ADD COLUMN start_kwh REAL`,
		`ALTER TABLE trips ADD COLUMN end_kwh REAL`,
		`ALTER TABLE trips ADD COLUMN kwh_kind TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := s.db.Exec(stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	return nil
}

func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func stamp(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

func parseStamp(v string) (time.Time, error) {
	if v == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(timeLayout, v)
	if err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339Nano, v)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Store) EnsureSettings() (model.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureSettings()
}

func (s *Store) ensureSettings() (model.Settings, error) {
	var raw string
	err := s.db.QueryRow(`SELECT json FROM settings WHERE id = 1`).Scan(&raw)
	if err == sql.ErrNoRows {
		settings := model.DefaultSettings()
		b, _ := json.Marshal(settings)
		_, err = s.db.Exec(`INSERT INTO settings (id, json) VALUES (1, ?)`, string(b))
		return settings, err
	}
	if err != nil {
		return model.Settings{}, err
	}
	var settings model.Settings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return model.Settings{}, err
	}
	return settings, nil
}

func (s *Store) SaveSettings(settings model.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO settings (id, json) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET json = excluded.json`, string(b))
	return err
}

func (s *Store) CreateSession(demo bool, ttl time.Duration) (string, Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw := NewID() + NewID()
	sess := Session{ID: NewID(), Demo: demo, Expiry: time.Now().Add(ttl)}
	_, err := s.db.Exec(`INSERT INTO sessions (id, token_hash, demo, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		sess.ID, hashToken(raw), boolInt(demo), stamp(time.Now()), stamp(sess.Expiry))
	return raw, sess, err
}

func (s *Store) LookupSession(raw string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var sess Session
	var demo int
	var exp string
	err := s.db.QueryRow(`SELECT id, demo, expires_at FROM sessions WHERE token_hash = ?`, hashToken(raw)).Scan(&sess.ID, &demo, &exp)
	if err != nil {
		return Session{}, err
	}
	sess.Demo = demo == 1
	sess.Expiry, err = parseStamp(exp)
	if err != nil {
		return Session{}, err
	}
	if time.Now().After(sess.Expiry) {
		return Session{}, sql.ErrNoRows
	}
	return sess, nil
}

func (s *Store) SaveHandoff(sessionID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	code := NewID()
	_, err := s.db.Exec(`INSERT INTO handoffs (code, session_id, expires_at) VALUES (?, ?, ?)`,
		code, sessionID, stamp(time.Now().Add(2*time.Minute)))
	return code, err
}

func (s *Store) ConsumeHandoff(code string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var sessionID, exp string
	err := s.db.QueryRow(`SELECT session_id, expires_at FROM handoffs WHERE code = ?`, code).Scan(&sessionID, &exp)
	if err != nil {
		return Session{}, err
	}
	_, _ = s.db.Exec(`DELETE FROM handoffs WHERE code = ?`, code)
	when, err := parseStamp(exp)
	if err != nil || time.Now().After(when) {
		return Session{}, sql.ErrNoRows
	}
	var sess Session
	var demo int
	var expires string
	err = s.db.QueryRow(`SELECT id, demo, expires_at FROM sessions WHERE id = ?`, sessionID).Scan(&sess.ID, &demo, &expires)
	if err != nil {
		return Session{}, err
	}
	sess.Demo = demo == 1
	sess.Expiry, _ = parseStamp(expires)
	return sess, nil
}

func (s *Store) IssueTokenForSession(sess Session) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw := NewID() + NewID()
	_, err := s.db.Exec(`UPDATE sessions SET token_hash = ? WHERE id = ?`, hashToken(raw), sess.ID)
	return raw, err
}

func (s *Store) SaveOAuthState(state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO oauth_states (state, created_at) VALUES (?, ?)`, state, stamp(time.Now()))
	return err
}

func (s *Store) ConsumeOAuthState(state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var created string
	err := s.db.QueryRow(`SELECT created_at FROM oauth_states WHERE state = ?`, state).Scan(&created)
	if err != nil {
		return err
	}
	_, _ = s.db.Exec(`DELETE FROM oauth_states WHERE state = ?`, state)
	when, err := parseStamp(created)
	if err != nil || time.Since(when) > 15*time.Minute {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SaveTeslaAuth(auth model.TeslaAuth) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO tesla_auth (id, access_token, refresh_token, expiry, scopes, fleet_base)
		VALUES (1, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET access_token = excluded.access_token, refresh_token = excluded.refresh_token,
		expiry = excluded.expiry, scopes = excluded.scopes, fleet_base = excluded.fleet_base`,
		auth.AccessToken, auth.RefreshToken, stamp(auth.Expiry), auth.Scopes, auth.FleetBase)
	return err
}

func (s *Store) TeslaAuth() (model.TeslaAuth, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var auth model.TeslaAuth
	var exp string
	err := s.db.QueryRow(`SELECT access_token, refresh_token, expiry, scopes, fleet_base FROM tesla_auth WHERE id = 1`).
		Scan(&auth.AccessToken, &auth.RefreshToken, &exp, &auth.Scopes, &auth.FleetBase)
	if err != nil {
		return model.TeslaAuth{}, err
	}
	auth.Expiry, err = parseStamp(exp)
	return auth, err
}

func (s *Store) Linked() bool {
	_, err := s.TeslaAuth()
	return err == nil
}

func (s *Store) UpsertVehicle(v model.Vehicle) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var seen any
	if v.LastSeen != nil {
		seen = stamp(*v.LastSeen)
	}
	_, err := s.db.Exec(`INSERT INTO vehicles (vin, demo, name, state, last_seen, access) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(vin) DO UPDATE SET name = excluded.name, state = excluded.state, last_seen = excluded.last_seen, demo = excluded.demo,
		access = CASE WHEN excluded.access = '' THEN vehicles.access ELSE excluded.access END`,
		v.VIN, boolInt(v.Demo), v.Name, v.State, seen, v.Access)
	return err
}

func (s *Store) Vehicle(vin string) (model.Vehicle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.vehicle(vin)
}

func (s *Store) vehicle(vin string) (model.Vehicle, error) {
	var v model.Vehicle
	var demo int
	var seen sql.NullString
	err := s.db.QueryRow(`SELECT vin, demo, name, state, last_seen, access FROM vehicles WHERE vin = ?`, vin).
		Scan(&v.VIN, &demo, &v.Name, &v.State, &seen, &v.Access)
	if err != nil {
		return model.Vehicle{}, err
	}
	v.Demo = demo == 1
	if seen.Valid {
		t, err := parseStamp(seen.String)
		if err == nil {
			v.LastSeen = &t
		}
	}
	return v, nil
}

func (s *Store) Vehicles(demo bool) ([]model.Vehicle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT vin FROM vehicles WHERE demo = ? ORDER BY name`, boolInt(demo))
	if err != nil {
		return nil, err
	}
	var vins []string
	for rows.Next() {
		var vin string
		if err := rows.Scan(&vin); err != nil {
			rows.Close()
			return nil, err
		}
		vins = append(vins, vin)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var out []model.Vehicle
	for _, vin := range vins {
		v, err := s.vehicle(vin)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Store) SaveSnapshot(sn model.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveSnapshot(sn)
}

func (s *Store) saveSnapshot(sn model.Snapshot) error {
	_, err := s.db.Exec(`INSERT INTO snapshots
		(vin, speed_mph, gear, lat, lng, soc, range_mi, charge_state, doors_open, door_summary, locked, seat, guest, odometer, updated_at, inside_home, speed_over, have_doors, detail)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(vin) DO UPDATE SET
		speed_mph = excluded.speed_mph, gear = excluded.gear, lat = excluded.lat, lng = excluded.lng,
		soc = excluded.soc, range_mi = excluded.range_mi, charge_state = excluded.charge_state,
		doors_open = excluded.doors_open, door_summary = excluded.door_summary, locked = excluded.locked,
		seat = excluded.seat, guest = excluded.guest, odometer = excluded.odometer, updated_at = excluded.updated_at,
		inside_home = excluded.inside_home, speed_over = excluded.speed_over, have_doors = excluded.have_doors,
		detail = excluded.detail`,
		sn.VIN, nullFloat(sn.SpeedMph), sn.Gear, nullFloat(sn.Lat), nullFloat(sn.Lng), nullFloat(sn.Soc), nullFloat(sn.RangeMi),
		sn.ChargeState, boolInt(sn.DoorsOpen), sn.DoorSummary, nullBool(sn.Locked), nullBool(sn.Seat), nullBool(sn.Guest),
		nullFloat(sn.Odometer), stamp(sn.UpdatedAt), nullBool(sn.InsideHome), boolInt(sn.SpeedOver), boolInt(sn.HaveDoors), sn.Detail)
	return err
}

func (s *Store) Snapshot(vin string) (model.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot(vin)
}

func (s *Store) snapshot(vin string) (model.Snapshot, error) {
	var sn model.Snapshot
	var speed, lat, lng, soc, rangeMi, odo sql.NullFloat64
	var locked, seat, guest, inside sql.NullInt64
	var doors, speedOver, haveDoors int
	var updated string
	err := s.db.QueryRow(`SELECT vin, speed_mph, gear, lat, lng, soc, range_mi, charge_state, doors_open, door_summary, locked, seat, guest, odometer, updated_at, inside_home, speed_over, have_doors, detail
		FROM snapshots WHERE vin = ?`, vin).Scan(
		&sn.VIN, &speed, &sn.Gear, &lat, &lng, &soc, &rangeMi, &sn.ChargeState, &doors, &sn.DoorSummary,
		&locked, &seat, &guest, &odo, &updated, &inside, &speedOver, &haveDoors, &sn.Detail)
	if err != nil {
		return model.Snapshot{}, err
	}
	sn.SpeedMph = floatPtr(speed)
	sn.Lat = floatPtr(lat)
	sn.Lng = floatPtr(lng)
	sn.Soc = floatPtr(soc)
	sn.RangeMi = floatPtr(rangeMi)
	sn.Odometer = floatPtr(odo)
	sn.Locked = boolPtr(locked)
	sn.Seat = boolPtr(seat)
	sn.Guest = boolPtr(guest)
	sn.InsideHome = boolPtr(inside)
	sn.DoorsOpen = doors == 1
	sn.SpeedOver = speedOver == 1
	sn.HaveDoors = haveDoors == 1
	sn.UpdatedAt, _ = parseStamp(updated)
	return sn, nil
}

func (s *Store) InsertGrab(vin string, demo bool, state, summary, facts string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if facts == "" {
		facts = "[]"
	}
	_, err := s.db.Exec(`INSERT INTO grabs (id, vin, demo, taken_at, state, summary, facts) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		NewID(), vin, boolInt(demo), stamp(at), state, summary, facts)
	return err
}

func (s *Store) Grabs(vin string, limit int) ([]model.Grab, error) {
	if limit <= 0 {
		limit = 20
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, vin, taken_at, state, summary, facts FROM grabs WHERE vin = ? ORDER BY taken_at DESC LIMIT ?`, vin, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Grab
	for rows.Next() {
		var g model.Grab
		var taken, facts string
		if err := rows.Scan(&g.ID, &g.VIN, &taken, &g.State, &g.Summary, &facts); err != nil {
			return nil, err
		}
		when, _ := parseStamp(taken)
		g.TakenAt = model.APITime(when)
		_ = json.Unmarshal([]byte(facts), &g.Facts)
		if g.Facts == nil {
			g.Facts = []model.Fact{}
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) SaveTrip(tr model.Trip) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveTrip(tr)
}

func (s *Store) saveTrip(tr model.Trip) error {
	poly, err := json.Marshal(tr.Polyline)
	if err != nil {
		return err
	}
	var ended, park any
	if tr.EndedAt != nil {
		ended = stamp(*tr.EndedAt)
	}
	if tr.ParkSince != nil {
		park = stamp(*tr.ParkSince)
	}
	_, err = s.db.Exec(`INSERT INTO trips
		(id, vin, demo, started_at, ended_at, max_speed_mph, distance_mi, over_limit, polyline, start_odo, end_odo, seat, guest, park_since, pending_close, callout,
		 start_soc, end_soc, start_rated, end_rated, range_kind, start_kwh, end_kwh, kwh_kind)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		ended_at = excluded.ended_at, max_speed_mph = excluded.max_speed_mph, distance_mi = excluded.distance_mi,
		over_limit = excluded.over_limit, polyline = excluded.polyline, start_odo = excluded.start_odo,
		end_odo = excluded.end_odo, seat = excluded.seat, guest = excluded.guest, park_since = excluded.park_since,
		pending_close = excluded.pending_close, callout = excluded.callout,
		start_soc = excluded.start_soc, end_soc = excluded.end_soc, start_rated = excluded.start_rated,
		end_rated = excluded.end_rated, range_kind = excluded.range_kind, start_kwh = excluded.start_kwh,
		end_kwh = excluded.end_kwh, kwh_kind = excluded.kwh_kind`,
		tr.ID, tr.VIN, boolInt(tr.Demo), stamp(tr.StartedAt), ended, tr.MaxSpeedMph, tr.DistanceMiles, boolInt(tr.OverLimit),
		string(poly), nullFloat(tr.StartOdo), nullFloat(tr.EndOdo), nullBool(tr.SeatOccupied), nullBool(tr.GuestMode),
		park, boolInt(tr.PendingClose), tr.Callout,
		nullFloat(tr.StartSoc), nullFloat(tr.EndSoc), nullFloat(tr.StartRated), nullFloat(tr.EndRated), tr.RangeKind,
		nullFloat(tr.StartKwh), nullFloat(tr.EndKwh), tr.KwhKind)
	return err
}

func (s *Store) OpenTrip(vin string) (*model.Trip, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.db.QueryRow(`SELECT id FROM trips WHERE vin = ? AND ended_at IS NULL ORDER BY started_at DESC LIMIT 1`, vin)
	var id string
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	tr, err := s.trip(id)
	if err != nil {
		return nil, err
	}
	return &tr, nil
}

func (s *Store) Trip(id string) (model.Trip, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.trip(id)
}

func (s *Store) trip(id string) (model.Trip, error) {
	var tr model.Trip
	var demo, over, pending int
	var started string
	var ended, park sql.NullString
	var poly string
	var startOdo, endOdo sql.NullFloat64
	var startSoc, endSoc, startRated, endRated, startKwh, endKwh sql.NullFloat64
	var seat, guest sql.NullInt64
	err := s.db.QueryRow(`SELECT id, vin, demo, started_at, ended_at, max_speed_mph, distance_mi, over_limit, polyline, start_odo, end_odo, seat, guest, park_since, pending_close, callout,
		start_soc, end_soc, start_rated, end_rated, range_kind, start_kwh, end_kwh, kwh_kind
		FROM trips WHERE id = ?`, id).Scan(&tr.ID, &tr.VIN, &demo, &started, &ended, &tr.MaxSpeedMph, &tr.DistanceMiles, &over, &poly, &startOdo, &endOdo, &seat, &guest, &park, &pending, &tr.Callout,
		&startSoc, &endSoc, &startRated, &endRated, &tr.RangeKind, &startKwh, &endKwh, &tr.KwhKind)
	if err != nil {
		return model.Trip{}, err
	}
	tr.Demo = demo == 1
	tr.OverLimit = over == 1
	tr.PendingClose = pending == 1
	tr.StartedAt, _ = parseStamp(started)
	if ended.Valid {
		t, err := parseStamp(ended.String)
		if err == nil {
			tr.EndedAt = &t
		}
	}
	if park.Valid {
		t, err := parseStamp(park.String)
		if err == nil {
			tr.ParkSince = &t
		}
	}
	tr.StartOdo = floatPtr(startOdo)
	tr.EndOdo = floatPtr(endOdo)
	tr.StartSoc = floatPtr(startSoc)
	tr.EndSoc = floatPtr(endSoc)
	tr.StartRated = floatPtr(startRated)
	tr.EndRated = floatPtr(endRated)
	tr.StartKwh = floatPtr(startKwh)
	tr.EndKwh = floatPtr(endKwh)
	tr.SeatOccupied = boolPtr(seat)
	tr.GuestMode = boolPtr(guest)
	if poly != "" {
		_ = json.Unmarshal([]byte(poly), &tr.Polyline)
	}
	return tr, nil
}

func (s *Store) Trips(vin string) ([]model.Trip, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id FROM trips WHERE vin = ? ORDER BY started_at DESC`, vin)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var out []model.Trip
	for _, id := range ids {
		tr, err := s.trip(id)
		if err != nil {
			return nil, err
		}
		out = append(out, tr)
	}
	return out, nil
}

func (s *Store) InsertAlert(a model.Alert, demo bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO alerts (id, vin, demo, kind, message, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		a.ID, a.VIN, boolInt(demo), a.Kind, a.Message, stamp(a.CreatedAt.Time()))
	return err
}

func (s *Store) Alerts(demo bool, limit int) ([]model.Alert, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, vin, kind, message, created_at, read_at FROM alerts WHERE demo = ? ORDER BY created_at DESC LIMIT ?`, boolInt(demo), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Alert
	for rows.Next() {
		var a model.Alert
		var created string
		var read sql.NullString
		if err := rows.Scan(&a.ID, &a.VIN, &a.Kind, &a.Message, &created, &read); err != nil {
			return nil, err
		}
		t, _ := parseStamp(created)
		a.CreatedAt = model.APITime(t)
		a.Read = read.Valid
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) MarkAlertRead(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE alerts SET read_at = ? WHERE id = ? AND read_at IS NULL`, stamp(time.Now()), id)
	return err
}

func (s *Store) SaveDrivers(vin string, demo bool, drivers []model.Driver, problem string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.Marshal(drivers)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO drivers (vin, demo, payload, error, fetched_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(vin) DO UPDATE SET payload = excluded.payload, error = excluded.error, fetched_at = excluded.fetched_at, demo = excluded.demo`,
		vin, boolInt(demo), string(b), problem, stamp(time.Now()))
	return err
}

func (s *Store) Drivers(vin string) ([]model.Driver, string, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var payload, problem, fetched string
	err := s.db.QueryRow(`SELECT payload, error, fetched_at FROM drivers WHERE vin = ?`, vin).Scan(&payload, &problem, &fetched)
	if err == sql.ErrNoRows {
		return nil, "", time.Time{}, nil
	}
	if err != nil {
		return nil, "", time.Time{}, err
	}
	var drivers []model.Driver
	if payload != "" {
		if err := json.Unmarshal([]byte(payload), &drivers); err != nil {
			return nil, "", time.Time{}, err
		}
	}
	when, _ := parseStamp(fetched)
	return drivers, problem, when, nil
}

func (s *Store) AddUsage(demo bool, kind string, units float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO usage_events (demo, kind, units, usd, created_at) VALUES (?, ?, ?, ?, ?)`,
		boolInt(demo), kind, units, billing.Cost(kind, units), stamp(time.Now()))
	return err
}

func (s *Store) Usage(demo bool, now time.Time) (model.Usage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	start := time.Date(now.UTC().Year(), now.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	rows, err := s.db.Query(`SELECT kind, COALESCE(SUM(units), 0), COALESCE(SUM(usd), 0) FROM usage_events
		WHERE demo = ? AND created_at >= ? AND created_at < ? GROUP BY kind`, boolInt(demo), stamp(start), stamp(end))
	if err != nil {
		return model.Usage{}, err
	}
	defer rows.Close()
	u := model.Usage{
		Month:            start.Format("2006-01"),
		Demo:             demo,
		MonthlyCreditUSD: billing.MonthlyCreditUSD,
		Note:             billing.Note,
	}
	if demo {
		u.Note = "Demo mode. Tesla has not been billed. The numbers are what the same pattern would cost on a live car. " + billing.Note
	}
	for rows.Next() {
		var kind string
		var units, usd float64
		if err := rows.Scan(&kind, &units, &usd); err != nil {
			return model.Usage{}, err
		}
		u.EstimatedUSD += usd
		switch kind {
		case "streaming_signal":
			u.StreamingSignals = int(units + 0.5)
		case "vehicle_data":
			u.VehicleDataCalls = int(units + 0.5)
		case "wake":
			u.Wakes = int(units + 0.5)
		case "command":
			u.Commands = int(units + 0.5)
		case "metadata":
			u.MetadataCalls = int(units + 0.5)
		}
	}
	u.EstimatedUSD = billing.RoundCents(u.EstimatedUSD)
	remain := billing.MonthlyCreditUSD - u.EstimatedUSD
	if remain < 0 {
		remain = 0
	}
	u.RemainingCreditUSD = billing.RoundCents(remain)
	return u, rows.Err()
}

func (s *Store) SaveDeviceToken(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO device_tokens (token, updated_at) VALUES (?, ?)
		ON CONFLICT(token) DO UPDATE SET updated_at = excluded.updated_at`, token, stamp(time.Now()))
	return err
}

func (s *Store) DeviceTokens() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT token FROM device_tokens`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) Ack(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO acks (id, at) VALUES (?, ?) ON CONFLICT(id) DO UPDATE SET at = excluded.at`, id, stamp(time.Now()))
	return err
}

func (s *Store) Acked(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	var at string
	err := s.db.QueryRow(`SELECT at FROM acks WHERE id = ?`, id).Scan(&at)
	return err == nil && at != ""
}

func (s *Store) SetMeta(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) Meta(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullBool(v *bool) any {
	if v == nil {
		return nil
	}
	if *v {
		return 1
	}
	return 0
}

func floatPtr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	n := v.Float64
	return &n
}

func boolPtr(v sql.NullInt64) *bool {
	if !v.Valid {
		return nil
	}
	b := v.Int64 != 0
	return &b
}

func (s *Store) TouchVehicle(vin, state string, seen time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE vehicles SET state = ?, last_seen = ? WHERE vin = ?`, state, stamp(seen), vin)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("unknown vehicle %s", vin)
	}
	return nil
}

const schema = `
CREATE TABLE IF NOT EXISTS meta (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	token_hash TEXT NOT NULL UNIQUE,
	demo INTEGER NOT NULL,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS oauth_states (
	state TEXT PRIMARY KEY,
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS handoffs (
	code TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	expires_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS tesla_auth (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	access_token TEXT NOT NULL,
	refresh_token TEXT NOT NULL,
	expiry TEXT NOT NULL,
	scopes TEXT NOT NULL,
	fleet_base TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS vehicles (
	vin TEXT PRIMARY KEY,
	demo INTEGER NOT NULL,
	name TEXT NOT NULL,
	state TEXT NOT NULL,
	last_seen TEXT,
	access TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS snapshots (
	vin TEXT PRIMARY KEY,
	speed_mph REAL,
	gear TEXT,
	lat REAL,
	lng REAL,
	soc REAL,
	range_mi REAL,
	charge_state TEXT,
	doors_open INTEGER,
	door_summary TEXT,
	locked INTEGER,
	seat INTEGER,
	guest INTEGER,
	odometer REAL,
	updated_at TEXT,
	inside_home INTEGER,
	speed_over INTEGER,
	have_doors INTEGER,
	detail TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS grabs (
	id TEXT PRIMARY KEY,
	vin TEXT NOT NULL,
	demo INTEGER NOT NULL,
	taken_at TEXT NOT NULL,
	state TEXT NOT NULL,
	summary TEXT NOT NULL,
	facts TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS trips (
	id TEXT PRIMARY KEY,
	vin TEXT NOT NULL,
	demo INTEGER NOT NULL,
	started_at TEXT NOT NULL,
	ended_at TEXT,
	max_speed_mph REAL NOT NULL,
	distance_mi REAL NOT NULL,
	over_limit INTEGER NOT NULL,
	polyline TEXT NOT NULL,
	start_odo REAL,
	end_odo REAL,
	seat INTEGER,
	guest INTEGER,
	park_since TEXT,
	pending_close INTEGER NOT NULL DEFAULT 0,
	callout TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS alerts (
	id TEXT PRIMARY KEY,
	vin TEXT NOT NULL,
	demo INTEGER NOT NULL,
	kind TEXT NOT NULL,
	message TEXT NOT NULL,
	created_at TEXT NOT NULL,
	read_at TEXT
);
CREATE TABLE IF NOT EXISTS drivers (
	vin TEXT PRIMARY KEY,
	demo INTEGER NOT NULL,
	payload TEXT NOT NULL,
	error TEXT NOT NULL DEFAULT '',
	fetched_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS usage_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	demo INTEGER NOT NULL,
	kind TEXT NOT NULL,
	units REAL NOT NULL,
	usd REAL NOT NULL,
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS device_tokens (
	token TEXT PRIMARY KEY,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS acks (
	id TEXT PRIMARY KEY,
	at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS charges (
	id TEXT PRIMARY KEY,
	vin TEXT NOT NULL,
	demo INTEGER NOT NULL,
	started_at TEXT NOT NULL,
	ended_at TEXT,
	last_seen TEXT NOT NULL,
	soc_start REAL,
	soc_end REAL,
	energy_kwh REAL,
	kind TEXT NOT NULL DEFAULT '',
	open INTEGER NOT NULL,
	one_reading INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS drains (
	id TEXT PRIMARY KEY,
	vin TEXT NOT NULL,
	demo INTEGER NOT NULL,
	started_at TEXT NOT NULL,
	ended_at TEXT,
	last_seen TEXT NOT NULL,
	soc_start REAL,
	soc_end REAL,
	range_start REAL,
	range_end REAL,
	open INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS battery_points (
	vin TEXT NOT NULL,
	at TEXT NOT NULL,
	odometer REAL NOT NULL,
	range_mi REAL,
	soc REAL,
	PRIMARY KEY (vin, at)
);
CREATE TABLE IF NOT EXISTS software (
	vin TEXT NOT NULL,
	version TEXT NOT NULL,
	first_seen TEXT NOT NULL,
	last_seen TEXT NOT NULL,
	notes TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (vin, version)
);
CREATE TABLE IF NOT EXISTS car_alerts (
	vin TEXT NOT NULL,
	name TEXT NOT NULL,
	at TEXT NOT NULL,
	audience TEXT NOT NULL DEFAULT '',
	detail TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (vin, name, at)
);
CREATE TABLE IF NOT EXISTS live_cache (
	key TEXT PRIMARY KEY,
	body TEXT NOT NULL,
	fetched_at TEXT NOT NULL
);
`
