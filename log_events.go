package main

import (
	"io"
)

type FyiHookWriter struct {
	w      io.Writer
	copyTo func(msg string)
}

func (h *FyiHookWriter) Write(p []byte) (int, error) {
	if h.copyTo != nil {
		h.copyTo(string(p))
	}

	if h.w == nil {
		return len(p), nil
	}

	return h.w.Write(p)
}
