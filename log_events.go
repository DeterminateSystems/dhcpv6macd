package main

import (
	"io"
)

type FyiHookWriter struct {
	w      io.Writer
	copyTo func(msg string)
}

func (h *FyiHookWriter) Write(p []byte) (int, error) {
	h.copyTo(string(p))
	return h.w.Write(p)
}
