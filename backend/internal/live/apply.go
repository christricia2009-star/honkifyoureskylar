package live

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/alerts"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/model"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/store"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/telemetry"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/trips"
)

const (
	DemoHomeLat = 34.1425
	DemoHomeLng = -118.2551
)

type Service struct {
	DB       *store.Store
	OnChange func(demo bool, alerts []model.Alert)
}

func (s *Service) Apply(demo bool, up telemetry.Update) ([]model.Alert, error) {
	if up.VIN == "" {
		return nil, fmt.Errorf("missing vin")
	}
	if up.At.IsZero() {
		up.At = time.Now()
	}
	settings, err := s.DB.EnsureSettings()
	if err != nil {
		return nil, err
	}
	if demo && !settings.HomeSet {
		settings.HomeLatitude = DemoHomeLat
		settings.HomeLongitude = DemoHomeLng
		settings.HomeRadiusMeters = 250
		settings.HomeSet = true
	}
	if err := s.ensureVehicle(demo, up); err != nil {
		return nil, err
	}
	sn, err := s.DB.Snapshot(up.VIN)
	if err == sql.ErrNoRows {
		sn = model.Snapshot{VIN: up.VIN, DoorSummary: "Unknown"}
		err = nil
	}
	if err != nil {
		return nil, err
	}
	merge(&sn, up)

	state := "online"
	if up.Online != nil && !*up.Online {
		state = "asleep"
	}
	seen := up.At
	if err := s.DB.TouchVehicle(up.VIN, state, seen); err != nil {
		return nil, err
	}

	current, err := s.DB.OpenTrip(up.VIN)
	if err != nil {
		return nil, err
	}
	prev := openFromModel(current)
	sample := sampleFrom(sn, up)
	res := trips.Step(prev, sample, trips.ParkDwell, store.NewID())
	limit := settings.SpeedLimitMph
	if limit <= 0 {
		limit = 75
	}
	if res.Started && res.Open != nil {
		res.Open.VIN = up.VIN
		res.Open.Demo = demo
	}
	if res.Open != nil {
		res.Open.VIN = up.VIN
		res.Open.Demo = demo
		if err := s.DB.SaveTrip(modelFromOpen(res.Open, limit, false)); err != nil {
			return nil, err
		}
	}
	if res.Ended != nil {
		res.Ended.VIN = up.VIN
		res.Ended.Demo = demo
		if err := s.DB.SaveTrip(modelFromOpen(res.Ended, limit, true)); err != nil {
			return nil, err
		}
	}

	event := ""
	if res.Started {
		event = "start"
	} else if res.Ended != nil {
		event = "end"
	}
	mem := alerts.Memory{SpeedOver: sn.SpeedOver, InsideHome: sn.InsideHome}
	drafts, mem := alerts.Evaluate(up.At, settings, mem, sn.SpeedMph, sn.Lat, sn.Lng, event)
	sn.SpeedOver = mem.SpeedOver
	sn.InsideHome = mem.InsideHome
	if err := s.DB.SaveSnapshot(sn); err != nil {
		return nil, err
	}
	if up.SignalCount > 0 {
		if err := s.DB.AddUsage(demo, "streaming_signal", float64(up.SignalCount)); err != nil {
			return nil, err
		}
	}
	var made []model.Alert
	for _, d := range drafts {
		a := model.Alert{
			ID:        store.NewID(),
			VIN:       up.VIN,
			Kind:      string(d.Kind),
			Message:   d.Message,
			CreatedAt: model.APITime(up.At),
		}
		if err := s.DB.InsertAlert(a, demo); err != nil {
			return nil, err
		}
		made = append(made, a)
	}
	if s.OnChange != nil {
		s.OnChange(demo, made)
	}
	return made, nil
}

func (s *Service) ensureVehicle(demo bool, up telemetry.Update) error {
	_, err := s.DB.Vehicle(up.VIN)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	seen := up.At
	name := up.VIN
	if demo {
		name = "Skylar's Getaway Car"
	}
	return s.DB.UpsertVehicle(model.Vehicle{VIN: up.VIN, Demo: demo, Name: name, State: "online", LastSeen: &seen})
}

