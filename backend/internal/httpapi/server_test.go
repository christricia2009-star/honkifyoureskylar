package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/config"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/demo"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/fleet"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/live"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/model"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/store"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/telemetry"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := demo.Seed(db); err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	cfg := config.Config{
		Domain:         "skylar.snapcollectibles.com",
		RedirectURI:    "https://skylar.snapcollectibles.com/path",
		IngestToken:    "test-ingest-token",
		DocsDir:        filepath.Join(root, "docs"),
		PublicKeyFile:  filepath.Join(root, "docs", ".well-known", "appspecific", "com.tesla.3p.public-key.pem"),
		FleetAPIBase:   "https://fleet-api.prd.na.vn.cloud.tesla.com",
		DemoAutoplay:   false,
	}
	svc := &live.Service{DB: db}
	s := New(cfg, db, svc)
	s.PublicKey = cfg.PublicKeyFile
	s.DocsDir = cfg.DocsDir
	s.WakePoll = 0
	return s
}

func sessionToken(t *testing.T, s *Server, demoMode bool) string {
	t.Helper()
	path := "/api/session/demo"
	if !demoMode {
		raw, _, err := s.DB.CreateSession(false, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, nil)
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"demo":true`) {
		t.Fatalf("demo session %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	const key = `"session":"`
	i := strings.Index(body, key)
	if i < 0 {
		t.Fatal(body)
	}
	rest := body[i+len(key):]
	j := strings.Index(rest, `"`)
	return rest[:j]
}

func authed(method, path, token, body string) *http.Request {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestDemoGarage(t *testing.T) {
	s := newTestServer(t)
	token := sessionToken(t, s, true)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, authed(http.MethodGet, "/api/garage", token, ""))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Skylar's Getaway Car") {
		t.Fatal(rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Skylar is driving") {
		t.Fatal(rec.Body.String())
	}
}

func TestPathAndPublicKey(t *testing.T) {
	s := newTestServer(t)
	h := s.Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/path", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "skylar.snapcollectibles.com/path") {
		t.Fatalf("path %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/appspecific/com.tesla.3p.public-key.pem", nil))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/x-pem-file" {
		t.Fatal(rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "BEGIN PUBLIC KEY") {
		t.Fatal(rec.Body.String())
	}
}

func TestIngestCreatesTripAndSpeedAlert(t *testing.T) {
	s := newTestServer(t)
	body := `{
	  "createdAt": "2026-10-06T20:10:00Z",
	  "vin": "5YJTESTVIN0000001",
	  "data": [
	    {"key": "VehicleSpeed", "value": {"stringValue": "82"}},
	    {"key": "Gear", "value": {"shiftStateValue": "ShiftStateD"}},
	    {"key": "Location", "value": {"locationValue": {"latitude": 34.2, "longitude": -118.3}}},
	    {"key": "DriverSeatOccupied", "value": {"booleanValue": true}},
	    {"key": "GuestModeEnabled", "value": {"booleanValue": false}}
	  ]
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ingest/telemetry", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-ingest-token")
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body.String())
	}
	token := sessionToken(t, s, false)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, authed(http.MethodGet, "/api/alerts", token, ""))
	if !strings.Contains(rec.Body.String(), "Skylar hit 82. I'm not mad, I'm just a horn.") {
		t.Fatal(rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, authed(http.MethodGet, "/api/trips", token, ""))
	if !strings.Contains(rec.Body.String(), "5YJTESTVIN0000001") {
		t.Fatal(rec.Body.String())
	}
	demoBody := strings.ReplaceAll(body, "5YJTESTVIN0000001", demo.VIN)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/ingest/telemetry", strings.NewReader(demoBody))
	req.Header.Set("Authorization", "Bearer test-ingest-token")
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body.String())
	}
}

type fakeFleet struct {
	gets, wakes, datas int
	state              string
}

func (f *fakeFleet) List(context.Context) ([]fleet.Vehicle, error) { return nil, nil }
func (f *fakeFleet) Get(context.Context, string) (fleet.Vehicle, error) {
	f.gets++
	return fleet.Vehicle{VIN: "5YJ3SKYLARLIVE001", Name: "Skylar's Real Car", State: f.state}, nil
}
func (f *fakeFleet) Wake(context.Context, string) (fleet.Vehicle, error) {
	f.wakes++
	f.state = "online"
	return fleet.Vehicle{VIN: "5YJ3SKYLARLIVE001", Name: "Skylar's Real Car", State: "online"}, nil
}
func (f *fakeFleet) Data(context.Context, string) (telemetry.Update, fleet.Vehicle, error) {
	f.datas++
	speed := 0.0
	gear := "P"
	return telemetry.Update{VIN: "5YJ3SKYLARLIVE001", At: time.Now(), SpeedMph: &speed, Gear: &gear, SignalCount: 1},
		fleet.Vehicle{VIN: "5YJ3SKYLARLIVE001", Name: "Skylar's Real Car", State: "online"}, nil
}
func (f *fakeFleet) Drivers(context.Context, string) (int, []model.Driver, string, error) {
	return 200, nil, "", nil
}

func TestRefreshConfirmWakesOnce(t *testing.T) {
	s := newTestServer(t)
	seen := time.Now().Add(-time.Hour)
	if err := s.DB.UpsertVehicle(model.Vehicle{
		VIN: "5YJ3SKYLARLIVE001", Demo: false, Name: "Skylar's Real Car", State: "asleep", LastSeen: &seen,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.SaveTeslaAuth(model.TeslaAuth{
		AccessToken: "test-access", RefreshToken: "test-refresh", Expiry: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	fake := &fakeFleet{state: "asleep"}
	s.Fleet = fake
	token := sessionToken(t, s, false)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, authed(http.MethodPost, "/api/refresh", token, `{"confirm":false}`))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "confirm_required") {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if fake.wakes != 0 || fake.datas != 0 {
		t.Fatalf("peeked early wake=%d data=%d", fake.wakes, fake.datas)
	}

	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, authed(http.MethodPost, "/api/refresh", token, `{"loop":true,"confirm":true}`))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Honk does not poll vehicle_data on a timer.") {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if fake.wakes != 0 || fake.datas != 0 {
		t.Fatalf("loop woke wake=%d data=%d", fake.wakes, fake.datas)
	}

	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, authed(http.MethodPost, "/api/refresh", token, `{"confirm":true}`))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if fake.wakes != 1 || fake.datas != 1 {
		t.Fatalf("gets=%d wakes=%d datas=%d", fake.gets, fake.wakes, fake.datas)
	}
	if !strings.Contains(rec.Body.String(), "Skylar's Real Car") {
		t.Fatal(rec.Body.String())
	}
}
