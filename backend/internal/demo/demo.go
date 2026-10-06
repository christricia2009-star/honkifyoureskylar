package demo

import (
	"context"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/live"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/model"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/store"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/telemetry"
)

const VIN = "5YJ3SKYLARDEMO001"

func Seed(db *store.Store) error {
	list, err := db.Vehicles(true)
	if err != nil {
		return err
	}
	if len(list) > 0 {
		return nil
	}
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.Local
	}
	now := time.Now().In(loc)
	yesterday := now.Add(-24 * time.Hour)
	day := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, loc)
	seen := now.Add(-2 * time.Minute)
	if err := db.UpsertVehicle(model.Vehicle{
		VIN: VIN, Demo: true, Name: "Skylar's Getaway Car", State: "online", LastSeen: &seen,
	}); err != nil {
		return err
	}
	speed := 0.0
	soc := 71.0
	rng := 214.0
	lat, lng := live.DemoHomeLat, live.DemoHomeLng
	locked := true
	seat := false
	guest := false
	if err := db.SaveSnapshot(model.Snapshot{
		VIN: VIN, SpeedMph: &speed, Gear: "P", Lat: &lat, Lng: &lng, Soc: &soc, RangeMi: &rng,
		ChargeState: "Disconnected", DoorSummary: "All closed", Locked: &locked, Seat: &seat, Guest: &guest,
		UpdatedAt: seen, HaveDoors: true,
	}); err != nil {
		return err
	}
	if err := db.SaveDrivers(VIN, true, []model.Driver{
		{Name: "Skylar", Detail: "Allow-list sample, not the person in the seat.", Sample: true},
		{Name: "Chris", Detail: "Allow-list sample, not the person in the seat.", Sample: true},
	}, ""); err != nil {
		return err
	}
	afternoon := day.Add(16*time.Hour + 10*time.Minute)
	if err := db.SaveTrip(closedTrip("demo-afternoon", afternoon, afternoon.Add(28*time.Minute), 64, 6.4, false)); err != nil {
		return err
	}
	night := day.Add(23*time.Hour + 40*time.Minute)
	nightEnd := night.Add(22 * time.Minute)
	if err := db.SaveTrip(closedTrip("demo-curfew", night, nightEnd, 82, 11.2, true)); err != nil {
		return err
	}
	if err := db.InsertAlert(model.Alert{
		ID: "demo-speed", VIN: VIN, Kind: "speed", CreatedAt: model.APITime(night.Add(8 * time.Minute)),
		Message: "Skylar hit 82. I'm not mad, I'm just a horn.",
	}, true); err != nil {
		return err
	}
	return db.InsertAlert(model.Alert{
		ID: "demo-curfew", VIN: VIN, Kind: "curfew_start", CreatedAt: model.APITime(night),
		Message: "It's 11:40pm and Skylar just left Park. The horn would like a word.",
	}, true)
}

func closedTrip(id string, start, end time.Time, max, miles float64, over bool) model.Trip {
	callout := ""
	if over {
		callout = "Skylar hit 82. I'm not mad, I'm just a horn."
	}
	seat := true
	guest := false
	return model.Trip{
		ID: id, VIN: VIN, Demo: true, StartedAt: start, EndedAt: &end,
		MaxSpeedMph: max, DistanceMiles: miles, OverLimit: over, Callout: callout,
		SeatOccupied: &seat, GuestMode: &guest, SpeedLimitMph: 75,
		Polyline: []model.LatLng{
			{Latitude: live.DemoHomeLat, Longitude: live.DemoHomeLng, SpeedMph: 12, At: model.APITime(start)},
			{Latitude: live.DemoHomeLat + 0.01, Longitude: live.DemoHomeLng - 0.004, SpeedMph: max, At: model.APITime(start.Add(10 * time.Minute))},
			{Latitude: live.DemoHomeLat + 0.002, Longitude: live.DemoHomeLng, SpeedMph: 8, At: model.APITime(end)},
		},
	}
}

type frame struct {
	at    time.Duration
	speed float64
	gear  string
	lat   float64
	lng   float64
	seat  bool
	soc   float64
}

func script() []frame {
	hLat, hLng := live.DemoHomeLat, live.DemoHomeLng
	return []frame{
		{0, 0, "P", hLat, hLng, false, 71},
		{8 * time.Second, 12, "D", hLat + 0.0006, hLng, true, 71},
		{16 * time.Second, 34, "D", hLat + 0.0014, hLng, true, 70},
		{24 * time.Second, 58, "D", hLat + 0.0032, hLng - 0.001, true, 70},
		{32 * time.Second, 82, "D", hLat + 0.008, hLng - 0.002, true, 69},
		{44 * time.Second, 48, "D", hLat + 0.004, hLng - 0.001, true, 69},
		{52 * time.Second, 16, "D", hLat + 0.0008, hLng, true, 69},
		{60 * time.Second, 0, "P", hLat, hLng, false, 69},
	}
}

// Run plays a demo drive on a loop. Park stays Park for the real 2 minute dwell.
func Run(ctx context.Context, svc *live.Service) {
	frames := script()
	origin := time.Now()
	last := -1
	var lastKeep time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
		elapsed := time.Since(origin)
		if elapsed > 60*time.Second+2*time.Minute+4*time.Second {
			origin = time.Now()
			last = -1
			continue
		}
		idx := 0
		for i, f := range frames {
			if elapsed >= f.at {
				idx = i
			}
		}
		if idx == last && time.Since(lastKeep) < 15*time.Second {
			continue
		}
		last = idx
		lastKeep = time.Now()
		f := frames[idx]
		gear, speed, soc := f.gear, f.speed, f.soc
		seat := f.seat
		locked := true
		guest := false
		doors := false
		summary := "All closed"
		charge := "Disconnected"
		lat, lng := f.lat, f.lng
		_, _ = svc.Apply(true, telemetry.Update{
			At: time.Now(), VIN: VIN, SpeedMph: &speed, Gear: &gear, Lat: &lat, Lng: &lng,
			Soc: &soc, Seat: &seat, Guest: &guest, Locked: &locked, DoorsOpen: &doors,
			DoorSummary: &summary, Charge: &charge, SignalCount: 4,
		})
	}
}
