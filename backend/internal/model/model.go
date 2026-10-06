package model

import (
	"encoding/json"
	"time"
)

// APITime is RFC3339 in UTC so the iOS decoder can read it.
type APITime time.Time

func (t APITime) MarshalJSON() ([]byte, error) {
	tt := time.Time(t)
	if tt.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + tt.UTC().Format(time.RFC3339) + `"`), nil
}

func (t *APITime) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*t = APITime{}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return err
		}
	}
	*t = APITime(parsed)
	return nil
}

func (t APITime) Time() time.Time { return time.Time(t) }

type Settings struct {
	SpeedLimitMph    float64 `json:"speedLimitMph"`
	CurfewStart      string  `json:"curfewStart"`
	CurfewEnd        string  `json:"curfewEnd"`
	HomeLatitude     float64 `json:"homeLatitude"`
	HomeLongitude    float64 `json:"homeLongitude"`
	HomeRadiusMeters float64 `json:"homeRadiusMeters"`
	HomeSet          bool    `json:"homeSet"`
	Timezone         string  `json:"timezone"`
	SelectedVIN      string  `json:"selectedVin"`
}

func DefaultSettings() Settings {
	return Settings{
		SpeedLimitMph:    75,
		CurfewStart:      "23:00",
		CurfewEnd:        "04:00",
		HomeRadiusMeters: 250,
		Timezone:         "America/Los_Angeles",
	}
}

type LatLng struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	SpeedMph  float64 `json:"speedMph,omitempty"`
	At        APITime `json:"at,omitempty"`
}

type Driver struct {
	Name   string `json:"name"`
	Detail string `json:"detail"`
	Sample bool   `json:"sample"`
}

type Alert struct {
	ID        string  `json:"id"`
	VIN       string  `json:"vin"`
	Kind      string  `json:"kind"`
	Message   string  `json:"message"`
	CreatedAt APITime `json:"createdAt"`
	Read      bool    `json:"read"`
}

type Trip struct {
	ID             string     `json:"id"`
	VIN            string     `json:"vin"`
	Demo           bool       `json:"demo"`
	StartedAt      time.Time  `json:"-"`
	EndedAt        *time.Time `json:"-"`
	MaxSpeedMph    float64    `json:"maxSpeedMph"`
	DistanceMiles  float64    `json:"distanceMiles"`
	OverLimit      bool       `json:"overLimit"`
	Polyline       []LatLng   `json:"polyline"`
	StartOdo       *float64   `json:"-"`
	EndOdo         *float64   `json:"-"`
	SeatOccupied   *bool      `json:"seatOccupied"`
	GuestMode      *bool      `json:"guestMode"`
	ParkSince      *time.Time `json:"-"`
	PendingClose   bool       `json:"pendingClose"`
	Callout        string     `json:"callout,omitempty"`
	SpeedLimitMph  float64    `json:"speedLimitMph"`
}

type Snapshot struct {
	VIN          string
	SpeedMph     *float64
	Gear         string
	Lat          *float64
	Lng          *float64
	Soc          *float64
	RangeMi      *float64
	ChargeState  string
	DoorsOpen    bool
	DoorSummary  string
	Locked       *bool
	Seat         *bool
	Guest        *bool
	Odometer     *float64
	UpdatedAt    time.Time
	InsideHome   *bool
	SpeedOver    bool
	HaveDoors    bool
}

type Vehicle struct {
	VIN      string
	Demo     bool
	Name     string
	State    string
	LastSeen *time.Time
}

type TeslaAuth struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
	Scopes       string
	FleetBase    string
}

type Usage struct {
	Month               string  `json:"month"`
	Demo                bool    `json:"demo"`
	StreamingSignals    int     `json:"streamingSignals"`
	VehicleDataCalls    int     `json:"vehicleDataCalls"`
	Wakes               int     `json:"wakes"`
	Commands            int     `json:"commands"`
	MetadataCalls       int     `json:"metadataCalls"`
	EstimatedUSD        float64 `json:"estimatedUsd"`
	MonthlyCreditUSD    float64 `json:"monthlyCreditUsd"`
	RemainingCreditUSD  float64 `json:"remainingCreditUsd"`
	Note                string  `json:"note"`
}

type Horn struct {
	Mood string `json:"mood"`
	Line string `json:"line"`
}

type VehicleView struct {
	VIN                 string   `json:"vin"`
	Name                string   `json:"name"`
	State               string   `json:"state"`
	LastSeen            *APITime `json:"lastSeen"`
	SpeedMph            *float64 `json:"speedMph"`
	Gear                string   `json:"gear"`
	GearLabel           string   `json:"gearLabel"`
	Latitude            *float64 `json:"latitude"`
	Longitude           *float64 `json:"longitude"`
	Soc                 *float64 `json:"soc"`
	EstRangeMiles       *float64 `json:"estRangeMiles"`
	ChargeState         string   `json:"chargeState"`
	DoorsOpen           bool     `json:"doorsOpen"`
	DoorSummary         string   `json:"doorSummary"`
	Locked              *bool    `json:"locked"`
	DriverSeatOccupied  *bool    `json:"driverSeatOccupied"`
	GuestMode           *bool    `json:"guestMode"`
	ActiveDriverNote    string   `json:"activeDriverNote"`
	InTrip              bool     `json:"inTrip"`
}

type TripView struct {
	ID              string   `json:"id"`
	VIN             string   `json:"vin"`
	StartedAt       APITime  `json:"startedAt"`
	EndedAt         *APITime `json:"endedAt"`
	MaxSpeedMph     float64  `json:"maxSpeedMph"`
	DistanceMiles   float64  `json:"distanceMiles"`
	OverLimit       bool     `json:"overLimit"`
	SpeedLimitMph   float64  `json:"speedLimitMph"`
	Polyline        []LatLng `json:"polyline"`
	SeatOccupied    *bool    `json:"seatOccupied"`
	GuestMode       *bool    `json:"guestMode"`
	PendingClose    bool     `json:"pendingClose"`
	Callout         string   `json:"callout,omitempty"`
}

type Garage struct {
	Demo       bool        `json:"demo"`
	Linked     bool        `json:"linked"`
	Vehicle    *VehicleView `json:"vehicle"`
	ActiveTrip *TripView   `json:"activeTrip"`
	Horn       Horn        `json:"horn"`
	Usage      Usage       `json:"usage"`
	ServerTime APITime     `json:"serverTime"`
}

const ActiveDriverNote = "The public Fleet API does not reliably return the in-car profile name. Honk shows the driver seat, guest mode, and the allow-list. It does not invent who is driving."
