// Package tou computes the time-of-use electricity rate in effect at a given moment, per
// the utility's own published residential flat-rate TOU schedule. This is deliberately
// independent of anything the EAGLE itself reports (its PriceCluster upload does not
// reflect these time-based adjustments), so cost math doesn't depend on trusting the
// device's number.
package tou

import "time"

// Location is the timezone the schedule below is defined in -- the on-peak/off-peak/
// overnight windows are local wall-clock time, not UTC.
var Location = mustLoadLocation("America/Vancouver")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		// Only reachable if the binary was built without timezone data available (see
		// main.go's time/tzdata import) -- a build-time misconfiguration, not a runtime
		// condition callers can recover from.
		panic("tou: " + err.Error())
	}
	return loc
}

// BaseRate is the flat rate, in dollars/kWh, charged on all usage before the time-of-use
// adjustments below.
const BaseRate = 0.1270

// Adjustments applied to BaseRate depending on when the usage occurred.
const (
	OvernightAdjustment = -0.05 // 11 p.m. to 7 a.m.
	OnPeakAdjustment    = 0.05  // 4 p.m. to 9 p.m.
	// Off-peak (7 a.m.-4 p.m. and 9-11 p.m.) gets no adjustment.
)

// RatePerKWh returns the dollars/kWh rate in effect at t, per the utility's residential
// flat rate with time-of-use pricing.
func RatePerKWh(t time.Time) float64 {
	hour := t.In(Location).Hour()
	switch {
	case hour >= 23 || hour < 7:
		return BaseRate + OvernightAdjustment
	case hour >= 16 && hour < 21:
		return BaseRate + OnPeakAdjustment
	default:
		return BaseRate
	}
}
