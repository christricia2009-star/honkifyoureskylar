package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/fleet"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/model"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/store"
)

const teslaReadNote = "Honk asked Tesla once for this. It does not refresh on a timer."

func (s *Server) carLog(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.require(w, r)
	if !ok {
		return
	}
	v, err := s.focus(sess.Demo)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"charges": []model.ChargeView{}, "drains": []model.DrainView{},
			"battery": []model.BatteryView{}, "software": []model.SoftwareView{},
			"note": "No car yet.",
		})
		return
	}
	charges, err := s.DB.Charges(v.VIN)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "charges"})
		return
	}
	drains, err := s.DB.Drains(v.VIN)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "drains"})
		return
	}
	battery, err := s.DB.Battery(v.VIN)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "battery"})
		return
	}
	software, err := s.DB.Software(v.VIN)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "software"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"charges": charges, "drains": drains, "battery": battery, "software": software,
		"note": "Charges, parked drain, and the battery line fill in from readings and the car's stream. Tesla does not send the old TeslaFi history.",
	})
}

func (s *Server) teslaAlerts(w http.ResponseWriter, r *http.Request) {
	sess, v, ok := s.liveCar(w, r)
	if !ok {
		return
	}
	if sess.Demo {
		writeJSON(w, http.StatusOK, map[string]any{"alerts": []model.CarAlert{}, "problem": "Sign in with Tesla to ask the car for alerts."})
		return
	}
	refresh := r.URL.Query().Get("refresh") == "1"
	if refresh {
		if problem := s.pullTesla(r, v.VIN, "/recent_alerts", func(body []byte) error {
			return s.DB.SaveCarAlerts(v.VIN, fleet.ParseRecentAlerts(body))
		}); problem != "" {
			list, _ := s.DB.CarAlerts(v.VIN)
			writeJSON(w, http.StatusOK, map[string]any{"alerts": emptyAlerts(list), "problem": problem})
			return
		}
	}
	list, err := s.DB.CarAlerts(v.VIN)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "alerts"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": emptyAlerts(list), "problem": "", "note": teslaReadNote})
}

func (s *Server) softwareNotes(w http.ResponseWriter, r *http.Request) {
	sess, v, ok := s.liveCar(w, r)
	if !ok {
		return
	}
	if sess.Demo {
		list, _ := s.DB.Software(v.VIN)
		writeJSON(w, http.StatusOK, map[string]any{"software": emptySoftware(list), "problem": "Sign in with Tesla to ask for release notes."})
		return
	}
	if r.URL.Query().Get("refresh") == "1" {
		problem := s.pullTesla(r, v.VIN, "/release_notes", func(body []byte) error {
			version, notes := fleet.ParseReleaseNotes(body)
			if version == "" {
				version, _ = s.DB.LatestSoftware(v.VIN)
			}
			if version == "" && notes == "" {
				return nil
			}
			return s.DB.SaveSoftwareNotes(v.VIN, version, notes, time.Now())
		})
		list, _ := s.DB.Software(v.VIN)
		writeJSON(w, http.StatusOK, map[string]any{"software": emptySoftware(list), "problem": problem, "note": teslaReadNote})
		return
	}
	list, err := s.DB.Software(v.VIN)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "software"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"software": emptySoftware(list), "problem": ""})
}

func (s *Server) chargers(w http.ResponseWriter, r *http.Request) {
	s.cachedRead(w, r, "chargers", "/nearby_charging_sites", func(body []byte) any {
		return map[string]any{"sites": emptySites(fleet.ParseChargingSites(body)), "problem": "", "note": teslaReadNote}
	})
}

func (s *Server) serviceStatus(w http.ResponseWriter, r *http.Request) {
	s.cachedRead(w, r, "service", "/service_data", func(body []byte) any {
		facts := fleet.ParseService(body)
		if facts == nil {
			facts = []model.Fact{}
		}
		return map[string]any{"facts": facts, "problem": "", "note": teslaReadNote}
	})
}

