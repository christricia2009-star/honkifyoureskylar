package trips

import (
	"testing"
	"time"
)

func TestTripStartsAndClosesAfterTwoMinutes(t *testing.T) {
	start := time.Date(2026, 10, 6, 18, 0, 0, 0, time.UTC)
	res := Step(nil, Sample{At: start, Gear: "P", SpeedMph: 0}, ParkDwell, "t1")
	if res.Started || res.Open != nil {
		t.Fatal("parked car should not start a trip")
	}

	res = Step(nil, Sample{At: start, Gear: "ShiftStateD", SpeedMph: 12, Lat: 34.14, Lng: -118.25, HasLoc: true}, ParkDwell, "t1")
	if !res.Started || res.Open == nil {
		t.Fatal("expected a trip to start")
	}
	open := res.Open

	res = Step(open, Sample{At: start.Add(time.Minute), Gear: "D", SpeedMph: 82, Lat: 34.15, Lng: -118.25, HasLoc: true}, ParkDwell, "")
	if res.Ended != nil || res.Open.MaxSpeedMph != 82 {
		t.Fatalf("max speed = %v ended=%v", res.Open.MaxSpeedMph, res.Ended)
	}
	open = res.Open
	if open.DistanceMiles < 0.5 {
		t.Fatalf("distance = %v", open.DistanceMiles)
	}

	res = Step(open, Sample{At: start.Add(2 * time.Minute), Gear: "P", SpeedMph: 0, Lat: 34.15, Lng: -118.25, HasLoc: true}, ParkDwell, "")
	if res.Ended != nil || res.Open.ParkSince == nil {
		t.Fatal("park should start the dwell, not close the trip")
	}
	open = res.Open

	res = Step(open, Sample{At: start.Add(3 * time.Minute), Gear: "D", SpeedMph: 5, Lat: 34.151, Lng: -118.25, HasLoc: true}, ParkDwell, "")
	if res.Open.ParkSince != nil {
		t.Fatal("leaving park should cancel the dwell")
	}
	open = res.Open

	parkAt := start.Add(4 * time.Minute)
	res = Step(open, Sample{At: parkAt, Gear: "Park", SpeedMph: 0}, ParkDwell, "")
	open = res.Open
	res = Step(open, Sample{At: parkAt.Add(2 * time.Minute), Gear: "P", SpeedMph: 0}, ParkDwell, "")
	if res.Ended == nil || res.Open != nil {
		t.Fatal("trip should close after 2 minutes in Park")
	}
	if res.Ended.MaxSpeedMph != 82 {
		t.Fatalf("closed max = %v", res.Ended.MaxSpeedMph)
	}
}

func TestOdometerDistanceWins(t *testing.T) {
	start := time.Date(2026, 10, 6, 18, 0, 0, 0, time.UTC)
	odo := 1000.0
	res := Step(nil, Sample{At: start, Gear: "D", SpeedMph: 10, Odometer: &odo, HasLoc: true, Lat: 1, Lng: 1}, ParkDwell, "t")
	odo2 := 1004.2
	res = Step(res.Open, Sample{At: start.Add(time.Minute), Gear: "D", SpeedMph: 10, Odometer: &odo2, HasLoc: true, Lat: 1.01, Lng: 1}, ParkDwell, "")
	park := start.Add(2 * time.Minute)
	res = Step(res.Open, Sample{At: park, Gear: "P", SpeedMph: 0, Odometer: &odo2}, ParkDwell, "")
	res = Step(res.Open, Sample{At: park.Add(ParkDwell), Gear: "P", SpeedMph: 0, Odometer: &odo2}, ParkDwell, "")
	if res.Ended == nil {
		t.Fatal("expected close")
	}
	if res.Ended.DistanceMiles < 4.1 || res.Ended.DistanceMiles > 4.3 {
		t.Fatalf("distance = %v", res.Ended.DistanceMiles)
	}
}
