package fleet

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/model"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/telemetry"
)

func vehiclesFrom(raw map[string]any) []Vehicle {
	resp, _ := raw["response"].([]any)
	if resp == nil {
		if one, ok := raw["response"].(map[string]any); ok {
			if v, ok := mapVehicle(one); ok {
				return []Vehicle{v}
			}
		}
		return nil
	}
	var out []Vehicle
	for _, item := range resp {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if v, ok := mapVehicle(m); ok {
			out = append(out, v)
		}
	}
	return out
}

func oneVehicle(raw map[string]any) (Vehicle, bool) {
	m, ok := raw["response"].(map[string]any)
	if !ok {
		return Vehicle{}, false
	}
	return mapVehicle(m)
}

func mapVehicle(m map[string]any) (Vehicle, bool) {
	vin, _ := m["vin"].(string)
	if vin == "" {
		return Vehicle{}, false
	}
	name, _ := m["display_name"].(string)
	if name == "" {
		name, _ = m["vehicle_name"].(string)
	}
	if name == "" {
		name = vin
	}
	state, _ := m["state"].(string)
	if state == "" {
		state = "offline"
	}
	return Vehicle{VIN: vin, Name: name, State: state}, true
}

// ParseVehicleData reads one vehicle_data payload.
// A null shift_state is Park. A null speed is 0.
// is_user_present is ignored. It is not the driver name and it is not DriverSeatOccupied.
func ParseVehicleData(vin string, raw map[string]any) (telemetry.Update, Vehicle) {
	resp, _ := raw["response"].(map[string]any)
	if resp == nil {
		resp = raw
	}
	vehicle, _ := mapVehicle(resp)
	if vehicle.VIN == "" {
		vehicle.VIN = vin
	}
	up := telemetry.Update{VIN: vehicle.VIN, At: time.Now().UTC(), SignalCount: 1}
	if drive, ok := resp["drive_state"].(map[string]any); ok {
		if _, present := drive["speed"]; present {
			n := number(drive["speed"])
			up.SpeedMph = &n
		}
		if _, present := drive["shift_state"]; present {
			gear := "P"
			if s, ok := drive["shift_state"].(string); ok && s != "" {
				gear = s
			}
			up.Gear = &gear
		}
		if lat, lng, ok := pair(drive["latitude"], drive["longitude"]); ok {
			up.Lat = &lat
			up.Lng = &lng
		}
	}
	if charge, ok := resp["charge_state"].(map[string]any); ok {
		if _, present := charge["battery_level"]; present {
			n := number(charge["battery_level"])
			up.Soc = &n
		}
		if _, present := charge["est_battery_range"]; present {
			n := number(charge["est_battery_range"])
			up.RangeMi = &n
		} else if _, present := charge["battery_range"]; present {
			n := number(charge["battery_range"])
			up.RangeMi = &n
		}
		if s, ok := charge["charging_state"].(string); ok && s != "" {
			up.Charge = &s
		}
	}
	if vs, ok := resp["vehicle_state"].(map[string]any); ok {
		if b, ok := vs["locked"].(bool); ok {
			up.Locked = &b
		}
		open, summary := doorsFromVehicleState(vs)
		up.DoorsOpen = &open
		up.DoorSummary = &summary
		if _, present := vs["odometer"]; present {
			n := number(vs["odometer"])
			up.Odometer = &n
		}
		if name, ok := vs["vehicle_name"].(string); ok && name != "" {
			vehicle.Name = name
		}
	}
	return up, vehicle
}

func doorsFromVehicleState(vs map[string]any) (bool, string) {
	labels := []struct {
		key   string
		label string
	}{
		{"df", "driver front"},
		{"dr", "driver rear"},
		{"pf", "passenger front"},
		{"pr", "passenger rear"},
		{"ft", "front trunk"},
		{"rt", "rear trunk"},
	}
	var open []string
	for _, item := range labels {
		if _, ok := vs[item.key]; !ok {
			continue
		}
		if number(vs[item.key]) != 0 {
			open = append(open, item.label)
		}
	}
	if len(open) == 0 {
		return false, "All closed"
	}
	return true, "Open: " + strings.Join(open, ", ")
}

func ParseDrivers(body []byte) []model.Driver {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}
	var items []any
	switch resp := raw["response"].(type) {
	case []any:
		items = resp
	case map[string]any:
		if list, ok := resp["drivers"].([]any); ok {
			items = list
		}
	}
	var out []model.Driver
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(joinName(m))
		if name == "" {
			if id, ok := m["user_id"]; ok {
				name = "Driver " + stringify(id)
			} else {
				name = "Unnamed driver"
			}
		}
		out = append(out, model.Driver{
			Name:   name,
			Detail: "Allowed driver from Tesla. This is not the person in the seat.",
		})
	}
	return out
}

func joinName(m map[string]any) string {
	if s, ok := m["name"].(string); ok && s != "" {
		return s
	}
	if s, ok := m["display_name"].(string); ok && s != "" {
		return s
	}
	first, _ := m["driver_first_name"].(string)
	last, _ := m["driver_last_name"].(string)
	return strings.TrimSpace(first + " " + last)
}

func stringify(v any) string {
	b, _ := json.Marshal(v)
	return strings.Trim(string(b), `"`)
}

func number(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

func pair(lat, lng any) (float64, float64, bool) {
	if lat == nil || lng == nil {
		return 0, 0, false
	}
	la, lo := number(lat), number(lng)
	if la == 0 && lo == 0 {
		return 0, 0, false
	}
	return la, lo, true
}