func (s *Server) invites(w http.ResponseWriter, r *http.Request) {
	sess, v, ok := s.ownerCar(w, r)
	if !ok {
		return
	}
	if sess.Demo {
		writeJSON(w, http.StatusOK, map[string]any{"invites": []model.Invite{}, "problem": "Sign in with Tesla to see invites."})
		return
	}
	if r.URL.Query().Get("refresh") != "1" {
		if body, fresh, err := s.DB.Cached("invites:"+v.VIN, 10*time.Minute); err == nil && fresh {
			writeJSON(w, http.StatusOK, json.RawMessage(body))
			return
		}
		list := []model.Invite{}
		writeJSON(w, http.StatusOK, map[string]any{"invites": list, "problem": "", "note": "Tap Refresh to ask Tesla for invite links. A new link lets another Tesla account use this car for a day."})
		return
	}
	var page any
	problem := s.pullTesla(r, v.VIN, "/invitations", func(body []byte) error {
		invites := fleet.ParseInvites(body)
		if invites == nil {
			invites = []model.Invite{}
		}
		page = map[string]any{"invites": invites, "problem": "", "note": teslaReadNote}
		raw, err := json.Marshal(page)
		if err != nil {
			return err
		}
		return s.DB.SaveCache("invites:"+v.VIN, string(raw), time.Now())
	})
	if problem != "" {
		writeJSON(w, http.StatusOK, map[string]any{"invites": []model.Invite{}, "problem": problem})
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) {
	sess, v, ok := s.ownerCar(w, r)
	if !ok {
		return
	}
	if sess.Demo {
		writeJSON(w, http.StatusConflict, map[string]string{"message": "The sample car cannot invite anyone."})
		return
	}
	status, body, err := s.fleetDo(r, http.MethodPost, "/api/1/vehicles/"+url.PathEscape(v.VIN)+"/invitations", map[string]any{})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": "The horn could not reach Tesla."})
		return
	}
	if status >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": teslaOr(body, status)})
		return
	}
	invites := fleet.ParseInvites(body)
	_ = s.DB.SaveCache("invites:"+v.VIN, "", time.Time{})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "invites": invites, "message": "Tesla made a one-day invite. The link is on this screen. Anyone who opens it can use this car in the Tesla app."})
}

func (s *Server) revokeInvite(w http.ResponseWriter, r *http.Request) {
	sess, v, ok := s.ownerCar(w, r)
	if !ok {
		return
	}
	if sess.Demo {
		writeJSON(w, http.StatusConflict, map[string]string{"message": "The sample car has no invites."})
		return
	}
	id := url.PathEscape(r.PathValue("id"))
	status, body, err := s.fleetDo(r, http.MethodPost, "/api/1/vehicles/"+url.PathEscape(v.VIN)+"/invitations/"+id+"/revoke", map[string]any{})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": "The horn could not reach Tesla."})
		return
	}
	if status >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": teslaOr(body, status)})
		return
	}
	_ = s.DB.SaveCache("invites:"+v.VIN, "", time.Time{})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Tesla revoked that invite."})
}

