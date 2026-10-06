package telemetry

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Update is a partial snapshot. Nil fields are left alone.
type Update struct {
	At           time.Time
	VIN          string
	SpeedMph     *float64
	Gear         *string
	Lat          *float64
	Lng          *float64
	Soc          *float64
	RangeMi      *float64
	Charge       *string
	DoorsOpen    *bool
	DoorSummary  *string
	Locked       *bool
	Seat         *bool
	Guest        *bool
	Odometer     *float64
	SignalCount  int
	Online       *bool
}

type record struct {
	CreatedAt string          `json:"createdAt"`
	Created   string          `json:"created_at"`
	VIN       string          `json:"vin"`
	Data      []datum         `json:"data"`
	Signals   map[string]any  `json:"signals"`
}

type datum struct {
	Key   string         `json:"key"`
	Value map[string]any `json:"value"`
}

// Parse accepts a Fleet Telemetry JSON record, a batch, a flat signal map,
// or a connectivity event.
func Parse(body []byte) ([]Update, error) {
	body = bytesTrim(body)
	if len(body) == 0 {
		return nil, fmt.Errorf("empty telemetry body")
	}
	var batch struct {
		Records []json.RawMessage `json:"records"`
	}
	if json.Unmarshal(body, &batch) == nil && len(batch.Records) > 0 {
		var out []Update
		for _, raw := range batch.Records {
			one, err := Parse(raw)
			if err != nil {
				return nil, err
			}
			out = append(out, one...)
		}
		return out, nil
	}
	var rec record
	if err := json.Unmarshal(body, &rec); err != nil {
		return nil, err
	}
	up := Update{VIN: rec.VIN, At: time.Now().UTC()}
	if ts := first(rec.CreatedAt, rec.Created); ts != "" {
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			up.At = t
		} else if t, err := time.Parse(time.RFC3339, ts); err == nil {
			up.At = t
		}
	}
	for _, d := range rec.Data {
		if d.Value != nil && truthyInvalid(d.Value["invalid"]) {
			continue
		}
		applySignal(&up, d.Key, signalValue(d.Value))
	}
	for k, v := range rec.Signals {
		applySignal(&up, k, v)
	}
	var conn struct {
		Status     string `json:"status"`
		Connection string `json:"connection"`
		VIN        string `json:"vin"`
	}
	_ = json.Unmarshal(body, &conn)
	if up.VIN == "" {
		up.VIN = conn.VIN
	}
	switch strings.ToUpper(first(conn.Status, conn.Connection)) {
	case "CONNECTED", "ONLINE":
		v := true
		up.Online = &v
	case "DISCONNECTED", "OFFLINE", "ASLEEP":
		v := false
		up.Online = &v
	}
	if up.VIN == "" && up.SignalCount == 0 && up.Online == nil {
		return nil, fmt.Errorf("telemetry record had no vin or signals")
	}
	return []Update{up}, nil
}

func applySignal(up *Update, key string, value any) {
	if value == nil {
		return
	}
	switch key {
	case "VehicleSpeed":
		if n, ok := asFloat(value); ok {
			up.SpeedMph = &n
			up.SignalCount++
		}
	case "Gear":
		if s, ok := asString(value); ok {
			up.Gear = &s
			up.SignalCount++
		}
	case "Location":
		lat, lng, ok := asLocation(value)
		if ok {
			up.Lat = &lat
			up.Lng = &lng
			up.SignalCount++
		}
	case "Soc":
		if n, ok := asFloat(value); ok {
			up.Soc = &n
			up.SignalCount++
		}
	case "EstBatteryRange":
		if n, ok := asFloat(value); ok {
			up.RangeMi = &n
			up.SignalCount++
		}
	case "ChargeState", "DetailedChargeState":
		if s, ok := asString(value); ok {
			label := cleanCharge(s)
			up.Charge = &label
			up.SignalCount++
		}
	case "DoorState":
		open, summary, ok := asDoors(value)
		if ok {
			up.DoorsOpen = &open
			up.DoorSummary = &summary
			up.SignalCount++
		}
	case "Locked":
		if b, ok := asBool(value); ok {
			up.Locked = &b
			up.SignalCount++
		}
	case "DriverSeatOccupied":
		if b, ok := asBool(value); ok {
			up.Seat = &b
			up.SignalCount++
		}
	case "GuestModeEnabled":
		if b, ok := asBool(value); ok {
			up.Guest = &b
			up.SignalCount++
		}
	case "Odometer":
		if n, ok := asFloat(value); ok {
			up.Odometer = &n
			up.SignalCount++
		}
	}
}

