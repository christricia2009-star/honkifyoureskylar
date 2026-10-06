package billing

import "testing"

func TestCaseStudyRates(t *testing.T) {
	commands := Cost("command", 1211)
	if commands < 1.20 || commands > 1.22 {
		t.Fatalf("commands = %v", commands)
	}
	signals := Cost("streaming_signal", 1000)
	if signals < 0.006 || signals > 0.007 {
		t.Fatalf("signals = %v", signals)
	}
	hour := Cost("vehicle_data", 60)
	if hour < 0.11 || hour > 0.13 {
		t.Fatalf("vehicle_data hour = %v", hour)
	}
	optimized := Cost("command", 4) + Cost("wake", 1) + Cost("streaming_signal", 300)
	if optimized < 0.025 || optimized > 0.027 {
		t.Fatalf("optimized session = %v", optimized)
	}
}
