package live

import (
	"testing"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/model"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/store"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/telemetry"
)

func TestDriveCreatesTripAndAlerts(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	settings := model.DefaultSettings()
	settings.Timezone = "UTC"
	if err := db.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	svc := &Service{DB: db}
	start := time.Date(2026, 10, 6, 23, 10, 0, 0, time.UTC)
	homeLat, homeLng := DemoHomeLat, DemoHomeLng
	gearD, gearP := "D", "P"
	speed := 10.0
	seat := true
	_, err = svc.Apply(true, telemetry.Update{
		At: start, VIN: "5YJTESTVIN0000001", Gear: &gearD, SpeedMph: &speed,
		Lat: &homeLat, Lng: &homeLng, Seat: &seat, SignalCount: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	fast := 82.0
	outLat := homeLat + 0.01
	got, err := svc.Apply(true, telemetry.Update{
		At: start.Add(time.Minute), VIN: "5YJTESTVIN0000001", Gear: &gearD, SpeedMph: &fast,
		Lat: &outLat, Lng: &homeLng, SignalCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 2 {
		t.Fatalf("expected speed and leave-home, got %#v", got)
	}
	park := start.Add(3 * time.Minute)
	zero := 0.0
	if _, err := svc.Apply(true, telemetry.Update{At: park, VIN: "5YJTESTVIN0000001", Gear: &gearP, SpeedMph: &zero, SignalCount: 2}); err != nil {
		t.Fatal(err)
	}
	got, err = svc.Apply(true, telemetry.Update{At: park.Add(2 * time.Minute), VIN: "5YJTESTVIN0000001", Gear: &gearP, SpeedMph: &zero, SignalCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	for _, a := range got {
		if a.Kind == "curfew_end" {
			closed = true
		}
	}
	if !closed {
		t.Fatalf("expected curfew end, got %#v", got)
	}
	trips, err := db.Trips("5YJTESTVIN0000001")
	if err != nil || len(trips) != 1 || trips[0].EndedAt == nil || !trips[0].OverLimit {
		t.Fatalf("trips %#v err %v", trips, err)
	}
}
