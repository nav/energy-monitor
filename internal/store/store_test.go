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