func merge(sn *model.Snapshot, up telemetry.Update) {
	if up.SpeedMph != nil {
		sn.SpeedMph = up.SpeedMph
	}
	if up.Gear != nil {
		sn.Gear = trips.NormGear(*up.Gear)
	}
	if up.Lat != nil && up.Lng != nil {
		sn.Lat = up.Lat
		sn.Lng = up.Lng
	}
	if up.Soc != nil {
		sn.Soc = up.Soc
	}
	if up.RangeMi != nil {
		sn.RangeMi = up.RangeMi
	}
	if up.Charge != nil {
		sn.ChargeState = *up.Charge
	}
	if up.DoorsOpen != nil {
		sn.DoorsOpen = *up.DoorsOpen
		sn.HaveDoors = true
	}
	if up.DoorSummary != nil {
		sn.DoorSummary = *up.DoorSummary
	}
	if up.Locked != nil {
		sn.Locked = up.Locked
	}
	if up.Seat != nil {
		sn.Seat = up.Seat
	}
	if up.Guest != nil {
		sn.Guest = up.Guest
	}
	if up.Odometer != nil {
		sn.Odometer = up.Odometer
	}
	sn.UpdatedAt = up.At
	if sn.DoorSummary == "" {
		sn.DoorSummary = "Unknown"
	}
}

func sampleFrom(sn model.Snapshot, up telemetry.Update) trips.Sample {
	speed := 0.0
	if sn.SpeedMph != nil {
		speed = *sn.SpeedMph
	}
	sample := trips.Sample{
		At:       up.At,
		Gear:     sn.Gear,
		SpeedMph: speed,
		Odometer: sn.Odometer,
		Seat:     sn.Seat,
		Guest:    sn.Guest,
	}
	if sn.Lat != nil && sn.Lng != nil {
		sample.HasLoc = true
		sample.Lat = *sn.Lat
		sample.Lng = *sn.Lng
	}
	return sample
}

func openFromModel(tr *model.Trip) *trips.Open {
	if tr == nil {
		return nil
	}
	open := &trips.Open{
		ID:            tr.ID,
		VIN:           tr.VIN,
		Demo:          tr.Demo,
		StartedAt:     tr.StartedAt,
		MaxSpeedMph:   tr.MaxSpeedMph,
		DistanceMiles: tr.DistanceMiles,
		Polyline:      tr.Polyline,
		StartOdo:      tr.StartOdo,
		EndOdo:        tr.EndOdo,
		Seat:          tr.SeatOccupied,
		Guest:         tr.GuestMode,
		ParkSince:     tr.ParkSince,
	}
	if n := len(tr.Polyline); n > 0 {
		last := tr.Polyline[n-1]
		open.HasLast = true
		open.LastLat = last.Latitude
		open.LastLng = last.Longitude
	}
	return open
}

func modelFromOpen(open *trips.Open, limit float64, closed bool) model.Trip {
	tr := model.Trip{
		ID:            open.ID,
		VIN:           open.VIN,
		Demo:          open.Demo,
		StartedAt:     open.StartedAt,
		EndedAt:       open.EndedAt,
		MaxSpeedMph:   open.MaxSpeedMph,
		DistanceMiles: open.DistanceMiles,
		OverLimit:     open.MaxSpeedMph > limit,
		Polyline:      open.Polyline,
		StartOdo:      open.StartOdo,
		EndOdo:        open.EndOdo,
		SeatOccupied:  open.Seat,
		GuestMode:     open.Guest,
		ParkSince:     open.ParkSince,
		PendingClose:  !closed && open.ParkSince != nil,
		SpeedLimitMph: limit,
	}
	if tr.OverLimit {
		tr.Callout = fmtSpeed(open.MaxSpeedMph)
	}
	return tr
}

func fmtSpeed(mph float64) string {
	return fmtInt(mph)
}

func fmtInt(mph float64) string {
	return "Skylar hit " + itoa(int(mph+0.5)) + ". I'm not mad, I'm just a horn."
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
