package fleet

import (
	"encoding/json"
	"fmt"
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
	access, _ := m["access_type"].(string)
	return Vehicle{VIN: vin, Name: name, State: state, Access: access}, true
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
		if _, present := charge["charge_energy_added"]; present {
			n := number(charge["charge_energy_added"])
			up.EnergyAddedKwh = &n
		}
		if _, present := charge["energy_remaining"]; present {
			n := number(charge["energy_remaining"])
			up.EnergyRemainingKwh = &n
		}
		if _, present := charge["battery_range"]; present {
			n := number(charge["battery_range"])
			up.RatedRangeMi = &n
		}
		if b, ok := charge["fast_charger_present"].(bool); ok {
			up.Fast = &b
		}
		if s, ok := charge["fast_charger_type"].(string); ok {
			up.FastType = s
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
		if g, ok := vs["guest_mode"].(bool); ok {
			up.Guest = &g
		}
		if ver := softwareVersion(vs); ver != "" {
			up.Software = ver
		}
	}
	up.Facts = readingFacts(resp)
	return up, vehicle
}

func readingFacts(resp map[string]any) []model.Fact {
	var facts []model.Fact
	add := func(label, value string) {
		if strings.TrimSpace(value) != "" {
			facts = append(facts, model.Fact{Label: label, Value: value})
		}
	}
	vs, _ := resp["vehicle_state"].(map[string]any)
	charge, _ := resp["charge_state"].(map[string]any)
	climate, _ := resp["climate_state"].(map[string]any)
	drive, _ := resp["drive_state"].(map[string]any)
	if vs != nil {
		add("Software", softwareVersion(vs))
		if _, ok := vs["odometer"]; ok {
			add("Odometer", fmt.Sprintf("%.0f mi", number(vs["odometer"])))
		}
		if b, ok := vs["sentry_mode"].(bool); ok {
			add("Sentry", onOff(b))
		}
		if b, ok := vs["valet_mode"].(bool); ok {
			add("Valet", onOff(b))
		}
		if mode, ok := vs["speed_limit_mode"].(map[string]any); ok {
			if active, ok := mode["active"].(bool); ok {
				if active {
					add("Speed limit mode", fmt.Sprintf("On, %.0f mph", number(mode["current_limit_mph"])))
				} else {
					add("Speed limit mode", "Off")
				}
			}
		}
		if b, ok := vs["guest_mode"].(bool); ok {
			add("Guest mode", onOff(b))
		}
		add("Tires", tireLine(vs))
		add("Windows", windowLine(vs))
	}
	if charge != nil {
		if s, ok := charge["charging_state"].(string); ok {
			add("Charging", s)
		}
		if _, ok := charge["charge_limit_soc"]; ok {
			add("Charge limit", fmt.Sprintf("%.0f%%", number(charge["charge_limit_soc"])))
		}
		if _, ok := charge["charger_power"]; ok && number(charge["charger_power"]) > 0 {
			add("Charger", fmt.Sprintf("%.0f kW", number(charge["charger_power"])))
		}
		if _, ok := charge["minutes_to_full_charge"]; ok && number(charge["minutes_to_full_charge"]) > 0 {
			add("Time to full", fmt.Sprintf("%.0f min", number(charge["minutes_to_full_charge"])))
		}
	}
	if climate != nil {
		if _, ok := climate["inside_temp"]; ok {
			add("Cabin", fahrenheit(number(climate["inside_temp"])))
		}
		if _, ok := climate["outside_temp"]; ok {
			add("Outside", fahrenheit(number(climate["outside_temp"])))
		}
		if b, ok := climate["is_climate_on"].(bool); ok {
			add("Climate", onOff(b))
		}
	}
	if drive != nil {
		if _, ok := drive["heading"]; ok {
			add("Heading", fmt.Sprintf("%.0f°", number(drive["heading"])))
		}
		if _, ok := drive["power"]; ok {
			add("Power", fmt.Sprintf("%.0f kW", number(drive["power"])))
		}
	}
	return facts
}

func softwareVersion(vs map[string]any) string {
	if su, ok := vs["software_update"].(map[string]any); ok {
		if ver, ok := su["version"].(string); ok && ver != "" {
			return ver
		}
	}
	if ver, ok := vs["car_version"].(string); ok {
		return ver
	}
	return ""
}

func tireLine(vs map[string]any) string {
	order := []struct {
		key, label string
	}{
		{"tpms_pressure_fl", "FL"},
		{"tpms_pressure_fr", "FR"},
		{"tpms_pressure_rl", "RL"},
		{"tpms_pressure_rr", "RR"},
	}
	var parts []string
	for _, item := range order {
		if _, ok := vs[item.key]; !ok {
			continue
		}
		psi := number(vs[item.key])
		if psi > 0 && psi < 10 {
			psi *= 14.5038
		}
		if psi <= 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %.0f", item.label, psi))
	}
	return strings.Join(parts, " · ")
}

func windowLine(vs map[string]any) string {
	order := []struct {
		key, label string
	}{
		{"fd_window", "driver front"},
		{"fp_window", "passenger front"},
		{"rd_window", "driver rear"},
		{"rp_window", "passenger rear"},
	}
	var open []string
	seen := false
	for _, item := range order {
		if _, ok := vs[item.key]; !ok {
			continue
		}
		seen = true
		if number(vs[item.key]) != 0 {
			open = append(open, item.label)
		}
	}
	if !seen {
		return ""
	}
	if len(open) == 0 {
		return "Closed"
	}
	return "Open: " + strings.Join(open, ", ")
}

func fahrenheit(celsius float64) string {
	return fmt.Sprintf("%.0f°F", celsius*9/5+32)
}

func onOff(v bool) string {
	if v {
		return "On"
	}
	return "Off"
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
		detail := "Allowed on this car. This is not the person in the seat."
		if ga, ok := m["granular_access"].(map[string]any); ok {
			if hide, _ := ga["hide_private"].(bool); hide {
				detail = "Allowed on this car. Tesla hides private data, including location, from this share. This is not the person in the seat."
			}
		}
		out = append(out, model.Driver{
			Name:   name,
			Detail: detail,
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