func (s *Server) command(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.require(w, r)
	if !ok {
		return
	}
	if sess.Demo {
		writeJSON(w, http.StatusConflict, map[string]string{"message": "The sample car does not take commands."})
		return
	}
	var req commandBody
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Honk could not read that command."})
		return
	}
	payload, err := commandPayload(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	v, err := s.focus(false)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "No car yet."})
		return
	}
	if canWake(v.State) {
		writeJSON(w, http.StatusConflict, map[string]string{"message": "Tesla lists this car as " + v.State + ". Poke it once on Garage, then try again. Honk will not wake the car from this button."})
		return
	}
	if s.Signer == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"message": "The virtual-key private key is not loaded on the server."})
		return
	}
	if req.Name == "add_charge_schedule" || req.Name == "add_precondition_schedule" {
		sn, serr := s.DB.Snapshot(v.VIN)
		if serr != nil || sn.Lat == nil || sn.Lng == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"message": "Honk needs a reading with the car's location before it can save a schedule."})
			return
		}
		payload["lat"] = *sn.Lat
		payload["lon"] = *sn.Lng
		payload["days_of_week"] = "ALL"
		payload["enabled"] = true
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Honk could not build that command."})
		return
	}
	token, err := s.teslaToken(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": "The horn could not use the Tesla sign-in."})
		return
	}
	code, resp, err := s.Signer.PostCommand(r.Context(), token, v.VIN, req.Name, raw)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": "The horn could not sign that command."})
		return
	}
	if code < 500 {
		_ = s.DB.AddUsage(false, "command", 1)
	}
	if code >= 300 {
		writeJSON(w, code, map[string]string{"message": teslaOr(resp, code)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Tesla accepted it."})
}

type commandBody struct {
	Name                string   `json:"name"`
	On                  *bool    `json:"on"`
	Enable              *bool    `json:"enable"`
	Percent             *float64 `json:"percent"`
	DriverTemp          *float64 `json:"driverTemp"`
	PassengerTemp       *float64 `json:"passengerTemp"`
	WhichTrunk          string   `json:"whichTrunk"`
	Window              string   `json:"window"`
	Password            string   `json:"password"`
	Pin                 string   `json:"pin"`
	LimitMph            *float64 `json:"limitMph"`
	FanOnly             *bool    `json:"fanOnly"`
	StartMinutes        *float64 `json:"startMinutes"`
	EndMinutes          *float64 `json:"endMinutes"`
	PreconditionMinutes *float64 `json:"preconditionMinutes"`
}

func commandPayload(req commandBody) (map[string]any, error) {
	switch req.Name {
	case "auto_conditioning_start", "auto_conditioning_stop", "charge_start", "charge_stop", "door_lock", "door_unlock", "flash_lights", "honk_horn":
		return map[string]any{}, nil
	case "set_temps":
		if req.DriverTemp == nil || req.PassengerTemp == nil {
			return nil, errCommand("Set a cabin temperature.")
		}
		return map[string]any{"driver_temp": *req.DriverTemp, "passenger_temp": *req.PassengerTemp}, nil
	case "set_cabin_overheat_protection":
		on, err := needBool(req.On, "Say whether cabin overheat protection is on.")
		if err != nil {
			return nil, err
		}
		fan := false
		if req.FanOnly != nil {
			fan = *req.FanOnly
		}
		return map[string]any{"on": on, "fan_only": fan}, nil
	case "set_charge_limit":
		if req.Percent == nil || *req.Percent < 50 || *req.Percent > 100 {
			return nil, errCommand("The charge limit has to be from 50 to 100.")
		}
		return map[string]any{"percent": *req.Percent}, nil
	case "set_sentry_mode":
		on, err := needBool(req.On, "Say whether sentry is on.")
		if err != nil {
			return nil, err
		}
		return map[string]any{"on": on}, nil
	case "actuate_trunk":
		if req.WhichTrunk != "front" && req.WhichTrunk != "rear" {
			return nil, errCommand("Choose the front or rear trunk.")
		}
		return map[string]any{"which_trunk": req.WhichTrunk}, nil
	case "window_control":
		if req.Window != "vent" && req.Window != "close" {
			return nil, errCommand("Windows can vent or close.")
		}
		return map[string]any{"command": req.Window}, nil
	case "guest_mode":
		on, err := needBool(req.Enable, "Say whether guest mode is on.")
		if err != nil {
			return nil, err
		}
		return map[string]any{"enable": on}, nil
	case "set_valet_mode":
		on, err := needBool(req.On, "Say whether valet is on.")
		if err != nil {
			return nil, err
		}
		if on && !fourDigits(req.Password) {
			return nil, errCommand("Valet needs a 4-digit code. Honk does not save it.")
		}
		return map[string]any{"on": on, "password": req.Password}, nil
	case "speed_limit_activate", "speed_limit_deactivate":
		if !fourDigits(req.Pin) {
			return nil, errCommand("Speed limit mode needs the 4-digit PIN. Honk does not save it.")
		}
		return map[string]any{"pin": req.Pin}, nil
	case "speed_limit_set_limit":
		if req.LimitMph == nil || *req.LimitMph < 50 || *req.LimitMph > 90 {
			return nil, errCommand("The speed limit has to be from 50 to 90 mph.")
		}
		return map[string]any{"limit_mph": *req.LimitMph}, nil
	case "add_charge_schedule":
		if req.StartMinutes == nil || req.EndMinutes == nil {
			return nil, errCommand("Set a charge start and end.")
		}
		return map[string]any{
			"start_time": *req.StartMinutes, "end_time": *req.EndMinutes,
			"start_enabled": true, "end_enabled": true, "one_time": false,
		}, nil
	case "add_precondition_schedule":
		if req.PreconditionMinutes == nil {
			return nil, errCommand("Set the time the cabin should be ready.")
		}
		return map[string]any{"precondition_time": *req.PreconditionMinutes, "one_time": false}, nil
	default:
		return nil, errCommand("That command is not available.")
	}
}

func needBool(v *bool, msg string) (bool, error) {
	if v == nil {
		return false, errCommand(msg)
	}
	return *v, nil
}

func fourDigits(v string) bool {
	if len(v) != 4 {
		return false
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

type commandError string

func (e commandError) Error() string { return string(e) }

func errCommand(msg string) error { return commandError(msg) }

func (s *Server) liveCar(w http.ResponseWriter, r *http.Request) (store.Session, model.Vehicle, bool) {
	session, ok := s.require(w, r)
	if !ok {
		return store.Session{}, model.Vehicle{}, false
	}
	vehicle, err := s.focus(session.Demo)
	if err != nil {
		writeJSON(w, http.StatusOK, emptyRead("No car yet."))
		return store.Session{}, model.Vehicle{}, false
	}
	return session, vehicle, true
}

func (s *Server) ownerCar(w http.ResponseWriter, r *http.Request) (store.Session, model.Vehicle, bool) {
	sess, v, ok := s.liveCar(w, r)
	if !ok {
		return sess, v, false
	}
	if sess.Demo {
		return sess, v, true
	}
	s.rememberAccess(r.Context())
	if fresh, err := s.DB.Vehicle(v.VIN); err == nil {
		v = fresh
	}
	if msg, blocked := driverShareProblem(v.Access); blocked {
		writeJSON(w, http.StatusOK, map[string]any{"invites": []model.Invite{}, "problem": msg})
		return sess, v, false
	}
	return sess, v, true
}

func (s *Server) cachedRead(w http.ResponseWriter, r *http.Request, kind, suffix string, build func([]byte) any) {
	sess, v, ok := s.liveCar(w, r)
	if !ok {
		return
	}
	if sess.Demo {
		writeJSON(w, http.StatusOK, map[string]any{"problem": "Sign in with Tesla to ask the car.", "sites": []model.ChargerSite{}, "facts": []model.Fact{}})
		return
	}
	key := kind + ":" + v.VIN
	refresh := r.URL.Query().Get("refresh") == "1"
	if !refresh {
		if body, fresh, err := s.DB.Cached(key, 10*time.Minute); err == nil && fresh && body != "" {
			writeJSON(w, http.StatusOK, json.RawMessage(body))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"sites": []model.ChargerSite{}, "facts": []model.Fact{}, "problem": "",
			"note": "Tap Ask Tesla once. Honk will not poll for this.",
		})
		return
	}
	var page any
	problem := s.pullTesla(r, v.VIN, suffix, func(body []byte) error {
		page = build(body)
		raw, err := json.Marshal(page)
		if err != nil {
			return err
		}
		return s.DB.SaveCache(key, string(raw), time.Now())
	})
	if problem != "" {
		writeJSON(w, http.StatusOK, map[string]any{"sites": []model.ChargerSite{}, "facts": []model.Fact{}, "problem": problem})
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) pullTesla(r *http.Request, vin, suffix string, save func([]byte) error) string {
	status, body, err := s.fleetDo(r, http.MethodGet, "/api/1/vehicles/"+url.PathEscape(vin)+suffix, nil)
	if err != nil {
		return "The horn could not reach Tesla."
	}
	if status >= 300 {
		return teslaOr(body, status)
	}
	if err := save(body); err != nil {
		return "Honk read Tesla but could not store it."
	}
	return ""
}

func (s *Server) fleetDo(r *http.Request, method, path string, body any) (int, []byte, error) {
	if s.Fleet == nil {
		return 0, nil, errCommand("Tesla is not connected on this server.")
	}
	status, resp, err := s.Fleet.Do(r.Context(), method, path, body)
	if err == nil && status < 500 {
		_ = s.DB.AddUsage(false, "metadata", 1)
	}
	return status, resp, err
}

func teslaOr(body []byte, status int) string {
	if msg := fleet.TeslaMessage(body); msg != "" {
		return msg
	}
	return "Tesla returned " + strconv.Itoa(status) + "."
}

func emptyRead(problem string) map[string]any {
	return map[string]any{
		"problem": problem, "note": "",
		"charges": []model.ChargeView{}, "drains": []model.DrainView{},
		"battery": []model.BatteryView{}, "software": []model.SoftwareView{},
		"alerts": []model.CarAlert{}, "sites": []model.ChargerSite{},
		"facts": []model.Fact{}, "invites": []model.Invite{},
	}
}

func emptyAlerts(in []model.CarAlert) []model.CarAlert {
	if in == nil {
		return []model.CarAlert{}
	}
	return in
}

func emptySoftware(in []model.SoftwareView) []model.SoftwareView {
	if in == nil {
		return []model.SoftwareView{}
	}
	return in
}

func emptySites(in []model.ChargerSite) []model.ChargerSite {
	if in == nil {
		return []model.ChargerSite{}
	}
	return in
}
