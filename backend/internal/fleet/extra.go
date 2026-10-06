package fleet

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/model"
)

func TeslaMessage(body []byte) string {
	var raw map[string]any
	if json.Unmarshal(body, &raw) != nil {
		return ""
	}
	if desc, ok := raw["error_description"].(string); ok && strings.TrimSpace(desc) != "" {
		return desc
	}
	if err, ok := raw["error"].(string); ok && strings.TrimSpace(err) != "" && err != "null" {
		return err
	}
	return ""
}

func ParseRecentAlerts(body []byte) []model.CarAlert {
	items := responseList(body, "recent_alerts")
	var out []model.CarAlert
	for _, item := range items {
		name := firstString(item, "name", "alert_name")
		if name == "" {
			continue
		}
		when := firstString(item, "time", "started_at", "created_at")
		parsed, err := time.Parse(time.RFC3339, when)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339Nano, when)
		}
		if err != nil {
			continue
		}
		audience := ""
		if list, ok := item["audiences"].([]any); ok {
			var parts []string
			for _, one := range list {
				if s, ok := one.(string); ok && s != "" {
					parts = append(parts, s)
				}
			}
			audience = strings.Join(parts, ", ")
		}
		if audience == "" {
			audience = firstString(item, "audience")
		}
		out = append(out, model.CarAlert{
			Name: name, At: model.APITime(parsed.UTC()), Audience: audience,
			Detail: firstString(item, "description", "user_text", "message"),
		})
	}
	return out
}

func ParseReleaseNotes(body []byte) (string, string) {
	raw := responseObject(body)
	version := firstString(raw, "version", "current_version", "car_version")
	var notes []string
	for _, key := range []string{"release_notes", "notes"} {
		list, _ := raw[key].([]any)
		for _, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			title := firstString(m, "title", "subtitle")
			desc := firstString(m, "description", "customer_version", "html")
			desc = stripTags(desc)
			line := strings.TrimSpace(strings.TrimSpace(title) + "\n" + strings.TrimSpace(desc))
			if line != "" {
				notes = append(notes, line)
			}
		}
	}
	text := strings.Join(notes, "\n\n")
	if len(text) > 4000 {
		text = text[:4000]
	}
	return version, text
}

func ParseChargingSites(body []byte) []model.ChargerSite {
	raw := responseObject(body)
	var out []model.ChargerSite
	out = append(out, sitesFrom(raw["superchargers"], "Supercharger")...)
	out = append(out, sitesFrom(raw["destination_charging"], "Destination")...)
	return out
}

func sitesFrom(raw any, kind string) []model.ChargerSite {
	list, _ := raw.([]any)
	var out []model.ChargerSite
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := firstString(m, "name", "title")
		if name == "" {
			continue
		}
		site := model.ChargerSite{Name: name, Kind: kind}
		if _, ok := m["distance_miles"]; ok {
			n := number(m["distance_miles"])
			site.DistanceMiles = &n
		}
		if _, ok := m["available_stalls"]; ok {
			n := int(number(m["available_stalls"]))
			site.Available = &n
		}
		if _, ok := m["total_stalls"]; ok {
			n := int(number(m["total_stalls"]))
			site.Stalls = &n
		}
		if b, ok := m["site_closed"].(bool); ok {
			site.Closed = b
		}
		out = append(out, site)
	}
	return out
}

func ParseService(body []byte) []model.Fact {
	raw := responseObject(body)
	var facts []model.Fact
	for _, key := range []string{"service_status", "status", "service_visit_status", "service_etc", "etc"} {
		if s := firstString(raw, key); s != "" {
			facts = append(facts, model.Fact{Label: labelize(key), Value: s})
		}
	}
	if len(facts) > 0 {
		return facts
	}
	for k, v := range raw {
		switch n := v.(type) {
		case string:
			if strings.TrimSpace(n) != "" {
				facts = append(facts, model.Fact{Label: labelize(k), Value: n})
			}
		case bool:
			facts = append(facts, model.Fact{Label: labelize(k), Value: strconv.FormatBool(n)})
		case float64:
			facts = append(facts, model.Fact{Label: labelize(k), Value: fmt.Sprintf("%g", n)})
		}
	}
	return facts
}

func ParseInvites(body []byte) []model.Invite {
	items := responseList(body, "invitations")
	if len(items) == 0 {
		if one := responseObject(body); len(one) > 0 && (firstString(one, "invitation_id", "id", "share_id") != "" || firstString(one, "share_link", "invite_url", "url") != "") {
			items = []map[string]any{one}
		}
	}
	var out []model.Invite
	for _, item := range items {
		id := firstString(item, "invitation_id", "id", "share_id")
		if id == "" {
			if n, ok := item["invitation_id"].(float64); ok {
				id = strconv.FormatInt(int64(n), 10)
			} else if n, ok := item["id"].(float64); ok {
				id = strconv.FormatInt(int64(n), 10)
			}
		}
		link := firstString(item, "share_link", "invite_url", "url", "invitation_url")
		if id == "" && link == "" {
			continue
		}
		if id == "" {
			id = link
		}
		out = append(out, model.Invite{
			ID: id, State: firstString(item, "state", "status"),
			ExpiresAt: firstString(item, "expires_at", "expiration"), Link: link,
		})
	}
	return out
}

func responseObject(body []byte) map[string]any {
	var raw map[string]any
	if json.Unmarshal(body, &raw) != nil {
		return map[string]any{}
	}
	if resp, ok := raw["response"].(map[string]any); ok {
		return resp
	}
	return raw
}

func responseList(body []byte, key string) []map[string]any {
	var raw map[string]any
	if json.Unmarshal(body, &raw) != nil {
		return nil
	}
	var list []any
	switch resp := raw["response"].(type) {
	case []any:
		list = resp
	case map[string]any:
		list, _ = resp[key].([]any)
		if list == nil {
			for _, v := range resp {
				if nested, ok := v.([]any); ok {
					list = nested
					break
				}
			}
		}
	}
	var out []map[string]any
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		switch v := m[key].(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return v
			}
		case float64:
			return strconv.FormatInt(int64(v), 10)
		}
	}
	return ""
}

func labelize(key string) string {
	key = strings.ReplaceAll(key, "_", " ")
	if key == "" {
		return key
	}
	return strings.ToUpper(key[:1]) + key[1:]
}

func stripTags(s string) string {
	var b strings.Builder
	skip := false
	for _, r := range s {
		switch {
		case r == '<':
			skip = true
		case r == '>':
			skip = false
		case !skip:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
