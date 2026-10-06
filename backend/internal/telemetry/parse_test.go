package telemetry

import "testing"

func TestParseDispatcherRecord(t *testing.T) {
	body := []byte(`{
	  "createdAt": "2026-10-06T02:46:51Z",
	  "vin": "5YJTESTVIN0000001",
	  "data": [
	    {"key": "VehicleSpeed", "value": {"stringValue": "82"}},
	    {"key": "Location", "value": {"locationValue": {"latitude": 34.15, "longitude": -118.25}}},
	    {"key": "Gear", "value": {"shiftStateValue": "ShiftStateD"}},
	    {"key": "DriverSeatOccupied", "value": {"booleanValue": true}},
	    {"key": "GuestModeEnabled", "value": {"booleanValue": false}},
	    {"key": "Locked", "value": {"booleanValue": true}},
	    {"key": "Soc", "value": {"doubleValue": 71}},
	    {"key": "EstBatteryRange", "value": {"stringValue": "210"}},
	    {"key": "ChargeState", "value": {"stringValue": "Disconnected"}},
	    {"key": "DoorState", "value": {"doorValue": {"DriverFront": false, "PassengerFront": true}}}
	  ]
	}`)
	got, err := Parse(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len %d", len(got))
	}
	u := got[0]
	if u.VIN != "5YJTESTVIN0000001" || u.SpeedMph == nil || *u.SpeedMph != 82 {
		t.Fatalf("speed %#v", u.SpeedMph)
	}
	if u.Gear == nil || *u.Gear != "ShiftStateD" {
		t.Fatal(u.Gear)
	}
	if u.Seat == nil || !*u.Seat || u.Guest == nil || *u.Guest {
		t.Fatalf("seat/guest %#v %#v", u.Seat, u.Guest)
	}
	if u.DoorsOpen == nil || !*u.DoorsOpen || u.DoorSummary == nil || *u.DoorSummary != "Open: passenger front" {
		t.Fatalf("doors %#v %#v", u.DoorsOpen, u.DoorSummary)
	}
	if u.SignalCount != 10 {
		t.Fatalf("signals %d", u.SignalCount)
	}
}

func TestParseFlatAndInvalid(t *testing.T) {
	body := []byte(`{"vin":"5YJTESTVIN0000001","signals":{"VehicleSpeed":0,"Gear":"P","GuestModeEnabled":true}}`)
	got, err := Parse(body)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Guest == nil || !*got[0].Guest {
		t.Fatal(got[0].Guest)
	}
	invalid := []byte(`{"vin":"5YJTESTVIN0000001","data":[{"key":"VehicleSpeed","value":{"invalid":true}}]}`)
	got, err = Parse(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].SpeedMph != nil {
		t.Fatal("invalid speed should be ignored")
	}
}
