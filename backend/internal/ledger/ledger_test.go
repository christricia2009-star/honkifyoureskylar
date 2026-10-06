package ledger

import (
	"testing"
	"time"
)

func f(v float64) *float64 { return &v }
func b(v bool) *bool       { return &v }

func TestChargeSessionOpensAndCloses(t *testing.T) {
	start := time.Date(2026, 10, 6, 18, 0, 0, 0, time.UTC)
	book, out := Apply(Book{}, Reading{
		At: start, Gear: "P", Charge: "Charging", Soc: f(20), EnergyAddedKwh: f(1), Fast: b(false),
	}, "c1")
	if out.Charge == nil || !out.Charge.Open || out.Charge.Kind != "AC" || out.Charge.SocStart == nil || *out.Charge.SocStart != 20 {
		t.Fatalf("open %#v", out.Charge)
	}
	_, out = Apply(book, Reading{
		At: start.Add(40 * time.Minute), Gear: "P", Charge: "Complete", Soc: f(80), EnergyAddedKwh: f(18.4),
	}, "c2")
	if out.Charge == nil || out.Charge.Open || out.Charge.EnergyKwh == nil || *out.Charge.EnergyKwh != 18.4 {
		t.Fatalf("close %#v", out.Charge)
	}
	if out.Charge.EndedAt == nil || out.Charge.EndedAt.Sub(start) != 40*time.Minute {
		t.Fatalf("duration %#v", out.Charge.EndedAt)
	}
}

func TestSuperchargerKindAndOneReading(t *testing.T) {
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	_, out := Apply(Book{}, Reading{
		At: at, Charge: "Complete", EnergyAddedKwh: f(42), FastType: "FastChargerSupercharger", Soc: f(90),
	}, "c1")
	if out.Charge == nil || !out.Charge.OneReading || out.Charge.Kind != "Supercharger" {
		t.Fatalf("%#v", out.Charge)
	}
	_, again := Apply(Book{LastClosed: out.Charge}, Reading{
		At: at.Add(time.Minute), Charge: "Complete", EnergyAddedKwh: f(42), FastType: "FastChargerSupercharger",
	}, "c2")
	if again.Charge != nil {
		t.Fatalf("duplicate %#v", again.Charge)
	}
}

func TestParkedDrainIgnoresDrivingAndCharging(t *testing.T) {
	start := time.Date(2026, 10, 6, 1, 0, 0, 0, time.UTC)
	book, out := Apply(Book{}, Reading{At: start, Gear: "P", Charge: "Disconnected", Soc: f(80), RatedRange: f(300)}, "d1")
	if out.Drain == nil || !out.Drain.Open {
		t.Fatal("expected open drain")
	}
	book, out = Apply(book, Reading{At: start.Add(3 * time.Hour), Gear: "P", Charge: "Disconnected", Soc: f(76), RatedRange: f(292)}, "d2")
	if out.Drain == nil || !out.Drain.Open || out.Drain.SocEnd == nil || *out.Drain.SocEnd != 76 {
		t.Fatalf("update %#v", out.Drain)
	}
	_, out = Apply(book, Reading{At: start.Add(3*time.Hour + time.Minute), Gear: "D", Charge: "Disconnected", Soc: f(76)}, "d3")
	if out.Drain == nil || out.Drain.Open {
		t.Fatalf("close %#v", out.Drain)
	}
	_, drive := Apply(Book{}, Reading{At: start, Gear: "D", Charge: "Disconnected", Soc: f(70)}, "x")
	if drive.Drain != nil {
		t.Fatal("drive opened a drain")
	}
	_, charging := Apply(Book{}, Reading{At: start, Gear: "P", Charge: "Charging", Soc: f(40)}, "y")
	if charging.Drain != nil {
		t.Fatal("charge opened a drain")
	}
}

func TestBatteryAndSoftware(t *testing.T) {
	start := time.Date(2026, 10, 6, 8, 0, 0, 0, time.UTC)
	book, out := Apply(Book{}, Reading{At: start, Odometer: f(1000), RatedRange: f(310), Software: "2026.20.1"}, "b")
	if out.Battery == nil || out.Battery.Odometer != 1000 || out.Software != "2026.20.1" {
		t.Fatalf("%#v %s", out.Battery, out.Software)
	}
	_, quiet := Apply(book, Reading{At: start.Add(time.Hour), Odometer: f(1002), RatedRange: f(309), Software: "2026.20.1"}, "b2")
	if quiet.Battery != nil || quiet.Software != "" {
		t.Fatalf("should stay quiet %#v %s", quiet.Battery, quiet.Software)
	}
	_, next := Apply(book, Reading{At: start.Add(2 * time.Hour), Odometer: f(1006), RatedRange: f(308), Software: "2026.26.3"}, "b3")
	if next.Battery == nil || next.Battery.Odometer != 1006 || next.Software != "2026.26.3" {
		t.Fatalf("%#v %s", next.Battery, next.Software)
	}
}
