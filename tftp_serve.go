package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
)

// global buffer that replaces the old ipxe_efi_x86_64 const
var ipxeX8664Efi []byte

// call this at startup, before you create the TFTP server
func LoadIPXEBinary(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading iPXE binary %q: %w", path, err)
	}

	if len(data) == 0 {
		return fmt.Errorf("iPXE binary %q is empty", path)
	}

	ipxeX8664Efi = data
	log.Printf("Loaded iPXE binary %q (%d bytes)", path, len(ipxeX8664Efi))
	return nil
}

func tftpReadHandler(filename string, rf io.ReaderFrom) error {
	mac, err := parseMACFromPath(filename)
	if err != nil {
		log.Println("Can't parse a MAC from ", filename, err)
		return err
	}

	log.Println("Serving ", filename)

	underlying_reader := bytes.NewReader(ipxeX8664Efi)

	tftpevent := TransferEvent{
		Protocol:   "tftp",
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
		tftpevent.State = "error"
		tftpevent.Error = err.Error()
		machine.Event(context.Background(), "serve_ipxe_over_tftp", tftpevent)
		log.Printf("Serving failure: %v", err)
		return err
	}

	tftpevent.State = "complete"
	machine.Event(context.Background(), "serve_ipxe_over_tftp", tftpevent)
	log.Printf("%d bytes sent for %s", n, filename)
	return nil

}
