package ledger

import (
	"strings"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/trips"
)

// Reading is one merged look at the car. Empty strings mean that field did not arrive.
type Reading struct {
	At             time.Time
	Gear           string
	Charge         string
	Soc            *float64
	EstRange       *float64
	RatedRange     *float64
	Odometer       *float64
	EnergyAddedKwh *float64
	ACEnergyKwh    *float64
	DCEnergyKwh    *float64
	LifetimeKwh    *float64
	RemainingKwh   *float64
	Fast           *bool
	FastType       string
	Software       string
}

// Charge is one plug-in session Honk actually saw.
type Charge struct {
	ID         string
	StartedAt  time.Time
	EndedAt    *time.Time
	LastSeen   time.Time
	SocStart   *float64
	SocEnd     *float64
	EnergyKwh  *float64
	Kind       string
	Open       bool
	OneReading bool
	previous   *Charge
}

// Drain is a parked stretch where the car was not charging.
type Drain struct {
	ID         string
	StartedAt  time.Time
	EndedAt    *time.Time
	LastSeen   time.Time
	SocStart   *float64
	SocEnd     *float64
	RangeStart *float64
	RangeEnd   *float64
	Open       bool
	previous   *Drain
}

// BatteryPoint is one odometer sample for the range trend.
type BatteryPoint struct {
	At       time.Time
	Odometer float64
	RangeMi  *float64
	Soc      *float64
}

// Book is the open ledger loaded for one car.
type Book struct {
	Charge       *Charge
	Drain        *Drain
	LastClosed   *Charge
	LastOdo      *float64
	LastBattery  time.Time
	LastSoftware string
}

// Outcome is what to write after one reading.
type Outcome struct {
	Charge   *Charge
	Drain    *Drain
	Battery  *BatteryPoint
	Software string
}

const gap = 6 * time.Hour

// Apply folds one reading into the open charge session, parked drain, battery trend, and software version.
func Apply(prev Book, r Reading, id string) (Book, Outcome) {
	if r.At.IsZero() {
		r.At = time.Now().UTC()
	}
	var out Outcome
	next := prev
	if c := stepCharge(prev, r, id); c != nil {
		out.Charge = c
		if c.previous != nil {
			closed := *c.previous
			next.LastClosed = &closed
		}
		if c.Open {
			next.Charge = c
		} else {
			next.Charge = nil
			closed := *c
			next.LastClosed = &closed
		}
	}
	if d := stepDrain(next, r, id); d != nil {
		out.Drain = d
		if d.Open {
			next.Drain = d
		} else {
			next.Drain = nil
		}
	}
	if p := stepBattery(prev, r); p != nil {
		out.Battery = p
		odo := p.Odometer
		next.LastOdo = &odo
		next.LastBattery = p.At
	}
	if r.Software != "" && r.Software != prev.LastSoftware {
		out.Software = r.Software
		next.LastSoftware = r.Software
	}
	return next, out
}

func stepCharge(prev Book, r Reading, id string) *Charge {
	energy := sessionEnergy(r)
	kind := chargeKind(r)
	active := charging(r.Charge)
	if prev.Charge != nil && prev.Charge.Open && r.At.Sub(prev.Charge.LastSeen) > gap {
		closed := *prev.Charge
		end := closed.LastSeen
		closed.EndedAt = &end
		closed.Open = false
		if active {
			fresh := newCharge(id, r, energy, kind)
			// The caller saves one charge. Prefer the new open session.
			// The closed one is returned by splitting: save closed first via LastSeen close
			// handled below by returning the fresh session only if we also mark the old closed.
			// Store both by closing in the returned charge's predecessor through a side field.
			fresh.previous = &closed
			return fresh
		}
		return &closed
	}
	if active {
		if prev.Charge == nil || !prev.Charge.Open {
			return newCharge(id, r, energy, kind)
		}
		c := *prev.Charge
		if c.EnergyKwh != nil && energy != nil && *energy+0.5 < *c.EnergyKwh {
			end := c.LastSeen
			c.EndedAt = &end
			c.Open = false
			fresh := newCharge(id+"b", r, energy, kind)
			fresh.previous = &c
			return fresh
		}
		c.LastSeen = r.At
		if r.Soc != nil {
			c.SocEnd = r.Soc
		}
		if energy != nil {
			c.EnergyKwh = energy
		}
		if kind != "" {
			c.Kind = kind
		}
		return &c
	}
	if prev.Charge != nil && prev.Charge.Open {
		c := *prev.Charge
		end := r.At
		c.EndedAt = &end
		c.LastSeen = r.At
		c.Open = false
		if r.Soc != nil {
			c.SocEnd = r.Soc
		}
		if energy != nil {
			c.EnergyKwh = energy
		}
		if kind != "" {
			c.Kind = kind
		}
		return &c
	}
	if energy != nil && *energy >= 0.2 && finishedCharge(r.Charge) && !sameCharge(prev.LastClosed, *energy, r.At) {
		end := r.At
		soc := r.Soc
		return &Charge{
			ID: id, StartedAt: r.At, EndedAt: &end, LastSeen: r.At,
			SocStart: soc, SocEnd: soc, EnergyKwh: energy, Kind: kind, OneReading: true,
		}
	}
	return nil
}

func newCharge(id string, r Reading, energy *float64, kind string) *Charge {
	c := &Charge{ID: id, StartedAt: r.At, LastSeen: r.At, Open: true, Kind: kind, EnergyKwh: energy}
	if r.Soc != nil {
		c.SocStart = r.Soc
		c.SocEnd = r.Soc
	}
	return c
}

