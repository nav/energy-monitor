// Package store holds the latest decoded EAGLE reading in memory for HTTP handlers to
// read from.
package store

import (
	"sync"
	"time"
)

// Reading is the latest known values from the EAGLE. Zero-value fields mean no reading
// of that kind has been received yet.
type Reading struct {
	Watts        float64
	KWhDelivered float64
	KWhReceived  float64
	UpdatedAt    time.Time
}

// Store is a mutex-protected cache of the latest Reading.
type Store struct {
	mu      sync.RWMutex
	reading Reading
}

// New returns an empty Store.
func New() *Store {
	return &Store{}
}

// SetDemand records a new instantaneous demand reading without disturbing previously
// recorded summation values.
func (s *Store) SetDemand(watts float64, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reading.Watts = watts
	s.reading.UpdatedAt = at
}

// SetSummation records new cumulative summation readings without disturbing the
// previously recorded demand value.
func (s *Store) SetSummation(deliveredKWh, receivedKWh float64, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reading.KWhDelivered = deliveredKWh
	s.reading.KWhReceived = receivedKWh
	s.reading.UpdatedAt = at
}

// Latest returns a copy of the current Reading.
func (s *Store) Latest() Reading {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reading
}
