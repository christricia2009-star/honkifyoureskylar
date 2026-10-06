package alerts

import (
	"fmt"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/geo"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/model"
)

type Kind string

const (
	Speed     Kind = "speed"
	CurfewOn  Kind = "curfew_start"
	CurfewOff Kind = "curfew_end"
	LeftHome  Kind = "leave_home"
)

type Draft struct {
	Kind    Kind
	Message string
}

type Memory struct {
	SpeedOver  bool
	InsideHome *bool
}

// Evaluate looks at one new sample. tripEvent is "start", "end", or "".
func Evaluate(at time.Time, settings model.Settings, mem Memory, speed *float64, lat, lng *float64, tripEvent string) ([]Draft, Memory) {
	var out []Draft
	limit := settings.SpeedLimitMph
	if limit <= 0 {
		limit = 75
	}
	over := speed != nil && *speed > limit
	if over && !mem.SpeedOver {
		out = append(out, Draft{Kind: Speed, Message: speedLine(*speed)})
	}
	mem.SpeedOver = over

	if settings.HomeSet && lat != nil && lng != nil && settings.HomeRadiusMeters > 0 {
		inside := geo.Meters(*lat, *lng, settings.HomeLatitude, settings.HomeLongitude) <= settings.HomeRadiusMeters
		if mem.InsideHome != nil && *mem.InsideHome && !inside {
			out = append(out, Draft{Kind: LeftHome, Message: "Skylar left home. Honk honk, from the porch."})
		}
		mem.InsideHome = &inside
	}

	if tripEvent != "" && inCurfew(at, settings) {
		clock := at.In(location(settings.Timezone)).Format("3:04pm")
		switch tripEvent {
		case "start":
			out = append(out, Draft{
				Kind:    CurfewOn,
				Message: fmt.Sprintf("It's %s and Skylar just left Park. The horn would like a word.", clock),
			})
		case "end":
			out = append(out, Draft{
				Kind:    CurfewOff,
				Message: fmt.Sprintf("Skylar parked at %s. The horn was awake for that.", clock),
			})
		}
	}
	return out, mem
}

func speedLine(mph float64) string {
	rounded := int(mph + 0.5)
	return fmt.Sprintf("Skylar hit %d. I'm not mad, I'm just a horn.", rounded)
}

func inCurfew(at time.Time, settings model.Settings) bool {
	loc := location(settings.Timezone)
	t := at.In(loc)
	start := minutes(settings.CurfewStart)
	end := minutes(settings.CurfewEnd)
	if start < 0 || end < 0 || start == end {
		return false
	}
	cur := t.Hour()*60 + t.Minute()
	if start < end {
		return cur >= start && cur < end
	}
	return cur >= start || cur < end
}

func location(name string) *time.Location {
	if name == "" {
		return time.Local
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.Local
	}
	return loc
}

func minutes(hm string) int {
	if len(hm) != 5 || hm[2] != ':' {
		return -1
	}
	h := int(hm[0]-'0')*10 + int(hm[1]-'0')
	m := int(hm[3]-'0')*10 + int(hm[4]-'0')
	if h > 23 || m > 59 {
		return -1
	}
	return h*60 + m
}
