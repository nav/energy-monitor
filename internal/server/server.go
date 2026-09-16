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
	lastUploadGauge prometheus.Gauge
}

// New builds a Server backed by the given Store, with its own Prometheus registry.
func New(st *store.Store) *Server {
	registry := prometheus.NewRegistry()

	s := &Server{
		store:    st,
		registry: registry,
		demandGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "eagle_demand_watts",
			Help: "Current instantaneous demand reported by the EAGLE, in watts.",
		}),
		deliveredGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "eagle_summation_delivered_kwh",
			Help: "Cumulative energy delivered from the utility to the user, in kWh.",
		}),
		receivedGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "eagle_summation_received_kwh",
			Help: "Cumulative energy received from the user by the utility (e.g. solar export), in kWh.",
		}),
		lastUploadGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "eagle_last_upload_timestamp_seconds",
			Help: "Unix timestamp of the last successfully decoded upload from the EAGLE.",
		}),
	}

	registry.MustRegister(s.demandGauge, s.deliveredGauge, s.receivedGauge, s.lastUploadGauge)
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
		watts, err := rf.InstantaneousDemand.Watts()
		if err != nil {
			log.Printf("eagle: failed to decode demand: %v", err)
		} else {
			s.store.SetDemand(watts, now)
			s.demandGauge.Set(watts)
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
			s.lastUploadGauge.Set(float64(now.Unix()))
		}
	}

	w.WriteHeader(http.StatusOK)
}

type currentResponse struct {
	Watts        float64 `json:"watts"`
	KWhDelivered float64 `json:"kwh_delivered"`
	KWhReceived  float64 `json:"kwh_received"`
	UpdatedAt    string  `json:"updated_at"`
}

func (s *Server) handleCurrent(w http.ResponseWriter, r *http.Request) {
	reading := s.store.Latest()
	resp := currentResponse{
		Watts:        reading.Watts,
		KWhDelivered: reading.KWhDelivered,
		KWhReceived:  reading.KWhReceived,
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