func sameCharge(prev *Charge, energy float64, at time.Time) bool {
	if prev == nil || prev.EnergyKwh == nil {
		return false
	}
	end := prev.LastSeen
	if prev.EndedAt != nil {
		end = *prev.EndedAt
	}
	if energy+0.3 < *prev.EnergyKwh || *prev.EnergyKwh+0.3 < energy {
		return false
	}
	return at.Sub(end) < 12*time.Hour
}

func stepDrain(prev Book, r Reading, id string) *Drain {
	// A charge session that just opened should end the parked drain.
	chargingNow := charging(r.Charge)
	gear := ""
	if r.Gear != "" {
		gear = trips.NormGear(r.Gear)
	}
	leavingPark := chargingNow || (gear != "" && gear != "P")
	if prev.Drain != nil && prev.Drain.Open && r.At.Sub(prev.Drain.LastSeen) > gap {
		closed := *prev.Drain
		end := closed.LastSeen
		closed.EndedAt = &end
		closed.Open = false
		if !leavingPark && gear == "P" && measurable(r) {
			fresh := newDrain(id, r)
			fresh.previous = &closed
			return fresh
		}
		return &closed
	}
	if leavingPark {
		if prev.Drain == nil || !prev.Drain.Open {
			return nil
		}
		d := *prev.Drain
		end := d.LastSeen
		if !r.At.IsZero() && r.At.After(end) && r.At.Sub(end) <= gap {
			end = r.At
		}
		d.EndedAt = &end
		d.Open = false
		return &d
	}
	if gear != "P" {
		return nil
	}
	if !measurable(r) && (prev.Drain == nil || !prev.Drain.Open) {
		return nil
	}
	if prev.Drain == nil || !prev.Drain.Open {
		if !measurable(r) {
			return nil
		}
		return newDrain(id, r)
	}
	d := *prev.Drain
	d.LastSeen = r.At
	if r.Soc != nil {
		d.SocEnd = r.Soc
	}
	if rng := drainRange(r); rng != nil {
		d.RangeEnd = rng
	}
	return &d
}

func newDrain(id string, r Reading) *Drain {
	d := &Drain{ID: id, StartedAt: r.At, LastSeen: r.At, Open: true}
	if r.Soc != nil {
		d.SocStart = r.Soc
		d.SocEnd = r.Soc
	}
	if rng := drainRange(r); rng != nil {
		d.RangeStart = rng
		d.RangeEnd = rng
	}
	return d
}

func stepBattery(prev Book, r Reading) *BatteryPoint {
	if r.Odometer == nil {
		return nil
	}
	rng := r.RatedRange
	if rng == nil {
		rng = r.EstRange
	}
	if rng == nil && r.Soc == nil {
		return nil
	}
	if prev.LastOdo != nil && *r.Odometer+0.2 < *prev.LastOdo {
		return nil
	}
	due := prev.LastOdo == nil || *r.Odometer-*prev.LastOdo >= 5 || r.At.Sub(prev.LastBattery) >= 24*time.Hour
	if !due {
		return nil
	}
	return &BatteryPoint{At: r.At, Odometer: *r.Odometer, RangeMi: rng, Soc: r.Soc}
}

func measurable(r Reading) bool {
	return r.Soc != nil || drainRange(r) != nil
}

func drainRange(r Reading) *float64 {
	if r.RatedRange != nil {
		return r.RatedRange
	}
	return r.EstRange
}

func charging(state string) bool {
	switch normalizeCharge(state) {
	case "charging", "starting":
		return true
	default:
		return false
	}
}

func finishedCharge(state string) bool {
	switch normalizeCharge(state) {
	case "complete", "stopped", "disconnected":
		return true
	default:
		return false
	}
}

func normalizeCharge(state string) string {
	s := strings.ToLower(strings.TrimSpace(state))
	s = strings.TrimPrefix(s, "detailedchargestate")
	s = strings.TrimPrefix(s, "chargestate")
	s = strings.TrimPrefix(s, "chargingstate")
	return s
}

func sessionEnergy(r Reading) *float64 {
	if r.ACEnergyKwh != nil || r.DCEnergyKwh != nil {
		sum := 0.0
		if r.ACEnergyKwh != nil {
			sum += *r.ACEnergyKwh
		}
		if r.DCEnergyKwh != nil {
			sum += *r.DCEnergyKwh
		}
		return &sum
	}
	return r.EnergyAddedKwh
}

func chargeKind(r Reading) string {
	t := strings.ToLower(r.FastType)
	switch {
	case strings.Contains(t, "supercharger"):
		return "Supercharger"
	case strings.Contains(t, "chademo"), strings.Contains(t, "combo"), strings.Contains(t, "gb"):
		return "DC fast"
	case r.Fast != nil && *r.Fast:
		return "DC fast"
	case r.Fast != nil && !*r.Fast:
		return "AC"
	default:
		return ""
	}
}

// TakePrevious pulls a charge or drain that must be saved before the returned open row.
func TakePreviousCharge(c *Charge) *Charge {
	if c == nil {
		return nil
	}
	prev := c.previous
	c.previous = nil
	return prev
}

func TakePreviousDrain(d *Drain) *Drain {
	if d == nil {
		return nil
	}
	prev := d.previous
	d.previous = nil
	return prev
}
