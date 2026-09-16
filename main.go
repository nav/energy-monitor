// Command energy-monitor receives push notifications from a Rainforest EAGLE (RFA-Z109)
// energy monitor and exposes the readings as Prometheus metrics and a small JSON API.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/nav/energy-monitor/internal/server"
	"github.com/nav/energy-monitor/internal/store"
)

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := server.New(store.New())

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      srv.Routes(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	log.Printf("energy-monitor listening on %s", addr)
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
