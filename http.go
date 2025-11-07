package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type neuteredFileSystem struct {
	fs http.FileSystem
}

func (nfs neuteredFileSystem) Open(path string) (http.File, error) {
	f, err := nfs.fs.Open(path)
	if err != nil {
		return nil, err
	}

	s, err := f.Stat()
	if err != nil {
		return nil, err
	}

	if s.IsDir() {
		index := filepath.Join(path, "index.html")
		if _, err := nfs.fs.Open(index); err != nil {
			closeErr := f.Close()
			if closeErr != nil {
				return nil, closeErr
			}

			return nil, err
		}
	}

	return f, nil
}

func webserver(addr string, netbootDir string, b *Broker, m *Machines) (*http.ServeMux, error) {
	server := http.NewServeMux()

	if netbootDir == "" {
		fmt.Printf("netboot directory was not specified, will not serve it: %s", netbootDir)
	} else if _, err := os.Stat(netbootDir); os.IsNotExist(err) {
		fmt.Printf("netboot directory does not exist, will not serve it: %s", netbootDir)
	} else {
		fs := http.FileServer(neuteredFileSystem{http.Dir(netbootDir)})
		server.HandleFunc("/mac/{mac_addr}/boot.efi", func(w http.ResponseWriter, r *http.Request) {
			// Extract MAC address from path
			macStr := r.PathValue("mac_addr")
			mac, err := net.ParseMAC(macStr)
			if err != nil {
				log.Printf("Got request with something that didn't look like a MAC address. Permitting anyway.")
			} else {
				machine := m.GetOrInitMachine(mac)

				// Trigger state transition to http_fetch_uki
				if err := machine.Event(r.Context(), "http_fetch_uki"); err != nil {
					log.Printf("Failed to transition to http_fetch_uki for %s: %v", macStr, err)
				}
			}

			http.StripPrefix("/mac/", fs).ServeHTTP(w, r)
		})
	}

	// SSE endpoint
	server.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.Query()
		macStr := params.Get("mac")

		mac, err := net.ParseMAC(macStr)
		if macStr != "" && err != nil {
			http.Error(w, fmt.Sprintf("MAC error: %v", err), http.StatusBadRequest)
			return
		}

		// Mandatory SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// Tell client to retry in 3s if disconnected
		if _, err = fmt.Fprint(w, "retry: 3000\n\n"); err != nil {
			return
		}

		if macStr == "" {
			data, err := json.Marshal(m)
			if err != nil {
				log.Println("JSON marshalling error: ", err)
			} else {
				if _, err = fmt.Fprintf(w, "data: %s\n\n", string(data)); err != nil {
					return
				}
			}
		} else {
			data, err := json.Marshal(machines.GetMachine(mac))
			if err != nil {
				log.Println("JSON marshalling error: ", err)
			} else {
				if _, err := fmt.Fprintf(w, "data: %s\n\n", string(data)); err != nil {
					return
				}
			}
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
				if _, err = fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
					return
				}
				flusher.Flush()
			case msg, ok := <-ch:
				if !ok {
					return
				}

				if macStr == "" || msg.Mac.String() == mac.String() {
					data, err := json.Marshal(msg)
					if err != nil {
						log.Println("JSON marshalling error: ", err)
					} else {
						if _, err = fmt.Fprintf(w, "data: %s\n\n", string(data)); err != nil {
							return
						}
						flusher.Flush()
					}
				}
			}
		}
	})

	return server, nil
}
