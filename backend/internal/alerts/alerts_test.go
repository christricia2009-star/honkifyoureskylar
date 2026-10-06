package alerts

import (
	"testing"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/model"
)

func settings() model.Settings {
	s := model.DefaultSettings()
	s.HomeSet = true
	s.HomeLatitude = 34.1425
	s.HomeLongitude = -118.2551
	s.HomeRadiusMeters = 250
	s.Timezone = "UTC"
	return s
}

func TestSpeedAlertsOnTheCrossingOnly(t *testing.T) {
	s := settings()
	at := time.Date(2026, 10, 6, 18, 0, 0, 0, time.UTC)
	mem := Memory{}
	slow := 70.0
	got, mem := Evaluate(at, s, mem, &slow, nil, nil, "")
	if len(got) != 0 {
		t.Fatalf("unexpected %#v", got)
	}
	fast := 82.4
	got, mem = Evaluate(at, s, mem, &fast, nil, nil, "")
	if len(got) != 1 || got[0].Message != "Skylar hit 82. I'm not mad, I'm just a horn." {
		t.Fatalf("%#v", got)
	}
	faster := 90.0
	got, mem = Evaluate(at, s, mem, &faster, nil, nil, "")
	if len(got) != 0 {
		t.Fatal("should not repeat while still over")
	}
	got, mem = Evaluate(at, s, mem, &slow, nil, nil, "")
	got, _ = Evaluate(at, s, mem, &fast, nil, nil, "")
	if len(got) != 1 {
		t.Fatal("expected a new crossing")
	}
}

func TestLeaveHomeOncePerDeparture(t *testing.T) {
	s := settings()
	at := time.Date(2026, 10, 6, 18, 0, 0, 0, time.UTC)
	homeLat, homeLng := s.HomeLatitude, s.HomeLongitude
	mem := Memory{}
	got, mem := Evaluate(at, s, mem, nil, &homeLat, &homeLng, "")
	if len(got) != 0 || mem.InsideHome == nil || !*mem.InsideHome {
		t.Fatalf("should arm inside home, got %#v mem %#v", got, mem)
	}
	outLat := homeLat + 0.01
	got, mem = Evaluate(at, s, mem, nil, &outLat, &homeLng, "")
	if len(got) != 1 || got[0].Kind != LeftHome {
		t.Fatalf("%#v", got)
	}
	further := outLat + 0.01
	got, mem = Evaluate(at, s, mem, nil, &further, &homeLng, "")
	if len(got) != 0 {
		t.Fatal("already outside")
	}
	got, mem = Evaluate(at, s, mem, nil, &homeLat, &homeLng, "")
	got, _ = Evaluate(at, s, mem, nil, &outLat, &homeLng, "")
	if len(got) != 1 {
		t.Fatal("expected a second departure")
	}
}

func TestCurfewWindowWrapsMidnight(t *testing.T) {
	s := settings()
	day := time.Date(2026, 10, 6, 15, 0, 0, 0, time.UTC)
	got, _ := Evaluate(day, s, Memory{}, nil, nil, nil, "start")
	if len(got) != 0 {
		t.Fatal("afternoon is not curfew")
	}
	night := time.Date(2026, 10, 6, 23, 14, 0, 0, time.UTC)
	got, _ = Evaluate(night, s, Memory{}, nil, nil, nil, "start")
	if len(got) != 1 || got[0].Kind != CurfewOn {
		t.Fatalf("%#v", got)
	}
	late := time.Date(2026, 10, 7, 1, 2, 0, 0, time.UTC)
	got, _ = Evaluate(late, s, Memory{}, nil, nil, nil, "end")
	if len(got) != 1 || got[0].Kind != CurfewOff {
		t.Fatalf("%#v", got)
	}
	morning := time.Date(2026, 10, 7, 4, 0, 0, 0, time.UTC)
	got, _ = Evaluate(morning, s, Memory{}, nil, nil, nil, "start")
	if len(got) != 0 {
		t.Fatal("4:00am is outside the window")
	}
}
