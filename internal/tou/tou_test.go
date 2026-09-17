package tou

import (
	"testing"
	"time"
)

func TestRatePerKWh(t *testing.T) {
	date := func(hour, min int) time.Time {
		return time.Date(2024, 6, 15, hour, min, 0, 0, Location)
	}

	tests := []struct {
		name string
		at   time.Time
		want float64
	}{
		{"overnight start (11pm)", date(23, 0), 0.0770},
		{"overnight middle (2am)", date(2, 0), 0.0770},
		{"overnight last minute (6:59am)", date(6, 59), 0.0770},
		{"off-peak morning start (7am)", date(7, 0), 0.1270},
		{"off-peak midday (noon)", date(12, 0), 0.1270},
		{"off-peak last minute before on-peak (3:59pm)", date(15, 59), 0.1270},
		{"on-peak start (4pm)", date(16, 0), 0.1770},
		{"on-peak middle (6pm)", date(18, 0), 0.1770},
		{"on-peak last minute (8:59pm)", date(20, 59), 0.1770},
		{"off-peak evening start (9pm)", date(21, 0), 0.1270},
		{"off-peak last minute before overnight (10:59pm)", date(22, 59), 0.1270},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RatePerKWh(tt.at); got != tt.want {
				t.Errorf("RatePerKWh(%v) = %v, want %v", tt.at, got, tt.want)
			}
		})
	}
}

func TestRatePerKWh_ConvertsFromOtherTimezones(t *testing.T) {
	// 2024-06-15 02:00 UTC is 2024-06-14 19:00 America/Vancouver (PDT, UTC-7) -- on-peak.
	utc := time.Date(2024, 6, 15, 2, 0, 0, 0, time.UTC)
	if got, want := RatePerKWh(utc), 0.1770; got != want {
		t.Errorf("RatePerKWh(%v) = %v, want %v (on-peak in local time)", utc, got, want)
	}
}
