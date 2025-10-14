package main

import (
	"encoding/json"
)

// Ring is a fixed-size FIFO queue with overwrite-on-full semantics.
type Ring[T any] struct {
	buf  []T
	head int
	size int
}

func (r *Ring[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.Slice())
}

// NewRing creates a ring with the given capacity.
func NewRing[T any](capacity int) *Ring[T] {
	if capacity <= 0 {
		panic("capacity must be > 0")
	}
	return &Ring[T]{buf: make([]T, 0, capacity)}
}

func (r *Ring[T]) Cap() int {
	return cap(r.buf)
}

func (r *Ring[T]) Len() int {
	return r.size
}

// Push appends x; if full, it overwrites (and evicts) the oldest element.
func (r *Ring[T]) Push(x T) {
	if r.size < cap(r.buf) {
		r.buf = append(r.buf, x)
		r.size++
		return
	}

	// Overwrite oldest and advance head.
	r.buf[r.head] = x
	r.head++
	if r.head == cap(r.buf) {
		r.head = 0
	}
}

// At returns the i-th element in logical order [0..Len()-1],
// where 0 is the oldest and Len()-1 is the newest.
func (r *Ring[T]) At(i int) T {
	if i < 0 || i >= r.size {
		panic("index out of range")
	}
	physical := r.head + i
	if r.size < cap(r.buf) {
		// Not yet wrapped; head is 0.
		return r.buf[physical]
	}
	if physical >= cap(r.buf) {
		physical -= cap(r.buf)
	}
	return r.buf[physical]
}

// Slice returns a view of the data in logical order as two slices.
// Join them if you really need one contiguous slice.
func (r *Ring[T]) Slices() (a, b []T) {
	if r.size == 0 {
		return nil, nil
	}
	if r.size < cap(r.buf) {
		return r.buf[:r.size], make([]T, 0)
	}

	a = r.buf[r.head:]
	b = r.buf[:r.head]
	return
}

// Slice returns a view of the data in logical order as two slices.
// Join them if you really need one contiguous slice.
func (r *Ring[T]) Slice() (a []T) {
	a, b := r.Slices()

	out := make([]T, 0, r.Len())
	out = append(out, a...)
	out = append(out, b...)

	return out
}
