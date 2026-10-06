package httpapi

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/apns"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/billing"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/command"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/config"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/demo"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/fleet"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/horn"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/live"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/model"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/oauth"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/store"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/telemetry"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/trips"
)

type FleetAPI interface {
	List(ctx context.Context) ([]fleet.Vehicle, error)
	Get(ctx context.Context, vin string) (fleet.Vehicle, error)
	Wake(ctx context.Context, vin string) (fleet.Vehicle, error)
	Data(ctx context.Context, vin string) (telemetry.Update, fleet.Vehicle, error)
	Drivers(ctx context.Context, vin string) (int, []model.Driver, string, error)
	Do(ctx context.Context, method, path string, body any) (int, []byte, error)
}

type Server struct {
	Cfg       config.Config
	DB        *store.Store
	Live      *live.Service
	Fleet     FleetAPI
	Signer    *command.Signer
	APNS      *apns.Client
	DocsDir   string
	PublicKey string
	WakePoll  time.Duration

	authMu sync.Mutex
	hub    *hub
	primed sync.Map
}

func New(cfg config.Config, db *store.Store, svc *live.Service) *Server {
	s := &Server{
		Cfg: cfg, DB: db, Live: svc,
		DocsDir:   firstExisting(cfg.DocsDir, "docs", "../docs"),
		PublicKey: firstExisting(cfg.PublicKeyFile, "../docs/.well-known/appspecific/com.tesla.3p.public-key.pem", "docs/.well-known/appspecific/com.tesla.3p.public-key.pem"),
		WakePoll:  3 * time.Second,
		hub:       newHub(),
	}
	if svc != nil {
		svc.OnChange = s.onChange
	}
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /auth/tesla/start", s.oauthStart)
	mux.HandleFunc("GET /auth/tesla/callback", s.oauthCallback)
	mux.HandleFunc("GET /path", s.oauthCallback)
	mux.HandleFunc("POST /auth/tesla/finish", s.oauthAppFinish)
	mux.HandleFunc("POST /auth/exchange", s.exchange)
	mux.HandleFunc("POST /api/session/demo", s.demoSession)
	mux.HandleFunc("GET /api/garage", s.garage)
	mux.HandleFunc("POST /api/vehicles/select", s.selectVehicle)
	mux.HandleFunc("GET /api/trips", s.tripList)
	mux.HandleFunc("GET /api/trips/{id}", s.tripDetail)
	mux.HandleFunc("GET /api/drivers", s.drivers)
	mux.HandleFunc("GET /api/alerts", s.alerts)
	mux.HandleFunc("POST /api/alerts/{id}/read", s.readAlert)
	mux.HandleFunc("GET /api/log", s.carLog)
	mux.HandleFunc("GET /api/tesla-alerts", s.teslaAlerts)
	mux.HandleFunc("GET /api/software", s.softwareNotes)
	mux.HandleFunc("GET /api/chargers", s.chargers)
	mux.HandleFunc("GET /api/service", s.serviceStatus)
	mux.HandleFunc("GET /api/invites", s.invites)
	mux.HandleFunc("POST /api/invites", s.createInvite)
	mux.HandleFunc("POST /api/invites/{id}/revoke", s.revokeInvite)
	mux.HandleFunc("POST /api/command", s.command)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("GET /api/usage", s.usage)
	mux.HandleFunc("GET /api/setup", s.setup)
	mux.HandleFunc("POST /api/setup/ack", s.ack)
	mux.HandleFunc("POST /api/setup/telemetry", s.configureTelemetry)
	mux.HandleFunc("POST /api/devices", s.device)
	mux.HandleFunc("POST /api/refresh", s.refresh)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("POST /ingest/telemetry", s.ingest)
	mux.HandleFunc("POST /ingest/connectivity", s.ingest)
	mux.HandleFunc("POST /admin/register", s.register)
	mux.HandleFunc("GET /.well-known/appspecific/com.tesla.3p.public-key.pem", s.publicKey)
	mux.HandleFunc("GET /privacy", s.page("privacy/index.html"))
	mux.HandleFunc("GET /privacy/", s.page("privacy/index.html"))
	mux.HandleFunc("GET /{$}", s.page("index.html"))
	if s.DocsDir != "" {
		mux.Handle("GET /assets/", http.StripPrefix("/", http.FileServer(http.Dir(s.DocsDir))))
	}
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"demoAvailable":    true,
		"teslaConfigured":  s.Cfg.TeslaConfigured(),
		"linked":           s.DB.Linked(),
		"domain":           s.Cfg.Domain,
		"redirectUri":      s.Cfg.RedirectURI,
		"pairingUrl":       "https://www.tesla.com/_ak/" + s.Cfg.Domain,
		"publicKeyUrl":     "https://" + s.Cfg.Domain + "/.well-known/appspecific/com.tesla.3p.public-key.pem",
		"virtualKeyPaired": s.DB.Acked("virtual_key"),
	})
}

