package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nav/energy-monitor/internal/store"
)

const instantaneousDemandFixture = `<?xml version="1.0"?>
<rainforest macId="0xf0ad4e00ce69" timestamp="1355292588s">
<InstantaneousDemand>
<DeviceMacId>0x00158d0000000004</DeviceMacId>
<MeterMacId>0x00178d0000000004</MeterMacId>
<TimeStamp>0x185adc1d</TimeStamp>
<Demand>0x001738</Demand>
<Multiplier>0x00000001</Multiplier>
<Divisor>0x000003e8</Divisor>
<DigitsRight>0x03</DigitsRight>
<DigitsLeft>0x00</DigitsLeft>
<SuppressLeadingZero>Y</SuppressLeadingZero>
</InstantaneousDemand>
</rainforest>`

func postUpload(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/eagle/upload", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHandleUpload_UpdatesCurrentAndMetrics(t *testing.T) {
	s := New(store.New())
	h := s.Routes()

	rec := postUpload(t, h, instantaneousDemandFixture)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status = %d, want 200", rec.Code)
	}

	currentReq := httptest.NewRequest(http.MethodGet, "/current", nil)
	currentRec := httptest.NewRecorder()
	h.ServeHTTP(currentRec, currentReq)
	if currentRec.Code != http.StatusOK {
		t.Fatalf("/current status = %d, want 200", currentRec.Code)
	}
	if !strings.Contains(currentRec.Body.String(), `"watts":5.944`) {
		t.Errorf("/current body = %s, want it to contain watts:5.944", currentRec.Body.String())
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	h.ServeHTTP(metricsRec, metricsReq)
	if !strings.Contains(metricsRec.Body.String(), "eagle_demand_watts 5.944") {
		t.Errorf("/metrics body missing eagle_demand_watts 5.944, got:\n%s", metricsRec.Body.String())
	}
}

func TestHandleUpload_MalformedBodyStillAcksAndDoesNotCrash(t *testing.T) {
	s := New(store.New())
	h := s.Routes()

	rec := postUpload(t, h, "not xml")
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status = %d, want 200 even for malformed body", rec.Code)
	}
}

func TestHandleCurrent_NoDataYet(t *testing.T) {
	s := New(store.New())
	h := s.Routes()

	req := httptest.NewRequest(http.MethodGet, "/current", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"watts":0`) {
		t.Errorf("body = %s, want zero-value watts before any upload", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"updated_at":""`) {
		t.Errorf("body = %s, want empty updated_at before any upload", rec.Body.String())
	}
}

func TestHandleHealthz(t *testing.T) {
	s := New(store.New())
	h := s.Routes()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
