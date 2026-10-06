package store

import (
	"database/sql"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/model"
)

func (s *Store) SaveCarAlerts(vin string, alerts []model.CarAlert) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range alerts {
		if a.Name == "" || time.Time(a.At).IsZero() {
			continue
		}
		_, err := s.db.Exec(`INSERT INTO car_alerts (vin, name, at, audience, detail) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(vin, name, at) DO UPDATE SET audience = excluded.audience, detail = excluded.detail`,
			vin, a.Name, stamp(time.Time(a.At)), a.Audience, a.Detail)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CarAlerts(vin string) ([]model.CarAlert, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT name, at, audience, detail FROM car_alerts WHERE vin = ? ORDER BY at DESC LIMIT 80`, vin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.CarAlert{}
	for rows.Next() {
		var a model.CarAlert
		var at string
		if err := rows.Scan(&a.Name, &at, &a.Audience, &a.Detail); err != nil {
			return nil, err
		}
		when, _ := parseStamp(at)
		a.At = model.APITime(when)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Cached(key string, maxAge time.Duration) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var body, at string
	err := s.db.QueryRow(`SELECT body, fetched_at FROM live_cache WHERE key = ?`, key).Scan(&body, &at)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	when, err := parseStamp(at)
	if err != nil || time.Since(when) > maxAge {
		return body, false, nil
	}
	return body, true, nil
}

func (s *Store) SaveCache(key, body string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO live_cache (key, body, fetched_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET body = excluded.body, fetched_at = excluded.fetched_at`, key, body, stamp(at))
	return err
}
