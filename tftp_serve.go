package main

import (
	"bytes"
	"context"
	"io"
	"log"
)

type ServeIPXEOverTFTPEvent struct {
	Filename   string `json:"file_name"`
	State      string `json:"state"`
	SentBytes  int64  `json:"sent_bytes"`
	TotalBytes int64  `json:"total_bytes"`
	Error      error  `json:"error"`
}

func tftpReadHandler(filename string, rf io.ReaderFrom) error {
	mac, err := parseMACFromPath(filename)
	if err != nil {
		log.Println("Can't parse a MAC from ", filename, err)
		return err
	}

	log.Println("Serving ", filename)

	underlying_reader := bytes.NewReader(ipxe_efi_x86_64)

	tftpevent := ServeIPXEOverTFTPEvent{
		Filename:   filename,
		State:      "init",
		TotalBytes: underlying_reader.Size(),
		SentBytes:  0,
	}

	machine := machines.GetOrInitMachine(mac)
	machine.Event(context.Background(), "serve_ipxe_over_tftp", tftpevent)

	tftpevent.State = "sending"

	r := newProgressReader(underlying_reader, func(bytes int64) error {
		tftpevent.SentBytes = bytes
		machine.Event(context.Background(), "serve_ipxe_over_tftp", tftpevent)
		return nil
	})
	n, err := rf.ReadFrom(r)
	if err != nil {
		tftpevent.Error = err
		machine.Event(context.Background(), "serve_ipxe_over_tftp", tftpevent)
		log.Printf("Serving failure: %v", err)
		return err
	}

	tftpevent.State = "complete"
	machine.Event(context.Background(), "serve_ipxe_over_tftp", tftpevent)
	log.Printf("%d bytes sent for %s", n, filename)
	return nil

}
