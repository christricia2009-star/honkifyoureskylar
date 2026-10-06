package horn

import (
	"fmt"
	"strings"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/model"
)

func Line(state string, speed *float64, limit float64, seat, guest *bool, inTrip bool, lastSeen *time.Time, now time.Time) model.Horn {
	if !strings.EqualFold(state, "online") {
		when := "a while ago"
		if lastSeen != nil {
			when = lastSeen.In(now.Location()).Format("3:04pm")
		}
		if strings.EqualFold(state, "asleep") {
			return model.Horn{
				Mood: "napping",
				Line: fmt.Sprintf("Skylar's car is asleep. Last I heard from it at %s. I don't poke sleeping cars.", when),
			}
		}
		label := state
		if label == "" {
			label = "offline"
		}
		return model.Horn{
			Mood: "napping",
			Line: fmt.Sprintf("Tesla lists this car as %s. Last I heard from it at %s. A confirmed poke tries one wake.", label, when),
		}
	}
	if guest != nil && *guest && seat != nil && *seat {
		line := "Guest mode is on and the seat is occupied. This might not be Skylar. The horn is not going to guess."
		if speed != nil && *speed > limit {
			line = fmt.Sprintf("Guest mode is on and the seat is occupied. The speedometer says %d. This might not be Skylar. The horn is not going to guess.", int(*speed+0.5))
		}
		return model.Horn{Mood: "watching", Line: line}
	}
	if speed != nil && *speed > limit {
		return model.Horn{
			Mood: "upset",
			Line: fmt.Sprintf("Skylar hit %d. I'm not mad, I'm just a horn.", int(*speed+0.5)),
		}
	}
	if inTrip {
		return model.Horn{
			Mood: "watching",
			Line: "The car is rolling. The horn is watching the speedometer and minding its own name.",
		}
	}
	if seat != nil && *seat {
		return model.Horn{
			Mood: "watching",
			Line: "Someone is in the driver seat. Honk knows the seat, not the name.",
		}
	}
	if speed == nil && seat == nil && guest == nil && !inTrip {
		return model.Horn{
			Mood: "waiting",
			Line: "The car is online. Grab a reading for battery, location, lock, and the settings the Tesla app buries.",
		}
	}
	return model.Horn{
		Mood: "napping",
		Line: "The car is parked. The horn is napping with one eye open.",
	}
}