func (s *Server) oauthStart(w http.ResponseWriter, r *http.Request) {
	if !s.Cfg.TeslaConfigured() {
		http.Error(w, "Tesla credentials are not on this server.", http.StatusConflict)
		return
	}
	state := store.NewID()
	if err := s.DB.SaveOAuthState(state); err != nil {
		http.Error(w, "could not start sign-in", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, oauth.AuthorizeURL(s.Cfg, state), http.StatusFound)
}

func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		s.serveFile(w, r, "path/index.html")
		return
	}
	state := r.URL.Query().Get("state")
	if err := s.DB.ConsumeOAuthState(state); err != nil {
		http.Redirect(w, r, "honkifyoureskylar://auth?error=state", http.StatusFound)
		return
	}
	sess, err := s.finishTeslaCode(r.Context(), code)
	if err != nil {
		slog.Error("tesla code exchange failed", "err", err.Error())
		http.Redirect(w, r, "honkifyoureskylar://auth?error=exchange", http.StatusFound)
		return
	}
	handoff, err := s.DB.SaveHandoff(sess.ID)
	if err != nil {
		http.Error(w, "could not finish sign-in", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "honkifyoureskylar://auth?code="+handoff, http.StatusFound)
}

func (s *Server) finishTeslaCode(ctx context.Context, code string) (store.Session, error) {
	tok, err := oauth.Exchange(ctx, s.Cfg, code)
	if err != nil {
		return store.Session{}, err
	}
	if err := s.DB.SaveTeslaAuth(model.TeslaAuth{
		AccessToken: tok.Access, RefreshToken: tok.Refresh, Expiry: tok.Expiry,
		Scopes: oauth.Scopes, FleetBase: s.Cfg.FleetAPIBase,
	}); err != nil {
		return store.Session{}, err
	}
	s.syncVehicles(ctx)
	_, sess, err := s.DB.CreateSession(false, 90*24*time.Hour)
	return sess, err
}

func (s *Server) oauthAppFinish(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if err := readJSON(r, &body); err != nil || body.Code == "" || body.State == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_code", "message": "The horn did not get a sign-in code."})
		return
	}
	if err := s.DB.ConsumeOAuthState(body.State); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "state", "message": "Sign-in expired. Try again."})
		return
	}
	sess, err := s.finishTeslaCode(r.Context(), body.Code)
	if err != nil {
		slog.Error("tesla code exchange failed", "err", err.Error())
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "exchange", "message": "Tesla did not finish sign-in."})
		return
	}
	raw, err := s.DB.IssueTokenForSession(sess)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session", "message": "Could not start an app session."})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": raw, "demo": false})
}

func (s *Server) exchange(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := readJSON(r, &body); err != nil || body.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_code"})
		return
	}
	sess, err := s.DB.ConsumeHandoff(body.Code)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "code_expired"})
		return
	}
	raw, err := s.DB.IssueTokenForSession(sess)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": raw, "demo": sess.Demo})
}

func (s *Server) demoSession(w http.ResponseWriter, r *http.Request) {
	raw, _, err := s.DB.CreateSession(true, 90*24*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": raw, "demo": true})
}

func (s *Server) garage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.require(w, r)
	if !ok {
		return
	}
	if !sess.Demo {
		s.rememberAccess(r.Context())
	}
	if v, err := s.focus(sess.Demo); err == nil {
		s.maybePrime(r.Context(), sess.Demo, v)
	}
	s.writeGarage(w, sess.Demo)
}

func (s *Server) selectVehicle(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.require(w, r)
	if !ok {
		return
	}
	var body struct {
		VIN string `json:"vin"`
	}
	if err := readJSON(r, &body); err != nil || body.VIN == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "vin"})
		return
	}
	v, err := s.DB.Vehicle(body.VIN)
	if err != nil || v.Demo != sess.Demo {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no_car"})
		return
	}
	settings, err := s.DB.EnsureSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "settings"})
		return
	}
	settings.SelectedVIN = v.VIN
	if err := s.DB.SaveSettings(settings); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "settings"})
		return
	}
	s.maybePrime(r.Context(), sess.Demo, v)
	s.writeGarage(w, sess.Demo)
}

func (s *Server) tripList(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.require(w, r)
	if !ok {
		return
	}
	v, err := s.focus(sess.Demo)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"trips": []model.TripView{}})
		return
	}
	settings, _ := s.DB.EnsureSettings()
	list, err := s.DB.Trips(v.VIN)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "trips"})
		return
	}
	out := make([]model.TripView, 0, len(list))
	for _, tr := range list {
		out = append(out, tripView(tr, settings.SpeedLimitMph, 0))
	}
	writeJSON(w, http.StatusOK, map[string]any{"trips": out})
}

func (s *Server) tripDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.require(w, r); !ok {
		return
	}
	tr, err := s.DB.Trip(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	settings, _ := s.DB.EnsureSettings()
	writeJSON(w, http.StatusOK, tripView(tr, settings.SpeedLimitMph, 0))
}

