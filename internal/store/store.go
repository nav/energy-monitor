// Package store holds the latest decoded EAGLE reading in memory for HTTP handlers to
// read from.
package store

import (
	"sync"
	"time"

	"github.com/nav/energy-monitor/internal/tou"
)

// Reading is the latest known values from the EAGLE. Zero-value fields mean no reading
// of that kind has been received yet.
type Reading struct {
	KW           float64
	KWhDelivered float64
	KWhReceived  float64
	// PricePerKWh, Currency and RateLabel are the EAGLE's own reported PriceCluster
	// values. They're informational only -- the device's reported price does not reflect
	// the utility's time-of-use adjustments (see package tou), so CostDollars below is
	// costed independently rather than from this field.
	PricePerKWh float64
	Currency    string
	RateLabel   string
	// CostDollars accrues delivered-kWh deltas at the tou.RatePerKWh in effect at the time
	// of each delta. It is only an in-memory running total (reset on process restart);
	// Prometheus's own counter-reset handling in rate()/increase() reconstructs the correct
	// total across restarts as long as a scrape lands close to the restart.
	CostDollars float64
	UpdatedAt   time.Time
}

// Store is a mutex-protected cache of the latest Reading.
type Store struct {
	mu               sync.RWMutex
	reading          Reading
	prevKWhDelivered float64
	haveKWhDelivered bool
}

// New returns an empty Store.
func New() *Store {
	return &Store{}
}

// SetDemand records a new instantaneous demand reading (in kW) without disturbing
// previously recorded summation or price values.
func (s *Store) SetDemand(kw float64, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reading.KW = kw
	s.reading.UpdatedAt = at
}

// SetSummation records new cumulative summation readings without disturbing the
// previously recorded demand or price values. If a prior delivered-kWh reading is known,
// the increase is costed at tou.RatePerKWh(at) -- the utility's time-of-use rate for when
// the usage occurred, not anything reported by the device -- and added to CostDollars.
// The first reading ever received only establishes a baseline (no prior value to diff
// against), and a decrease (e.g. a meter reset) is ignored rather than treated as negative
// cost.
func (s *Store) SetSummation(deliveredKWh, receivedKWh float64, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.haveKWhDelivered {
		delta := deliveredKWh - s.prevKWhDelivered
		if delta > 0 {
			s.reading.CostDollars += delta * tou.RatePerKWh(at)
		}
	}
	s.prevKWhDelivered = deliveredKWh
	s.haveKWhDelivered = true

	s.reading.KWhDelivered = deliveredKWh
	s.reading.KWhReceived = receivedKWh
	s.reading.UpdatedAt = at
}

// SetPrice records the current price without disturbing previously recorded demand or
// summation values.
func (s *Store) SetPrice(pricePerKWh float64, currency, rateLabel string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reading.PricePerKWh = pricePerKWh
	s.reading.Currency = currency
	s.reading.RateLabel = rateLabel
	s.reading.UpdatedAt = at
}

// Latest returns a copy of the current Reading.
func (s *Store) Latest() Reading {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reading
}
