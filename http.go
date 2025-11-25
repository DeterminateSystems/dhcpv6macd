package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
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

func webserver(netbootDir string, b *Broker, m *Machines) (*http.ServeMux, error) {
	server := http.NewServeMux()

	if netbootDir == "" {
		log.Printf("netboot directory was not specified, will not serve it: %s", netbootDir)
	} else if _, err := os.Stat(netbootDir); os.IsNotExist(err) {
		log.Printf("netboot directory does not exist, will not serve it: %s", netbootDir)
	} else {
		server.HandleFunc("/mac/{mac_addr}/boot.efi", func(w http.ResponseWriter, r *http.Request) {
			// Extract MAC address from path
			macStr := r.PathValue("mac_addr")
			mac, err := net.ParseMAC(macStr)
			if err != nil {
				log.Printf("Got request with something that didn't look like a MAC address. Returning 404.")
				http.NotFound(w, r)
				return
			}

			name := path.Clean(path.Join(netbootDir, mac.String(), "boot.efi"))
			log.Println("?", name)

			if !strings.HasPrefix(name, netbootDir+"/") {
				log.Println("Path did not clean to subordinate of", netbootDir)
				http.Error(w, "fishy path", 400)
				return
			}

			f, err := os.Open(name)
			if err != nil {
				msg, code := toHTTPError(err)
				http.Error(w, msg, code)
				return
			}
			defer f.Close()

			stat, err := f.Stat()
			if err != nil {
				msg, code := toHTTPError(err)
				http.Error(w, msg, code)
				return
			}

			if stat.IsDir() {
				http.Error(w, "boot.efi was a directory", 500)
				return
			}

			w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
			w.Header().Set("Content-Type", "application/octet-stream")

			machine := m.GetOrInitMachine(mac)

			event := TransferEvent{
				Filename:   name,
				State:      "init",
				TotalBytes: stat.Size(),
			}

			if r.TLS == nil {
				event.Protocol = "http"
			} else {
				event.Protocol = "https"
			}

			// Trigger state transition to http_fetch_uki
			if err := machine.Event(r.Context(), "http_fetch_uki", event); err != nil {
				log.Printf("Failed to transition to http_fetch_uki for %s: %v", macStr, err)
			}

			event.State = "sending"

			reader := newProgressReader(f, func(bytes int64) error {
				event.SentBytes = bytes
				machine.Event(context.Background(), "http_fetch_uki", event)
				return nil
			})

			_, err = io.Copy(w, reader)

			if err != nil {
				event.Error = err
				machine.Event(context.Background(), "http_fetch_uki", event)
				log.Printf("Serving failure: %v", err)
			} else {
				event.State = "complete"
				machine.Event(context.Background(), "http_fetch_uki", event)
			}
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

				if macStr == "" || msg.Mac.String() == "" || msg.Mac.String() == mac.String() {
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

// Following was lifted from net/http:
//
// toHTTPError returns a non-specific HTTP error message and status code
// for a given non-nil error value. It's important that toHTTPError does not
// actually return err.Error(), since msg and httpStatus are returned to users,
// and historically Go's ServeContent always returned just "404 Not Found" for
// all errors. We don't want to start leaking information in error messages.
func toHTTPError(err error) (msg string, httpStatus int) {
	if errors.Is(err, fs.ErrNotExist) {
		return "404 page not found", http.StatusNotFound
	}
	if errors.Is(err, fs.ErrPermission) {
		return "403 Forbidden", http.StatusForbidden
	}

	// Default:
	return "500 Internal Server Error", http.StatusInternalServerError
}