func (s *Server) drivers(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.require(w, r)
	if !ok {
		return
	}
	v, err := s.focus(sess.Demo)
	note := model.ActiveDriverNote
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"drivers": []model.Driver{}, "problem": "No car yet.", "note": note})
		return
	}
	if !sess.Demo {
		s.rememberAccess(r.Context())
		if fresh, ferr := s.DB.Vehicle(v.VIN); ferr == nil {
			v = fresh
		}
	}
	if msg, blocked := driverShareProblem(v.Access); blocked {
		_ = s.DB.SaveDrivers(v.VIN, sess.Demo, nil, msg)
		writeJSON(w, http.StatusOK, map[string]any{"drivers": []model.Driver{}, "problem": msg, "note": note})
		return
	}
	cached, problem, when, err := s.DB.Drivers(v.VIN)
	problem = ownerDriverProblem(problem)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "drivers"})
		return
	}
	refresh := r.URL.Query().Get("refresh") == "1"
	staleRefusal := strings.Contains(problem, "refused the driver list")
	if sess.Demo || (s.Fleet == nil) || (!refresh && !staleRefusal && !when.IsZero() && time.Since(when) < 10*time.Minute) {
		writeJSON(w, http.StatusOK, map[string]any{"drivers": emptyDrivers(cached), "problem": problem, "note": note})
		return
	}
	status, drivers, problem, callErr := s.Fleet.Drivers(r.Context(), v.VIN)
	if callErr == nil && status < 500 {
		_ = s.DB.AddUsage(false, "metadata", 1)
	}
	if callErr != nil {
		problem = "The horn could not reach Tesla for the driver list."
	}
	problem = ownerDriverProblem(problem)
	if err := s.DB.SaveDrivers(v.VIN, false, drivers, problem); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "drivers"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"drivers": emptyDrivers(drivers), "problem": problem, "note": note})
}

func (s *Server) alerts(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.require(w, r)
	if !ok {
		return
	}
	list, err := s.DB.Alerts(sess.Demo, 50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "alerts"})
		return
	}
	if list == nil {
		list = []model.Alert{}
	}
	settings, _ := s.DB.EnsureSettings()
	writeJSON(w, http.StatusOK, map[string]any{"alerts": list, "settings": settings})
}

func (s *Server) readAlert(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.require(w, r); !ok {
		return
	}
	if err := s.DB.MarkAlertRead(r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "alert"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.require(w, r); !ok {
		return
	}
	settings, err := s.DB.EnsureSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "settings"})
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.require(w, r); !ok {
		return
	}
	var settings model.Settings
	if err := readJSON(r, &settings); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_settings"})
		return
	}
	if settings.SpeedLimitMph < 20 || settings.SpeedLimitMph > 130 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "speed_limit"})
		return
	}
	if !validClock(settings.CurfewStart) || !validClock(settings.CurfewEnd) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "curfew"})
		return
	}
	if settings.Timezone != "" {
		if _, err := time.LoadLocation(settings.Timezone); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "timezone"})
			return
		}
	}
	if settings.HomeSet && (settings.HomeRadiusMeters < 50 || settings.HomeRadiusMeters > 5000) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "radius"})
		return
	}
	if err := s.DB.SaveSettings(settings); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "settings"})
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) usage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.require(w, r)
	if !ok {
		return
	}
	u, err := s.DB.Usage(sess.Demo, time.Now())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "usage"})
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.require(w, r); !ok {
		return
	}
	keyOK := s.PublicKey != ""
	if keyOK {
		if _, err := os.Stat(s.PublicKey); err != nil {
			keyOK = false
		}
	}
	registered, _ := s.DB.Meta("registered")
	items := []map[string]any{
		{"id": "app_registration", "title": "App registration", "done": s.Cfg.TeslaConfigured() && registered == "ok",
			"detail": "Create the app at developer.tesla.com, then call the partner register endpoint. Credentials stay in the server environment."},
		{"id": "domain_key", "title": "Domain key", "done": keyOK,
			"detail": "The public key must answer at https://" + s.Cfg.Domain + "/.well-known/appspecific/com.tesla.3p.public-key.pem. The private key stays on the server."},
		{"id": "virtual_key", "title": "Car key for streaming", "done": s.DB.Acked("virtual_key"),
			"detail": "Sign-in already lets Honk read a car that is awake. The link https://www.tesla.com/_ak/" + s.Cfg.Domain + " is a key you add inside the Tesla app so the car will stream on its own and accept commands. A snapshot does not need that key."},
		{"id": "billing", "title": "Payment method and billing cap", "done": s.DB.Acked("billing"),
			"detail": "In the Tesla developer portal, add a card and set a billing limit. A limit of $0 disables the API. The $10 monthly credit is the working assumption. Honk does not poll vehicle_data on a timer."},
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":        items,
		"pairingUrl":   "https://www.tesla.com/_ak/" + s.Cfg.Domain,
		"publicKeyUrl": "https://" + s.Cfg.Domain + "/.well-known/appspecific/com.tesla.3p.public-key.pem",
		"redirectUri":  s.Cfg.RedirectURI,
		"scopes":       strings.Split(oauth.Scopes, " "),
		"locationNote": "vehicle_location makes the car show the location-sharing icon. That is expected.",
	})
}

