package horn

import (
	"strings"
	"testing"
	"time"
)

func TestGuestModeDoesNotNameTheDriver(t *testing.T) {
	seat, guest := true, true
	speed := 10.0
	h := Line("online", &speed, 75, &seat, &guest, true, nil, time.Now())
	if strings.Contains(h.Line, "Skylar is driving") || strings.Contains(h.Line, "Skylar hit") {
		t.Fatal(h.Line)
	}
	if !strings.Contains(h.Line, "not going to guess") {
		t.Fatal(h.Line)
	}
}

func TestOfflineIsNotCalledAsleep(t *testing.T) {
	seen := time.Date(2026, 10, 6, 15, 4, 0, 0, time.Local)
	h := Line("offline", nil, 75, nil, nil, false, &seen, seen)
	if strings.Contains(strings.ToLower(h.Line), "asleep") || !strings.Contains(h.Line, "offline") {
		t.Fatal(h.Line)
	}
}

func TestSpeedCopy(t *testing.T) {
	speed := 82.2
	h := Line("online", &speed, 75, nil, nil, true, nil, time.Now())
	if h.Line != "Skylar hit 82. I'm not mad, I'm just a horn." || h.Mood != "upset" {
		t.Fatal(h)
	}
}
