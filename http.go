package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

func webserver(addr string, b *Broker, m *Machines) error {
	// SSE endpoint
	http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		// Mandatory SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		// CORS (optional; useful when testing from other origins)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// Tell client to retry in 3s if disconnected
		fmt.Fprint(w, "retry: 3000\n\n")
		data, err := json.Marshal(m)
		if err != nil {
			log.Println("JSON marshalling error: ", err)
		} else {
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			flusher.Flush()
		}
		flusher.Flush()

		// Subscribe this client
		ch, unsubscribe := b.Subscribe()
		defer unsubscribe()

		// Heartbeats to keep connections alive through proxies
		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeat.C:
				// comment lines are ignored by EventSource but keep the pipe warm
				fmt.Fprint(w, ": ping\n\n")
				flusher.Flush()
			case msg, ok := <-ch:
				if !ok {
					return
				}

				data, err := json.Marshal(msg)
				if err != nil {
					log.Println("JSON marshalling error: ", err)
				} else {
					fmt.Fprintf(w, "data: %s\n\n", string(data))
					flusher.Flush()
				}
			}
		}
	})

	log.Printf("SSE server listening on %s", addr)
	return http.ListenAndServe(addr, nil)
}