func (s *Server) ack(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.require(w, r); !ok {
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_ack"})
		return
	}
	if body.ID != "virtual_key" && body.ID != "billing" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown_ack"})
		return
	}
	if err := s.DB.Ack(body.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ack"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) device(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.require(w, r); !ok {
		return
	}
	var body struct {
		PushToken string `json:"pushToken"`
	}
	if err := readJSON(r, &body); err != nil || body.PushToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "push_token"})
		return
	}
	if err := s.DB.SaveDeviceToken(body.PushToken); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "device"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.require(w, r)
	if !ok {
		return
	}
	var body struct {
		Confirm bool   `json:"confirm"`
		Loop    bool   `json:"loop"`
		VIN     string `json:"vin"`
	}
	_ = readJSON(r, &body)
	if body.Loop {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":   "no_poll_loop",
			"message": "Honk does not poll vehicle_data on a timer.",
		})
		return
	}
	v, err := s.focus(sess.Demo)
	if body.VIN != "" {
		got, gerr := s.DB.Vehicle(body.VIN)
		if gerr == nil && got.Demo == sess.Demo {
			v = got
			err = nil
		}
	}
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no_car"})
		return
	}
	willWake := canWake(v.State)
	if !body.Confirm {
		est := billing.USDPerVehicleData
		if willWake {
			est += billing.USDPerWake
		}
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":          "confirm_required",
			"message":        warnText(v.State),
			"willWake":       willWake,
			"estimatedUsd":   est,
			"vehicleDataUsd": billing.USDPerVehicleData,
			"wakeUsd":        billing.USDPerWake,
		})
		return
	}
	if sess.Demo {
		_ = s.DB.AddUsage(true, "vehicle_data", 1)
		if willWake {
			_ = s.DB.AddUsage(true, "wake", 1)
		}
		s.writeGarage(w, true)
		return
	}
	if s.Fleet == nil || !s.DB.Linked() {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error":   "not_linked",
			"message": "Sign in with Tesla before peeking at the real car.",
		})
		return
	}
	got, gerr := s.Fleet.Get(r.Context(), v.VIN)
	if gerr == nil || billable(gerr) {
		_ = s.DB.AddUsage(false, "metadata", 1)
	}
	if gerr != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "tesla", "message": gerr.Error()})
		return
	}
	state := got.State
	seen := time.Now()
	_ = s.DB.UpsertVehicle(model.Vehicle{VIN: got.VIN, Demo: false, Name: got.Name, State: state, Access: got.Access, LastSeen: &seen})
	triedWake := false
	if canWake(state) {
		slog.Info("confirmed peek", "car", vinSuffix(v.VIN), "state", state)
		woke, werr := s.Fleet.Wake(r.Context(), v.VIN)
		triedWake = true
		if werr == nil || billable(werr) {
			_ = s.DB.AddUsage(false, "wake", 1)
		}
		if werr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "wake", "message": werr.Error()})
			return
		}
		state = woke.State
		for i := 0; i < 8 && !strings.EqualFold(state, "online"); i++ {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(s.WakePoll):
			}
			again, aerr := s.Fleet.Get(r.Context(), v.VIN)
			if aerr == nil || billable(aerr) {
				_ = s.DB.AddUsage(false, "metadata", 1)
			}
			if aerr != nil {
				break
			}
			state = again.State
		}
		slog.Info("wake result", "car", vinSuffix(v.VIN), "state", state)
	}
	if !strings.EqualFold(state, "online") {
		_ = s.DB.TouchVehicle(v.VIN, state, time.Now())
		note := "This car is " + state + ". Honk reads a car that is already online. It wakes a car only when Tesla says asleep or offline, and only after you confirm."
		if triedWake {
			note = "Tesla still lists this car as " + state + ". Honk tried one wake and stopped. It will not keep poking."
		}
		_ = s.DB.SetMeta("read:"+v.VIN, note)
		s.writeGarage(w, false)
		return
	}
	up, veh, derr := s.Fleet.Data(r.Context(), v.VIN)
	if derr == nil || billable(derr) {
		_ = s.DB.AddUsage(false, "vehicle_data", 1)
	}
	if derr != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "vehicle_data", "message": derr.Error()})
		return
	}
	if err := s.absorbReading(v.VIN, up, veh); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "apply", "message": "Honk read the car but could not store it."})
		return
	}
	s.writeGarage(w, false)
}

