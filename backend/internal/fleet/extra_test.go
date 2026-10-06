package fleet

import "testing"

func TestParseRecentAlertsAndNotes(t *testing.T) {
	alerts := ParseRecentAlerts([]byte(`{"response":[{"name":"TPMS","time":"2026-10-06T15:04:05Z","audiences":["customer"],"user_text":"Tire pressure"}]}`))
	if len(alerts) != 1 || alerts[0].Name != "TPMS" || alerts[0].Detail != "Tire pressure" {
		t.Fatal(alerts)
	}
	version, notes := ParseReleaseNotes([]byte(`{"response":{"version":"2026.26.3","release_notes":[{"title":"Lights","description":"<p>Brighter</p>"}]}}`))
	if version != "2026.26.3" || notes != "Lights\nBrighter" {
		t.Fatalf("%q %q", version, notes)
	}
}

func TestParseChargersServiceInvites(t *testing.T) {
	sites := ParseChargingSites([]byte(`{"response":{"superchargers":[{"name":"Baker","distance_miles":12.5,"available_stalls":4,"total_stalls":40,"site_closed":false}],"destination_charging":[{"name":"Inn","distance_miles":1}]}}`))
	if len(sites) != 2 || sites[0].Kind != "Supercharger" || sites[0].Name != "Baker" || sites[1].Kind != "Destination" {
		t.Fatal(sites)
	}
	facts := ParseService([]byte(`{"response":{"service_status":"in_service"}}`))
	if len(facts) != 1 || facts[0].Value != "in_service" {
		t.Fatal(facts)
	}
	invites := ParseInvites([]byte(`{"response":[{"invitation_id":42,"state":"pending","share_link":"https://tesla.example/invite"}]}`))
	if len(invites) != 1 || invites[0].ID != "42" || invites[0].Link == "" {
		t.Fatal(invites)
	}
}
