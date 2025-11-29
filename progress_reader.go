package main

import "io"

type TransferEvent struct {
	Protocol   string `json:"protocol"`
	Filename   string `json:"file_name"`
	State      string `json:"state"`
	SentBytes  int64  `json:"sent_bytes"`
	TotalBytes int64  `json:"total_bytes"`
	Error      string `json:"error,omitempty"`
}

const fiveMiB = 5 * 1024 * 1024

type progressReader struct {
	r          io.Reader
	total      int64
	nextMark   int64
	firedFinal bool
	onMark     func(bytes int64) error
}

func newProgressReader(r io.Reader, onMark func(bytes int64) error) *progressReader {
	return &progressReader{
		r:        r,
		nextMark: fiveMiB,
		onMark:   onMark,
	}
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.total += int64(n)

		for p.total >= p.nextMark {
			if err := p.onMark(p.total); err != nil {
				return n, err
			}
			p.nextMark += fiveMiB
		}
	}

	// If we've hit EOF, emit one final progress event
	if err == io.EOF && !p.firedFinal {
		p.firedFinal = true

		if err := p.onMark(p.total); err != nil {
			return n, err
		}
	}

	return n, err
}
