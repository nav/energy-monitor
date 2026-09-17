package store

import (
	"testing"
	"time"

	"github.com/nav/energy-monitor/internal/tou"
)

func TestStore_LatestReflectsSetDemand(t *testing.T) {
	s := New()
	now := time.Now()

	s.SetDemand(5.944, now)

	got := s.Latest()
	if got.KW != 5.944 {
		t.Errorf("KW = %v, want 5.944", got.KW)
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
	if got.KW != 5.944 {
		t.Errorf("KW = %v, want 5.944 to be preserved after SetSummation", got.KW)
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
	if got.KW != 0 || got.KWhDelivered != 0 || got.KWhReceived != 0 {
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
	if got.KW != 5.944 || got.KWhDelivered != 90 || got.KWhReceived != 0.01 {
		t.Errorf("Latest() = %+v, want demand/summation preserved after SetPrice", got)
	}
	if got.PricePerKWh != 0.1097 || got.Currency != "CAD" || got.RateLabel != "Block 1" {
		t.Errorf("Latest() = %+v, want price fields set", got)
	}
}

func TestStore_CostAccruesOnSummationDeltaAtTOURate(t *testing.T) {
	// Noon: off-peak, no time-of-use adjustment on top of the base rate.
	offPeak := time.Date(2024, 6, 15, 12, 0, 0, 0, tou.Location)

	s := New()
	s.SetSummation(100, 0, offPeak)                // first reading: establishes baseline, no cost yet
	s.SetSummation(105, 0, offPeak.Add(time.Hour)) // +5 kWh at $0.1270/kWh = $0.635

	got := s.Latest()
	if want := 0.635; got.CostDollars != want {
		t.Errorf("CostDollars = %v, want %v", got.CostDollars, want)
	}
}

func TestStore_CostAccruesAtTheRateInEffectWhenEachDeltaHappened(t *testing.T) {
	overnight := time.Date(2024, 6, 15, 2, 0, 0, 0, tou.Location)
	onPeak := time.Date(2024, 6, 15, 18, 0, 0, 0, tou.Location)

	s := New()
	s.SetSummation(100, 0, overnight)                // baseline
	s.SetSummation(105, 0, overnight.Add(time.Hour)) // +5 kWh overnight at $0.0770/kWh = $0.385
	s.SetSummation(110, 0, onPeak)                   // +5 kWh on-peak at $0.1770/kWh = $0.885

	got := s.Latest()
	if want := 0.385 + 0.885; got.CostDollars != want {
		t.Errorf("CostDollars = %v, want %v", got.CostDollars, want)
	}
}

func TestStore_CostAccrualIgnoresDeviceReportedPrice(t *testing.T) {
	offPeak := time.Date(2024, 6, 15, 12, 0, 0, 0, tou.Location)

	s := New()
	// A device-reported price wildly different from the TOU rate shouldn't affect cost.
	s.SetPrice(9.99, "CAD", "Block 1", offPeak)
	s.SetSummation(100, 0, offPeak)
	s.SetSummation(105, 0, offPeak.Add(time.Hour))

	got := s.Latest()
	if want := 0.635; got.CostDollars != want {
		t.Errorf("CostDollars = %v, want %v (device price should be ignored)", got.CostDollars, want)
	}
}

func TestStore_CostDoesNotAccrueOnSummationDecrease(t *testing.T) {
	t1 := time.Date(2024, 6, 15, 12, 0, 0, 0, tou.Location)

	s := New()
	s.SetSummation(100, 0, t1)
	s.SetSummation(50, 0, t1.Add(time.Hour)) // meter reset/rollback: ignore, don't accrue negative cost

	got := s.Latest()
	if got.CostDollars != 0 {
		t.Errorf("CostDollars = %v, want 0 when summation decreases", got.CostDollars)
	}
}
