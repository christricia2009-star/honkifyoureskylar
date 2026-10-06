package trips

import (
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/geo"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/model"
)

const ParkDwell = 2 * time.Minute

type Sample struct {
	At        time.Time
	Gear      string
	SpeedMph  float64
	Lat       float64
	Lng       float64
	HasLoc    bool
	Odometer  *float64
	Seat      *bool
	Guest     *bool
}

type Open struct {
	ID            string
	VIN           string
	Demo          bool
	StartedAt     time.Time
	MaxSpeedMph   float64
	DistanceMiles float64
	Polyline      []model.LatLng
	StartOdo      *float64
	EndOdo        *float64
	Seat          *bool
	Guest         *bool
	ParkSince     *time.Time
	EndedAt       *time.Time
	LastLat       float64
	LastLng       float64
	HasLast       bool
}

type Result struct {
	Open    *Open
	Started bool
	Ended   *Open
}

func IsPark(gear string) bool {
	switch NormGear(gear) {
	case "P":
		return true
	default:
		return false
	}
}

func NormGear(gear string) string {
	switch gear {
	case "P", "Park", "park", "ShiftStateP", "shiftStateP", "SHIFT_STATE_P":
		return "P"
	case "R", "Reverse", "reverse", "ShiftStateR", "shiftStateR":
		return "R"
	case "N", "Neutral", "neutral", "ShiftStateN", "shiftStateN":
		return "N"
	case "D", "Drive", "drive", "ShiftStateD", "shiftStateD":
		return "D"
	default:
		return gear
	}
}

func GearLabel(gear string) string {
	switch NormGear(gear) {
	case "P":
		return "Park"
	case "R":
		return "Reverse"
	case "N":
		return "Neutral"
	case "D":
		return "Drive"
	case "":
		return "Unknown"
	default:
		return gear
	}
}

// Step moves the trip state machine.
// A trip starts when gear is out of Park and speed is above 0.
// It ends after gear has been Park for parkFor (2 minutes in production).
func Step(prev *Open, s Sample, parkFor time.Duration, newID string) Result {
	gear := NormGear(s.Gear)
	driving := !IsPark(gear) && s.SpeedMph > 0
	parked := IsPark(gear)

	if prev == nil {
		if !driving {
			return Result{}
		}
		open := &Open{
			ID:          newID,
			StartedAt:   s.At,
			MaxSpeedMph: s.SpeedMph,
			Seat:        s.Seat,
			Guest:       s.Guest,
		}
		absorb(open, s, gear)
		return Result{Open: open, Started: true}
	}

	open := *prev
	if s.SpeedMph > open.MaxSpeedMph {
		open.MaxSpeedMph = s.SpeedMph
	}
	if s.Seat != nil {
		open.Seat = s.Seat
	}
	if s.Guest != nil {
		open.Guest = s.Guest
	}
	absorb(&open, s, gear)

	if parked {
		if open.ParkSince == nil {
			t := s.At
			open.ParkSince = &t
			return Result{Open: &open}
		}
		if s.At.Sub(*open.ParkSince) >= parkFor {
			ended := open
			at := s.At
			ended.EndedAt = &at
			ended.DistanceMiles = chooseDistance(ended)
			return Result{Ended: &ended}
		}
		return Result{Open: &open}
	}

	open.ParkSince = nil
	return Result{Open: &open}
}

func absorb(open *Open, s Sample, gear string) {
	if s.Odometer != nil {
		if open.StartOdo == nil {
			v := *s.Odometer
			open.StartOdo = &v
		}
		v := *s.Odometer
		open.EndOdo = &v
	}
	if !s.HasLoc {
		return
	}
	if open.HasLast {
		miles := geo.Miles(open.LastLat, open.LastLng, s.Lat, s.Lng)
		if miles >= 0.003 {
			open.DistanceMiles += miles
		}
	}
	open.LastLat = s.Lat
	open.LastLng = s.Lng
	open.HasLast = true
	open.Polyline = append(open.Polyline, model.LatLng{
		Latitude:  s.Lat,
		Longitude: s.Lng,
		SpeedMph:  s.SpeedMph,
		At:        model.APITime(s.At),
	})
	_ = gear
}

func chooseDistance(o Open) float64 {
	if o.StartOdo != nil && o.EndOdo != nil {
		d := *o.EndOdo - *o.StartOdo
		if d >= 0 && d < 1000 {
			return d
		}
	}
	return o.DistanceMiles
}
