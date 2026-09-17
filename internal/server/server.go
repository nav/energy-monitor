// Package server wires up the HTTP endpoints: the EAGLE upload receiver, a Prometheus
// /metrics endpoint, a small JSON /current endpoint for the ESP8266, and /healthz.
package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/nav/energy-monitor/internal/eagle"
	"github.com/nav/energy-monitor/internal/store"
	"github.com/nav/energy-monitor/internal/tou"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server holds the shared state for all HTTP handlers.
type Server struct {
	store *store.Store

	registry        *prometheus.Registry
	demandGauge     prometheus.Gauge
	deliveredGauge  prometheus.Gauge
	receivedGauge   prometheus.Gauge
	priceGauge      prometheus.Gauge
	costGauge       prometheus.Gauge
	lastUploadGauge prometheus.Gauge
}

// New builds a Server backed by the given Store, with its own Prometheus registry.
func New(st *store.Store) *Server {
	registry := prometheus.NewRegistry()

	s := &Server{
		store:    st,
		registry: registry,
		demandGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "eagle_demand_kw",
			Help: "Current instantaneous demand reported by the EAGLE, in kilowatts.",
		}),
		deliveredGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "eagle_summation_delivered_kwh",
			Help: "Cumulative energy delivered from the utility to the user, in kWh.",
		}),
		receivedGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "eagle_summation_received_kwh",
			Help: "Cumulative energy received from the user by the utility (e.g. solar export), in kWh.",
		}),
		priceGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "eagle_price_per_kwh",
			Help: "Current price per kWh as reported by the meter's own PriceCluster upload. " +
				"Informational only -- it does not reflect the utility's time-of-use " +
				"adjustments, so it is not used to compute cost; see energy_tou_rate_dollars_per_kwh.",
		}),
		costGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "eagle_cost_accrued_dollars",
			Help: "Running total cost of delivered energy, priced at the rate in effect at the time of each delivered-kWh increase.",
		}),
		lastUploadGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "eagle_last_upload_timestamp_seconds",
			Help: "Unix timestamp of the last successfully decoded upload from the EAGLE.",
		}),
	}

	touRateGauge := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "energy_tou_rate_dollars_per_kwh",
		Help: "Dollars/kWh in effect right now per the utility's time-of-use schedule (package tou) -- the rate actually used to compute cost.",
	}, func() float64 {
		return tou.RatePerKWh(time.Now())
	})

	registry.MustRegister(s.demandGauge, s.deliveredGauge, s.receivedGauge, s.priceGauge, s.costGauge, s.lastUploadGauge, touRateGauge)
	return s
}

// Routes returns the HTTP handler serving all of the service's endpoints.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /eagle/upload", s.handleUpload)
	mux.Handle("GET /metrics", promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /current", s.handleCurrent)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	return mux
}

// handleUpload receives the EAGLE's push. The device sends
// Content-Type: application/x-www-form-urlencoded even though the body is XML, so the raw
// body is read directly rather than parsed as a form. Malformed or undecodable payloads
// are logged and still acknowledged with 200 OK, since the EAGLE has no documented retry
// behavior we want to trigger by returning an error status.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	rf, err := eagle.Parse(body)
	if err != nil {
		log.Printf("eagle: failed to parse upload: %v", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	now := time.Now()

	if rf.InstantaneousDemand != nil {
		kw, err := rf.InstantaneousDemand.KW()
		if err != nil {
			log.Printf("eagle: failed to decode demand: %v", err)
		} else {
			s.store.SetDemand(kw, now)
			s.demandGauge.Set(kw)
			s.lastUploadGauge.Set(float64(now.Unix()))
		}
	}

	if rf.CurrentSummationDelivered != nil {
		delivered, errD := rf.CurrentSummationDelivered.DeliveredKWh()
		received, errR := rf.CurrentSummationDelivered.ReceivedKWh()
		if errD != nil || errR != nil {
			log.Printf("eagle: failed to decode summation: delivered=%v received=%v", errD, errR)
		} else {
			s.store.SetSummation(delivered, received, now)
			s.deliveredGauge.Set(delivered)
			s.receivedGauge.Set(received)
			s.costGauge.Set(s.store.Latest().CostDollars)
			s.lastUploadGauge.Set(float64(now.Unix()))
		}
	}

	if rf.PriceCluster != nil {
		price, err := rf.PriceCluster.PricePerUnit()
		if err != nil {
			log.Printf("eagle: failed to decode price: %v", err)
		} else {
			s.store.SetPrice(price, rf.PriceCluster.CurrencyName(), rf.PriceCluster.RateLabel, now)
			s.priceGauge.Set(price)
			s.lastUploadGauge.Set(float64(now.Unix()))
		}
	}

	w.WriteHeader(http.StatusOK)
}

type currentResponse struct {
	KW           float64 `json:"kw"`
	KWhDelivered float64 `json:"kwh_delivered"`
	KWhReceived  float64 `json:"kwh_received"`
	PricePerKWh  float64 `json:"price_per_kwh"`
	Currency     string  `json:"currency"`
	RateLabel    string  `json:"rate_label"`
	CostDollars  float64 `json:"cost_dollars"`
	UpdatedAt    string  `json:"updated_at"`
}

func (s *Server) handleCurrent(w http.ResponseWriter, r *http.Request) {
	reading := s.store.Latest()
	resp := currentResponse{
		KW:           reading.KW,
		KWhDelivered: reading.KWhDelivered,
		KWhReceived:  reading.KWhReceived,
		PricePerKWh:  reading.PricePerKWh,
		Currency:     reading.Currency,
		RateLabel:    reading.RateLabel,
		CostDollars:  reading.CostDollars,
	}
	if !reading.UpdatedAt.IsZero() {
		resp.UpdatedAt = reading.UpdatedAt.UTC().Format(time.RFC3339)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("current: failed to encode response: %v", err)
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