func signalValue(v map[string]any) any {
	if v == nil {
		return nil
	}
	for _, k := range []string{
		"stringValue", "string_value",
		"doubleValue", "double_value",
		"floatValue", "float_value",
		"intValue", "int_value",
		"longValue", "long_value",
		"booleanValue", "boolean_value",
		"locationValue", "location_value",
		"shiftStateValue", "shift_state_value",
		"doorValue", "door_value",
		"chargingValue", "charging_value",
	} {
		if raw, ok := v[k]; ok {
			return raw
		}
	}
	return v
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	case bool:
		return 0, false
	default:
		return 0, false
	}
}

func asBool(v any) (bool, bool) {
	switch n := v.(type) {
	case bool:
		return n, true
	case string:
		switch strings.ToLower(strings.TrimSpace(n)) {
		case "true", "1", "yes", "on":
			return true, true
		case "false", "0", "no", "off":
			return false, true
		}
	case float64:
		return n != 0, true
	}
	return false, false
}

func asString(v any) (string, bool) {
	switch n := v.(type) {
	case string:
		return n, true
	case float64, bool:
		return fmt.Sprint(n), true
	default:
		if n == nil {
			return "", false
		}
		b, err := json.Marshal(n)
		if err != nil {
			return "", false
		}
		return string(b), true
	}
}

func asLocation(v any) (float64, float64, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return 0, 0, false
	}
	lat, ok1 := asFloat(pick(m, "latitude", "lat"))
	lng, ok2 := asFloat(pick(m, "longitude", "lng", "lon"))
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	if lat == 0 && lng == 0 {
		return 0, 0, false
	}
	return lat, lng, true
}

func asDoors(v any) (bool, string, bool) {
	if s, ok := v.(string); ok {
		open := !strings.Contains(strings.ToLower(s), "closed") && s != ""
		if strings.EqualFold(s, "all closed") || strings.EqualFold(s, "closed") {
			open = false
		}
		return open, s, true
	}
	m, ok := v.(map[string]any)
	if !ok {
		return false, "", false
	}
	names := []struct {
		keys  []string
		label string
	}{
		{[]string{"DriverFront", "driverFront", "df", "driver_front"}, "driver front"},
		{[]string{"DriverRear", "driverRear", "dr", "driver_rear"}, "driver rear"},
		{[]string{"PassengerFront", "passengerFront", "pf", "passenger_front"}, "passenger front"},
		{[]string{"PassengerRear", "passengerRear", "pr", "passenger_rear"}, "passenger rear"},
		{[]string{"TrunkFront", "trunkFront", "ft", "front_trunk"}, "front trunk"},
		{[]string{"TrunkRear", "trunkRear", "rt", "rear_trunk"}, "rear trunk"},
	}
	var openNames []string
	seen := false
	for _, n := range names {
		raw := pick(m, n.keys...)
		if raw == nil {
			continue
		}
		seen = true
		if b, ok := asBool(raw); ok && b {
			openNames = append(openNames, n.label)
		}
	}
	if !seen {
		return false, "", false
	}
	if len(openNames) == 0 {
		return false, "All closed", true
	}
	return true, "Open: " + strings.Join(openNames, ", "), true
}

func cleanCharge(s string) string {
	s = strings.TrimPrefix(s, "DetailedChargeState")
	s = strings.TrimPrefix(s, "ChargeState")
	s = strings.TrimPrefix(s, "ChargingState")
	if s == "" {
		return "Unknown"
	}
	return s
}

func pick(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

func truthyInvalid(v any) bool {
	b, ok := asBool(v)
	return ok && b
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
