package store

import (
	"testing"
	"time"
)

func TestStore_LatestReflectsSetDemand(t *testing.T) {
	s := New()
	now := time.Now()

	s.SetDemand(5.944, now)

	got := s.Latest()
	if got.Watts != 5.944 {
		t.Errorf("Watts = %v, want 5.944", got.Watts)
	}
	if !got.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, now)
	}
}

func TestStore_SetSummationDoesNotClearDemand(t *testing.T) {
	s := New()
	t1 := time.Now()
	t2 := t1.Add(time.Second)

	s.SetDemand(5.944, t1)
	s.SetSummation(90, 0.01, t2)

	got := s.Latest()
	if got.Watts != 5.944 {
		t.Errorf("Watts = %v, want 5.944 to be preserved after SetSummation", got.Watts)
	}
	if got.KWhDelivered != 90 {
		t.Errorf("KWhDelivered = %v, want 90", got.KWhDelivered)
	}
	if got.KWhReceived != 0.01 {
		t.Errorf("KWhReceived = %v, want 0.01", got.KWhReceived)
	}
	if !got.UpdatedAt.Equal(t2) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, t2)
	}
}

func TestStore_LatestBeforeAnyUpdateIsZeroValue(t *testing.T) {
	s := New()
	got := s.Latest()
	if got.Watts != 0 || got.KWhDelivered != 0 || got.KWhReceived != 0 {
		t.Errorf("Latest() = %+v, want zero-value reading", got)
	}
	if !got.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt = %v, want zero time", got.UpdatedAt)
	}
}

func TestStore_SetPriceDoesNotClearDemandOrSummation(t *testing.T) {
	s := New()
	now := time.Now()

	s.SetDemand(5.944, now)
	s.SetSummation(90, 0.01, now)
	s.SetPrice(0.1097, "CAD", "Block 1", now)

	got := s.Latest()
	if got.Watts != 5.944 || got.KWhDelivered != 90 || got.KWhReceived != 0.01 {
		t.Errorf("Latest() = %+v, want demand/summation preserved after SetPrice", got)
	}
	if got.PricePerKWh != 0.1097 || got.Currency != "CAD" || got.RateLabel != "Block 1" {
		t.Errorf("Latest() = %+v, want price fields set", got)
	}
}

func TestStore_CostAccruesOnSummationDeltaAtKnownPrice(t *testing.T) {
	s := New()
	t1 := time.Now()

	s.SetPrice(0.10, "CAD", "Block 1", t1)
	s.SetSummation(100, 0, t1)                // first reading: establishes baseline, no cost yet
	s.SetSummation(105, 0, t1.Add(time.Hour)) // +5 kWh at $0.10/kWh = $0.50

	got := s.Latest()
	if want := 0.50; got.CostDollars != want {
		t.Errorf("CostDollars = %v, want %v", got.CostDollars, want)
	}
}

func TestStore_CostDoesNotAccrueBeforePriceIsKnown(t *testing.T) {
	s := New()
	t1 := time.Now()

	s.SetSummation(100, 0, t1)
	s.SetSummation(105, 0, t1.Add(time.Hour)) // no price set yet

	got := s.Latest()
	if got.CostDollars != 0 {
		t.Errorf("CostDollars = %v, want 0 when price was never set", got.CostDollars)
	}
}

func TestStore_CostDoesNotAccrueOnSummationDecrease(t *testing.T) {
	s := New()
	t1 := time.Now()

	s.SetPrice(0.10, "CAD", "Block 1", t1)
	s.SetSummation(100, 0, t1)
	s.SetSummation(50, 0, t1.Add(time.Hour)) // meter reset/rollback: ignore, don't accrue negative cost

	got := s.Latest()
	if got.CostDollars != 0 {
		t.Errorf("CostDollars = %v, want 0 when summation decreases", got.CostDollars)
	}
}
