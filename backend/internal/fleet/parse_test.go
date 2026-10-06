package fleet

import (
	"strings"
	"testing"
)

func TestParseDriversUsesFirstAndLastName(t *testing.T) {
	body := []byte(`{"response":[{"driver_first_name":"Ada","driver_last_name":"Lovelace","granular_access":{"hide_private":false}}]}`)
	got := ParseDrivers(body)
	if len(got) != 1 || got[0].Name != "Ada Lovelace" || strings.Contains(got[0].Detail, "hides private") {
		t.Fatal(got)
	}
}

func TestReadingFactsFromVehicleData(t *testing.T) {
	raw := map[string]any{
		"response": map[string]any{
			"vin":   "5YJTEST",
			"state": "online",
			"drive_state": map[string]any{
				"speed": nil, "shift_state": nil, "heading": 90.0, "power": 0.0,
			},
			"charge_state": map[string]any{
				"battery_level": 80.0, "charging_state": "Disconnected", "charge_limit_soc": 70.0,
			},
			"climate_state": map[string]any{"inside_temp": 22.0, "is_climate_on": false},
			"vehicle_state": map[string]any{
				"car_version": "2026.20.1", "odometer": 12000.0, "locked": true, "sentry_mode": true,
				"speed_limit_mode": map[string]any{"active": true, "current_limit_mph": 70.0},
				"tpms_pressure_fl": 2.9, "fd_window": 0.0, "fp_window": 0.0, "rd_window": 0.0, "rp_window": 0.0,
			},
		},
	}
	up, vehicle := ParseVehicleData("5YJTEST", raw)
	if vehicle.VIN != "5YJTEST" || up.Soc == nil || *up.Soc != 80 {
		t.Fatalf("parse %+v %+v", vehicle, up)
	}
	joined := ""
	for _, fact := range up.Facts {
		joined += fact.Label + "=" + fact.Value + "\n"
	}
	for _, want := range []string{"Software=2026.20.1", "Sentry=On", "Speed limit mode=On, 70 mph", "Charge limit=70%", "Cabin=72°F", "Odometer=12000 mi"} {
		if !strings.Contains(joined, want) {
			t.Fatal(joined)
		}
	}
}
