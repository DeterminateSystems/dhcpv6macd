package main

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestNewRingPanicsOnBadCapacity(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic for capacity <= 0")
		}
	}()
	_ = NewRing[int](0)
}

func TestPushAndLenUntilFull(t *testing.T) {
	const N = 10
	r := NewRing[int](N)
	if r.Cap() != N {
		t.Fatalf("Cap = %d, want %d", r.Cap(), N)
	}
	for i := 0; i < N; i++ {
		r.Push(i)
		if got, want := r.Len(), i+1; got != want {
			t.Fatalf("Len after %d pushes = %d, want %d", i+1, got, want)
		}
		// Oldest is always 0 until full
		if r.At(0) != 0 {
			t.Fatalf("oldest = %d, want 0", r.At(0))
		}
		if r.At(r.Len()-1) != i {
			t.Fatalf("newest = %d, want %d", r.At(r.Len()-1), i)
		}
	}
}

func TestOverwriteOnFull(t *testing.T) {
	const N = 5
	r := NewRing[int](N)
	for i := 0; i < N; i++ {
		r.Push(i) // 0..4
	}
	if got := r.Slice(); fmt.Sprint(got) != "[0 1 2 3 4]" {
		t.Fatalf("before overwrite: %v", got)
	}

	// Push 5..9; should evict oldest each time
	for i := N; i < 2*N; i++ {
		r.Push(i)
	}

	want := []int{5, 6, 7, 8, 9}
	got := r.Slice()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("after overwrite got %v, want %v", got, want)
	}

	// Check At indexing: oldest..newest
	for i := 0; i < r.Len(); i++ {
		if r.At(i) != want[i] {
			t.Fatalf("At(%d) = %d, want %d", i, r.At(i), want[i])
		}
	}
}

func TestAtPanicsOnOutOfRange(t *testing.T) {
	r := NewRing[int](1)
	r.Push(1)
	defer func() {
		if p := recover(); p == nil {
			t.Fatalf("expected panic on At out of range")
		}
	}()
	_ = r.At(1) // Len==1, index 1 should panic
}

func TestSliceLayoutNotFull(t *testing.T) {
	r := NewRing[string](5)
	r.Push("a")
	r.Push("b")
	a, b := r.Slices()
	if len(b) != 0 {
		t.Fatalf("expected second slice to be nil/empty before full, got len=%d", len(b))
	}
	if fmt.Sprint(a) != "[a b]" {
		t.Fatalf("Slice() before full = %v", a)
	}
}

func TestSliceLayoutWhenWrapped(t *testing.T) {
	r := NewRing[int](4)
	for i := 0; i < 4; i++ {
		r.Push(i) // [0 1 2 3]
	}
	// Cause wrap by pushing 4 and 5 -> contents should be [2 3 4 5]
	r.Push(4)
	r.Push(5)

	got := r.Slice()
	want := []int{2, 3, 4, 5}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("wrapped order got %v, want %v", got, want)
	}
}

func TestGenericTypeSupport(t *testing.T) {
	type item struct {
		ID int
		S  string
	}
	r := NewRing[item](2)
	r.Push(item{1, "x"})
	r.Push(item{2, "y"})
	r.Push(item{3, "z"}) // evicts {1,"x"}
	got := r.Slice()
	want := []item{{2, "y"}, {3, "z"}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("generic items got %v, want %v", got, want)
	}
}

func TestJsonEmpty(t *testing.T) {
	r := NewRing[int](3)
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshall: %v", err)
	}
	if string(b) != "[]" {
		t.Fatalf("Wanted [], got %s", string(b))
	}
}

func TestJsonPartial(t *testing.T) {
	r := NewRing[int](3)
	r.Push(1)
	r.Push(2)
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshall: %v", err)
	}
	if string(b) != "[1,2]" {
		t.Fatalf("Wanted [1,2], got %s", string(b))
	}
}

func TestJsonFull(t *testing.T) {
	r := NewRing[int](3)
	r.Push(1)
	r.Push(2)
	r.Push(3)
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshall: %v", err)
	}
	if string(b) != "[1,2,3]" {
		t.Fatalf("Wanted [1,2,3], got %s", string(b))
	}
}

func TestJsonOverflowed(t *testing.T) {
	r := NewRing[int](3)
	r.Push(1)
	r.Push(2)
	r.Push(3)
	r.Push(4)
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshall: %v", err)
	}
	if string(b) != "[2,3,4]" {
		t.Fatalf("Wanted [2,3,4], got %s", string(b))
	}
}

// Example for documentation (shown by `go test`).
func ExampleRing() {
	r := NewRing[int](3)
	for _, v := range []int{10, 11, 12, 13} {
		r.Push(v)
	}
	// Contents are [11 12 13] (10 was evicted)
	fmt.Println(r.Slice())
	// Output:
	// [11 12 13]
}

func BenchmarkPush(b *testing.B) {
	const N = 1_000_000
	r := NewRing[int](N)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Push(i)
	}
}
