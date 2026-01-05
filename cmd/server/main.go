package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"counter/internal/loki"
)

//go:embed static
var staticFiles embed.FS

var (
	lokiClient *loki.Client
	labels     = map[string]string{"app": "counter"}
)

func main() {
	lokiURL := os.Getenv("LOKI_URL")
	if lokiURL == "" {
		lokiURL = "http://loki:3100"
	}

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	lokiClient = loki.NewClient(lokiURL)

	staticFS, _ := fs.Sub(staticFiles, "static")
	http.Handle("GET /", http.FileServer(http.FS(staticFS)))
	http.HandleFunc("POST /increment", handleIncrement)
	http.HandleFunc("GET /count", handleCount)
	http.HandleFunc("GET /health", handleHealth)

	log.Printf("starting server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func handleIncrement(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	line := fmt.Sprintf("increment ip=%s", ip)
	if err := lokiClient.Push(labels, time.Now(), line); err != nil {
		log.Printf("failed to push increment: %v", err)
		http.Error(w, "failed to record increment", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := len(xff); idx > 0 {
			for i, c := range xff {
				if c == ',' {
					return xff[:i]
				}
			}
			return xff
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func handleCount(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	count, err := lokiClient.CountSince(labels, startOfDay)
	if err != nil {
		log.Printf("failed to query count: %v", err)
		http.Error(w, "failed to get count", http.StatusInternalServerError)
		return
	}

	lastTs, err := lokiClient.LastTimestamp(labels)
	if err != nil {
		log.Printf("failed to query last timestamp: %v", err)
		http.Error(w, "failed to get last timestamp", http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{"count": count}
	if !lastTs.IsZero() {
		resp["last"] = lastTs.Format(time.RFC3339)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
