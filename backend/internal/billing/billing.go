package billing

import "math"

// Rates are the ones implied by Tesla's own billing examples:
// 1,211 commands = $1.21, so a command is $0.001.
// 1,000 streaming signals an hour = $0.00667, so a signal is $0.00000667.
// Polling vehicle_data for an hour at one request a minute = $0.12, so a
// vehicle_data call is $0.002.
// The optimized case study ($0.026 = 4 commands + 1 wake + 300 signals)
// puts a wake at $0.02.
// Confirm the live table in the Tesla developer portal. These are estimates.
const (
	USDPerStreamingSignal = 0.00000667
	USDPerVehicleData     = 0.002
	USDPerCommand         = 0.001
	USDPerWake            = 0.02
	MonthlyCreditUSD      = 10.0
)

func Cost(kind string, units float64) float64 {
	switch kind {
	case "streaming_signal":
		return units * USDPerStreamingSignal
	case "vehicle_data":
		return units * USDPerVehicleData
	case "command":
		return units * USDPerCommand
	case "wake":
		return units * USDPerWake
	default:
		return 0
	}
}

func RoundCents(v float64) float64 {
	return math.Round(v*100) / 100
}

const Note = "Estimated from Tesla's published billing examples, not from a live invoice. Streaming signal $0.00000667, vehicle_data $0.002, command $0.001, wake $0.02. List and drivers calls are counted and not given a dollar amount. A $10 monthly credit is the default assumption. Tesla bills requests that return a status below 500. Honk does not poll vehicle_data on a timer."
