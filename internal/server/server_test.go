package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nav/energy-monitor/internal/store"
	"github.com/nav/energy-monitor/internal/tou"
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
	if !strings.Contains(currentRec.Body.String(), `"kw":5.944`) {
		t.Errorf("/current body = %s, want it to contain kw:5.944", currentRec.Body.String())
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	h.ServeHTTP(metricsRec, metricsReq)
	if !strings.Contains(metricsRec.Body.String(), "eagle_demand_kw 5.944") {
		t.Errorf("/metrics body missing eagle_demand_kw 5.944, got:\n%s", metricsRec.Body.String())
	}
}

const priceClusterFixture = `<?xml version="1.0"?>
<rainforest macId="0xd8d5b90019dc" version="undefined" timestamp="1789531271s">
<PriceCluster>
<DeviceMacId>0xd8d5b900000032a1</DeviceMacId>
<MeterMacId>0x0007810000b3b716</MeterMacId>
<Price>0x00000449</Price>
<Currency>0x007c</Currency>
<TrailingDigits>0x04</TrailingDigits>
<Tier>0x01</Tier>
<RateLabel>Block 1</RateLabel>
</PriceCluster>
</rainforest>`

func summationFixture(deliveredHex string) string {
	return `<?xml version="1.0"?>
<rainforest macId="0xd8d5b90019dc" version="undefined" timestamp="1789531271s">
<CurrentSummationDelivered>
<DeviceMacId>0xd8d5b900000032a1</DeviceMacId>
<MeterMacId>0x0007810000b3b716</MeterMacId>
<SummationDelivered>` + deliveredHex + `</SummationDelivered>
<SummationReceived>0x00000000</SummationReceived>
<Multiplier>0x00000001</Multiplier>
<Divisor>0x000003e8</Divisor>
</CurrentSummationDelivered>
</rainforest>`
}

func TestHandleUpload_PriceCluster_UpdatesCurrentAndMetrics(t *testing.T) {
	s := New(store.New())
	h := s.Routes()

	rec := postUpload(t, h, priceClusterFixture)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status = %d, want 200", rec.Code)
	}

	currentReq := httptest.NewRequest(http.MethodGet, "/current", nil)
	currentRec := httptest.NewRecorder()
	h.ServeHTTP(currentRec, currentReq)
	body := currentRec.Body.String()
	if !strings.Contains(body, `"price_per_kwh":0.1097`) {
		t.Errorf("/current body = %s, want price_per_kwh:0.1097", body)
	}
	if !strings.Contains(body, `"currency":"CAD"`) {
		t.Errorf("/current body = %s, want currency:CAD", body)
	}
	if !strings.Contains(body, `"rate_label":"Block 1"`) {
		t.Errorf("/current body = %s, want rate_label:Block 1", body)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	h.ServeHTTP(metricsRec, metricsReq)
	if !strings.Contains(metricsRec.Body.String(), "eagle_price_per_kwh 0.1097") {
		t.Errorf("/metrics body missing eagle_price_per_kwh 0.1097, got:\n%s", metricsRec.Body.String())
	}
}

func TestHandleUpload_CostAccruesAcrossSummationUpdatesAtTOURate(t *testing.T) {
	s := New(store.New())
	h := s.Routes()

	// Posting a (deliberately wrong-looking) PriceCluster first shouldn't affect cost --
	// see TestHandleUpload_CostAccrualIgnoresDeviceReportedPrice below for the explicit
	// version of this check.
	postUpload(t, h, priceClusterFixture)
	postUpload(t, h, summationFixture("0x000186A0")) // 100000 raw / 1000 divisor = 100 kWh baseline
	postUpload(t, h, summationFixture("0x0001D4C0")) // 120000 raw / 1000 divisor = 120 kWh (+20 kWh)

	// handleUpload prices each delta at tou.RatePerKWh(time.Now()) internally, so the
	// expected cost has to be computed the same way rather than hardcoded -- the actual
	// rate depends on which TOU window the test happens to run in.
	wantCost := 20.0 * tou.RatePerKWh(time.Now())
	wantLine := fmt.Sprintf("eagle_cost_accrued_dollars %v", wantCost)

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	h.ServeHTTP(metricsRec, metricsReq)
	if !strings.Contains(metricsRec.Body.String(), wantLine) {
		t.Errorf("/metrics body missing %q, got:\n%s", wantLine, metricsRec.Body.String())
	}
}

func TestHandleUpload_CostAccrualIgnoresDeviceReportedPrice(t *testing.T) {
	withDevicePrice := New(store.New())
	hWith := withDevicePrice.Routes()
	postUpload(t, hWith, priceClusterFixture) // $0.1097/kWh -- should have no effect below
	postUpload(t, hWith, summationFixture("0x000186A0"))
	postUpload(t, hWith, summationFixture("0x0001D4C0"))

	withoutDevicePrice := New(store.New())
	hWithout := withoutDevicePrice.Routes()
	postUpload(t, hWithout, summationFixture("0x000186A0"))
	postUpload(t, hWithout, summationFixture("0x0001D4C0"))

	got := withDevicePrice.store.Latest().CostDollars
	want := withoutDevicePrice.store.Latest().CostDollars
	if got != want {
		t.Errorf("CostDollars with a PriceCluster upload = %v, want %v (same as without one)", got, want)
	}
}

func TestHandleUpload_TOURateGaugeReflectsCurrentRate(t *testing.T) {
	s := New(store.New())
	h := s.Routes()

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	h.ServeHTTP(metricsRec, metricsReq)

	wantLine := fmt.Sprintf("energy_tou_rate_dollars_per_kwh %v", tou.RatePerKWh(time.Now()))
	if !strings.Contains(metricsRec.Body.String(), wantLine) {
		t.Errorf("/metrics body missing %q, got:\n%s", wantLine, metricsRec.Body.String())
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
	if !strings.Contains(rec.Body.String(), `"kw":0`) {
		t.Errorf("body = %s, want zero-value kw before any upload", rec.Body.String())
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
