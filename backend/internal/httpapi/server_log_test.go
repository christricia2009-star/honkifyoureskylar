package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/model"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/telemetry"
)

func TestCommandRejectsUnknownAndSleepingCar(t *testing.T) {
	s := newTestServer(t)
	fleet := &fakeFleet{}
	s.Fleet = fleet
	seen := time.Now()
	if err := s.DB.UpsertVehicle(model.Vehicle{VIN: "5YJ3SKYLARLIVE001", Name: "Car", State: "asleep", LastSeen: &seen}); err != nil {
		t.Fatal(err)
	}
	token := sessionToken(t, s, false)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, authed(http.MethodPost, "/api/command", token, `{"name":"remote_boombox"}`))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "not available") {
		t.Fatalf("boombox %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, authed(http.MethodPost, "/api/command", token, `{"name":"door_lock"}`))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "asleep") {
		t.Fatalf("asleep %d %s", rec.Code, rec.Body.String())
	}
}

func TestDriverShareSkipsInvites(t *testing.T) {
	s := newTestServer(t)
	fleet := &fakeFleet{}
	s.Fleet = fleet
	seen := time.Now()
	if err := s.DB.UpsertVehicle(model.Vehicle{
		VIN: "5YJ3SKYLARLIVE001", Name: "Car", State: "online", Access: "DRIVER", LastSeen: &seen,
	}); err != nil {
		t.Fatal(err)
	}
	token := sessionToken(t, s, false)
	settings, _ := s.DB.EnsureSettings()
	settings.SelectedVIN = "5YJ3SKYLARLIVE001"
	if err := s.DB.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, authed(http.MethodGet, "/api/invites?refresh=1", token, ""))
	if fleet.rawCalls != 0 || !strings.Contains(rec.Body.String(), "not the owner") {
		t.Fatalf("calls %d body %s", fleet.rawCalls, rec.Body.String())
	}
}

func TestChargeLandsInLog(t *testing.T) {
	s := newTestServer(t)
	start := time.Date(2026, 10, 6, 18, 0, 0, 0, time.UTC)
	gear := "P"
	charge := "Charging"
	soc := 30.0
	added := 2.0
	fast := false
	if _, err := s.Live.Apply(false, telemetry.Update{
		At: start, VIN: "5YJ3SKYLARLIVE001", Gear: &gear, Charge: &charge, Soc: &soc,
		EnergyAddedKwh: &added, Fast: &fast, SignalCount: 1,
	}); err != nil {
		t.Fatal(err)
	}
	done := "Complete"
	soc = 70
	added = 15
	if _, err := s.Live.Apply(false, telemetry.Update{
		At: start.Add(30 * time.Minute), VIN: "5YJ3SKYLARLIVE001", Gear: &gear, Charge: &done, Soc: &soc,
		EnergyAddedKwh: &added, Fast: &fast, SignalCount: 1,
	}); err != nil {
		t.Fatal(err)
	}
	token := sessionToken(t, s, false)
	settings, _ := s.DB.EnsureSettings()
	settings.SelectedVIN = "5YJ3SKYLARLIVE001"
	if err := s.DB.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, authed(http.MethodGet, "/api/log", token, ""))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `"energyKwh":15`) || !strings.Contains(body, `"kind":"AC"`) {
		t.Fatalf("%d %s", rec.Code, body)
	}
}