func (s *Server) configureTelemetry(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.require(w, r); !ok {
		return
	}
	var body struct {
		Confirm bool `json:"confirm"`
	}
	_ = readJSON(r, &body)
	if !body.Confirm {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "confirm_required",
			"message": "Sending the telemetry config is one signed command, about $0.001. It does not start a poll loop.",
		})
		return
	}
	if s.Signer == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "no_private_key", "message": "The virtual-key private key is not loaded on the server."})
		return
	}
	if !s.DB.Linked() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "not_linked"})
		return
	}
	v, err := s.focus(false)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no_car"})
		return
	}
	payload, err := s.telemetryPayload(v.VIN)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "config", "message": err.Error()})
		return
	}
	token, err := s.teslaToken(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "token"})
		return
	}
	code, resp, err := s.Signer.PostTelemetryConfig(r.Context(), token, payload)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "sign"})
		return
	}
	if code < 500 {
		_ = s.DB.AddUsage(false, "command", 1)
	}
	if code < 300 {
		_ = s.DB.Ack("virtual_key")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(resp)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.require(w, r)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	if g, err := s.buildGarage(sess.Demo); err == nil {
		b, _ := json.Marshal(g)
		fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", b)
		flusher.Flush()
	}
	ch := s.hub.sub()
	defer s.hub.unsub(ch)
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			if ev.demo != sess.Demo {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.name, ev.data)
			flusher.Flush()
		case <-ping.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	if !s.allowIngest(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "ingest"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body"})
		return
	}
	updates, err := telemetry.Parse(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "parse", "message": err.Error()})
		return
	}
	var alerts []model.Alert
	for _, up := range updates {
		if up.VIN == "" || up.VIN == demo.VIN {
			continue
		}
		made, err := s.Live.Apply(false, up)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "apply"})
			return
		}
		alerts = append(alerts, made...)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "alerts": len(alerts)})
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if s.Cfg.AdminToken == "" || subtle.ConstantTimeCompare([]byte(bearer(r)), []byte(s.Cfg.AdminToken)) != 1 {
		http.NotFound(w, r)
		return
	}
	if !s.Cfg.TeslaConfigured() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "no_credentials"})
		return
	}
	token, err := oauth.PartnerToken(r.Context(), s.Cfg)
	if err != nil {
		slog.Error("partner token failed", "err", err.Error())
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "partner_token"})
		return
	}
	code, body, err := oauth.Register(r.Context(), s.Cfg, token)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "register"})
		return
	}
	if code < 300 {
		_ = s.DB.SetMeta("registered", "ok")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(body))
}

func (s *Server) publicKey(w http.ResponseWriter, r *http.Request) {
	if s.PublicKey == "" {
		http.NotFound(w, r)
		return
	}
	b, err := os.ReadFile(s.PublicKey)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func (s *Server) page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.serveFile(w, r, name) }
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, name string) {
	if s.DocsDir == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(s.DocsDir, name))
}

// UseFleet wires the real Fleet API client. The phone never sees these tokens.
func (s *Server) UseFleet() {
	if !s.Cfg.TeslaConfigured() || s.Cfg.FleetAPIBase == "" {
		return
	}
	s.Fleet = &fleet.Client{Base: s.Cfg.FleetAPIBase, Token: s.teslaToken}
}

func (s *Server) writeGarage(w http.ResponseWriter, demoMode bool) {
	g, err := s.buildGarage(demoMode)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "garage"})
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) buildGarage(demoMode bool) (model.Garage, error) {
	settings, err := s.DB.EnsureSettings()
	if err != nil {
		return model.Garage{}, err
	}
	usage, err := s.DB.Usage(demoMode, time.Now())
	if err != nil {
		return model.Garage{}, err
	}
	g := model.Garage{
		Demo: demoMode, Linked: s.DB.Linked(), Usage: usage, ServerTime: model.APITime(time.Now()),
		Horn:     model.Horn{Mood: "napping", Line: "No car is parked in the garage yet."},
		Vehicles: []model.VehicleBrief{}, Grabs: []model.Grab{},
	}
	cars, err := s.DB.Vehicles(demoMode)
	if err != nil {
		return model.Garage{}, err
	}
	v, err := s.focus(demoMode)
	for _, car := range cars {
		g.Vehicles = append(g.Vehicles, model.VehicleBrief{
			VIN: car.VIN, Name: car.Name, State: car.State, Access: car.Access, Selected: err == nil && car.VIN == v.VIN,
		})
	}
	if err != nil {
		return g, nil
	}
	sn, snapErr := s.DB.Snapshot(v.VIN)
	if snapErr != nil && !errors.Is(snapErr, sql.ErrNoRows) {
		return model.Garage{}, snapErr
	}
	facts := []model.Fact{}
	if sn.Detail != "" {
		_ = json.Unmarshal([]byte(sn.Detail), &facts)
	}
	if facts == nil {
		facts = []model.Fact{}
	}
	view := model.VehicleView{
		VIN: v.VIN, Name: v.Name, State: v.State, SpeedMph: sn.SpeedMph, Gear: sn.Gear,
		GearLabel: trips.GearLabel(sn.Gear), Latitude: sn.Lat, Longitude: sn.Lng, Soc: sn.Soc,
		EstRangeMiles: sn.RangeMi, ChargeState: or(sn.ChargeState, "Unknown"), DoorsOpen: sn.DoorsOpen,
		DoorSummary: or(sn.DoorSummary, "Unknown"), Locked: sn.Locked, DriverSeatOccupied: sn.Seat,
		GuestMode: sn.Guest, ActiveDriverNote: model.ActiveDriverNote, Facts: facts,
	}
	if v.LastSeen != nil {
		t := model.APITime(*v.LastSeen)
		view.LastSeen = &t
	}
	open, err := s.DB.OpenTrip(v.VIN)
	if err != nil {
		return model.Garage{}, err
	}
	view.InTrip = open != nil
	g.Vehicle = &view
	if open != nil {
		tv := tripView(*open, settings.SpeedLimitMph, 300)
		g.ActiveTrip = &tv
	}
	limit := settings.SpeedLimitMph
	if limit <= 0 {
		limit = 75
	}
	g.Horn = horn.Line(v.State, sn.SpeedMph, limit, sn.Seat, sn.Guest, view.InTrip, v.LastSeen, time.Now())
	grabs, err := s.DB.Grabs(v.VIN, 20)
	if err != nil {
		return model.Garage{}, err
	}
	if grabs == nil {
		grabs = []model.Grab{}
	}
	g.Grabs = grabs
	if note, _ := s.DB.Meta("read:" + v.VIN); note != "" {
		g.SnapshotNote = note
	}
	return g, nil
}

func (s *Server) PrimeAwake(ctx context.Context) {
	if s.Fleet == nil || !s.DB.Linked() {
		return
	}
	cars, err := s.DB.Vehicles(false)
	if err != nil {
		slog.Error("prime cars", "err", err.Error())
		return
	}
	for _, car := range cars {
		s.maybePrime(ctx, false, car)
	}
}

func (s *Server) maybePrime(ctx context.Context, demo bool, v model.Vehicle) {
	if demo || s.Fleet == nil || !s.DB.Linked() || !strings.EqualFold(v.State, "online") {
		return
	}
	sn, err := s.DB.Snapshot(v.VIN)
	if err == nil && !sn.UpdatedAt.IsZero() {
		return
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return
	}
	if _, loaded := s.primed.LoadOrStore(v.VIN, true); loaded {
		return
	}
	up, veh, derr := s.Fleet.Data(ctx, v.VIN)
	if derr == nil || billable(derr) {
		_ = s.DB.AddUsage(false, "vehicle_data", 1)
	}
	if derr != nil {
		_ = s.DB.SetMeta("read:"+v.VIN, publicTeslaError(derr))
		return
	}
	if err := s.absorbReading(v.VIN, up, veh); err != nil {
		slog.Error("store car reading", "err", err.Error())
	}
}

func (s *Server) absorbReading(vin string, up telemetry.Update, veh fleet.Vehicle) error {
	up.SignalCount = 0
	if up.VIN == "" {
		up.VIN = vin
	}
	now := time.Now()
	name := veh.Name
	if name == "" {
		name = vin
	}
	if err := s.DB.UpsertVehicle(model.Vehicle{VIN: vin, Demo: false, Name: name, State: "online", LastSeen: &now}); err != nil {
		return err
	}
	if _, err := s.Live.Apply(false, up); err != nil {
		return err
	}
	_ = s.DB.SetMeta("read:"+vin, "")
	return s.recordGrab(false, vin, "online")
}

func (s *Server) recordGrab(demo bool, vin, state string) error {
	sn, err := s.DB.Snapshot(vin)
	if err != nil {
		return err
	}
	facts := sn.Detail
	if facts == "" {
		facts = "[]"
	}
	return s.DB.InsertGrab(vin, demo, state, grabSummary(sn), facts, sn.UpdatedAt)
}

func grabSummary(sn model.Snapshot) string {
	parts := []string{trips.GearLabel(sn.Gear)}
	if sn.Soc != nil {
		parts = append(parts, fmt.Sprintf("%d%%", int(*sn.Soc+0.5)))
	}
	if sn.RangeMi != nil {
		parts = append(parts, fmt.Sprintf("%.0f mi", *sn.RangeMi))
	}
	if sn.Locked != nil {
		if *sn.Locked {
			parts = append(parts, "locked")
		} else {
			parts = append(parts, "unlocked")
		}
	}
	return strings.Join(parts, " · ")
}

func publicTeslaError(err error) string {
	var se *fleet.StatusError
	if errors.As(err, &se) {
		var raw map[string]any
		if json.Unmarshal([]byte(se.Body), &raw) == nil {
			if msg, ok := raw["error"].(string); ok && msg != "" && len(msg) < 180 {
				return fmt.Sprintf("Tesla returned %d: %s", se.Status, msg)
			}
		}
		return fmt.Sprintf("Tesla returned %d for the car reading.", se.Status)
	}
	return "The horn could not reach Tesla for a reading."
}

func (s *Server) rememberAccess(ctx context.Context) {
	if s.Fleet == nil {
		return
	}
	cars, err := s.DB.Vehicles(false)
	if err != nil {
		return
	}
	missing := false
	for _, car := range cars {
		if car.Access == "" {
			missing = true
			break
		}
	}
	if !missing {
		return
	}
	list, err := s.Fleet.List(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	for _, v := range list {
		seen := now
		_ = s.DB.UpsertVehicle(model.Vehicle{VIN: v.VIN, Demo: false, Name: v.Name, State: v.State, Access: v.Access, LastSeen: &seen})
	}
}

func driverShareProblem(access string) (string, bool) {
	if access == "" || strings.EqualFold(access, "OWNER") {
		return "", false
	}
	return "This Tesla account is a " + strings.ToLower(access) + " on this car, not the owner. Tesla only sends the allow-list to the owner.", true
}

func ownerDriverProblem(problem string) string {
	const old = "Tesla only returns the driver list to the vehicle owner, and only when vehicle_device_data is granted."
	const next = "Tesla refused the driver list for this car. Sign-in already includes vehicle_device_data. Tesla only gives that list to the owner of this specific car."
	if problem == old {
		return next
	}
	return problem
}

func (s *Server) focus(demoMode bool) (model.Vehicle, error) {
	settings, err := s.DB.EnsureSettings()
	if err != nil {
		return model.Vehicle{}, err
	}
	if settings.SelectedVIN != "" {
		v, err := s.DB.Vehicle(settings.SelectedVIN)
		if err == nil && v.Demo == demoMode {
			return v, nil
		}
	}
	list, err := s.DB.Vehicles(demoMode)
	if err != nil {
		return model.Vehicle{}, err
	}
	if len(list) == 0 {
		return model.Vehicle{}, sql.ErrNoRows
	}
	return list[0], nil
}

func (s *Server) syncVehicles(ctx context.Context) {
	if s.Fleet == nil {
		return
	}
	list, err := s.Fleet.List(ctx)
	if err != nil {
		slog.Error("vehicle list failed", "err", err.Error())
		if billable(err) {
			_ = s.DB.AddUsage(false, "metadata", 1)
		}
		return
	}
	_ = s.DB.AddUsage(false, "metadata", 1)
	now := time.Now()
	for _, v := range list {
		seen := now
		_ = s.DB.UpsertVehicle(model.Vehicle{VIN: v.VIN, Demo: false, Name: v.Name, State: v.State, Access: v.Access, LastSeen: &seen})
	}
	if len(list) == 1 {
		settings, err := s.DB.EnsureSettings()
		if err == nil && settings.SelectedVIN == "" {
			settings.SelectedVIN = list[0].VIN
			_ = s.DB.SaveSettings(settings)
		}
	}
}

func (s *Server) teslaToken(ctx context.Context) (string, error) {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	auth, err := s.DB.TeslaAuth()
	if err != nil {
		return "", err
	}
	if time.Until(auth.Expiry) > 2*time.Minute {
		return auth.AccessToken, nil
	}
	tok, err := oauth.Refresh(ctx, s.Cfg, auth.RefreshToken)
	if err != nil {
		return "", err
	}
	if tok.Refresh == "" {
		tok.Refresh = auth.RefreshToken
	}
	auth.AccessToken = tok.Access
	auth.RefreshToken = tok.Refresh
	auth.Expiry = tok.Expiry
	if err := s.DB.SaveTeslaAuth(auth); err != nil {
		return "", err
	}
	return auth.AccessToken, nil
}

func (s *Server) telemetryPayload(vin string) ([]byte, error) {
	if s.Cfg.TelemetryCAFile == "" {
		return nil, fmt.Errorf("set TELEMETRY_CA_FILE to the Fleet Telemetry server certificate")
	}
	ca, err := os.ReadFile(s.Cfg.TelemetryCAFile)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{
		"VehicleSpeed":        map[string]any{"interval_seconds": 10, "minimum_delta": 1},
		"Gear":                map[string]any{"interval_seconds": 10},
		"Location":            map[string]any{"interval_seconds": 10},
		"Soc":                 map[string]any{"interval_seconds": 60},
		"EstBatteryRange":     map[string]any{"interval_seconds": 60},
		"ChargeState":         map[string]any{"interval_seconds": 60},
		"DoorState":           map[string]any{"interval_seconds": 10},
		"Locked":              map[string]any{"interval_seconds": 10},
		"DriverSeatOccupied":  map[string]any{"interval_seconds": 10},
		"GuestModeEnabled":    map[string]any{"interval_seconds": 10},
		"Odometer":            map[string]any{"interval_seconds": 60},
		"RatedRange":          map[string]any{"interval_seconds": 60},
		"ACChargingEnergyIn":  map[string]any{"interval_seconds": 60},
		"DCChargingEnergyIn":  map[string]any{"interval_seconds": 60},
		"EnergyRemaining":     map[string]any{"interval_seconds": 60},
		"LifetimeEnergyUsed":  map[string]any{"interval_seconds": 60},
		"DetailedChargeState": map[string]any{"interval_seconds": 60},
		"FastChargerPresent":  map[string]any{"interval_seconds": 60},
		"FastChargerType":     map[string]any{"interval_seconds": 60},
		"Version":             map[string]any{"interval_seconds": 3600},
	}
	body := map[string]any{
		"vins": []string{vin},
		"config": map[string]any{
			"hostname": s.Cfg.TelemetryHost, "port": s.Cfg.TelemetryPort, "ca": string(ca),
			"prefer_typed": true, "fields": fields, "alert_types": []string{"service"},
			"exp": time.Now().Add(180 * 24 * time.Hour).Unix(),
		},
	}
	return json.Marshal(body)
}

func (s *Server) onChange(demoMode bool, alerts []model.Alert) {
	if g, err := s.buildGarage(demoMode); err == nil {
		s.hub.publish(demoMode, "snapshot", g)
	}
	for _, a := range alerts {
		s.hub.publish(demoMode, "alert", a)
		s.push(a)
	}
}

func (s *Server) push(a model.Alert) {
	if s.APNS == nil {
		return
	}
	tokens, err := s.DB.DeviceTokens()
	if err != nil {
		return
	}
	for _, token := range tokens {
		if err := s.APNS.Notify(token, "Honk if You're Skylar", a.Message); err != nil {
			slog.Error("apns failed", "err", err.Error())
		}
	}
}

func (s *Server) allowIngest(r *http.Request) bool {
	got := bearer(r)
	if got == "" {
		got = r.Header.Get("X-Honk-Ingest-Token")
	}
	if s.Cfg.IngestToken != "" {
		return subtle.ConstantTimeCompare([]byte(got), []byte(s.Cfg.IngestToken)) == 1
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) require(w http.ResponseWriter, r *http.Request) (store.Session, bool) {
	sess, err := s.DB.LookupSession(bearer(r))
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return store.Session{}, false
	}
	return sess, true
}

func tripView(tr model.Trip, limit float64, trim int) model.TripView {
	poly := tr.Polyline
	if trim > 0 && len(poly) > trim {
		poly = poly[len(poly)-trim:]
	}
	if tr.SpeedLimitMph > 0 {
		limit = tr.SpeedLimitMph
	}
	view := model.TripView{
		ID: tr.ID, VIN: tr.VIN, StartedAt: model.APITime(tr.StartedAt), MaxSpeedMph: tr.MaxSpeedMph,
		DistanceMiles: tr.DistanceMiles, OverLimit: tr.OverLimit, SpeedLimitMph: limit, Polyline: poly,
		SeatOccupied: tr.SeatOccupied, GuestMode: tr.GuestMode, PendingClose: tr.PendingClose, Callout: tr.Callout,
	}
	if tr.EndedAt != nil {
		t := model.APITime(*tr.EndedAt)
		view.EndedAt = &t
	}
	if view.Polyline == nil {
		view.Polyline = []model.LatLng{}
	}
	fillEconomy(&view, tr)
	return view
}

func fillEconomy(view *model.TripView, tr model.Trip) {
	if tr.DistanceMiles >= 0.2 && tr.StartKwh != nil && tr.EndKwh != nil {
		delta := 0.0
		switch tr.KwhKind {
		case "lifetime":
			delta = *tr.EndKwh - *tr.StartKwh
		case "remaining":
			delta = *tr.StartKwh - *tr.EndKwh
		}
		if delta > 0.01 {
			wh := delta * 1000 / tr.DistanceMiles
			view.WhPerMile = &wh
			if tr.KwhKind == "lifetime" {
				view.EnergyNote = "Watt-hours per mile from Tesla's lifetime energy counter."
			} else {
				view.EnergyNote = "Watt-hours per mile from the drop in energy remaining."
			}
		}
	}
	if tr.StartRated != nil && tr.EndRated != nil {
		used := *tr.StartRated - *tr.EndRated
		if used > 0.05 {
			view.RangeUsed = &used
			if tr.RangeKind == "rated" {
				view.RangeNote = "Rated miles used"
			} else {
				view.RangeNote = "Estimated range used"
			}
		}
	}
}

func canWake(state string) bool {
	switch strings.ToLower(state) {
	case "asleep", "offline":
		return true
	default:
		return false
	}
}

func warnText(state string) string {
	switch strings.ToLower(state) {
	case "asleep":
		return "This peeks once. Honk will not poll on a timer. The car is asleep, so this also wakes it: about $0.02 to wake and $0.002 for the snapshot. The $10 monthly credit is the cushion."
	case "offline":
		return "This peeks once. Honk will not poll on a timer. Tesla lists this car as offline, which usually means it is asleep and out of touch. This tries one wake, about $0.02, then one snapshot if it comes online. It will not keep trying."
	default:
		return "This peeks once. Honk will not poll on a timer. One vehicle_data call is about $0.002. The $10 monthly credit covers a lot of these."
	}
}

func vinSuffix(vin string) string {
	if len(vin) <= 6 {
		return vin
	}
	return vin[len(vin)-6:]
}

func billable(err error) bool {
	var se *fleet.StatusError
	if errors.As(err, &se) {
		return se.Status < 500
	}
	return false
}

func emptyDrivers(in []model.Driver) []model.Driver {
	if in == nil {
		return []model.Driver{}
	}
	return in
}

func validClock(v string) bool {
	if len(v) != 5 || v[2] != ':' {
		return false
	}
	h := int(v[0]-'0')*10 + int(v[1]-'0')
	m := int(v[3]-'0')*10 + int(v[4]-'0')
	return h >= 0 && h <= 23 && m >= 0 && m <= 59 && v[0] >= '0' && v[1] >= '0' && v[3] >= '0' && v[4] >= '0'
}

func or(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	raw, ok := strings.CutPrefix(h, "Bearer ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(raw)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	return dec.Decode(v)
}

func firstExisting(paths ...string) string {
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, p := range paths {
		if p != "" {
			return p
		}
	}
	return ""
}

type event struct {
	demo bool
	name string
	data []byte
}

type hub struct {
	mu   sync.Mutex
	subs map[chan event]struct{}
}

func newHub() *hub { return &hub{subs: map[chan event]struct{}{}} }

func (h *hub) sub() chan event {
	ch := make(chan event, 8)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *hub) unsub(ch chan event) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func (h *hub) publish(demoMode bool, name string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	ev := event{demo: demoMode, name: name, data: b}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}
