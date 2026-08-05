package backoff

import (
	"math"
	"time"
)

// Exp is a simple exponential backoff with a hard cap.
type Exp struct {
	Initial time.Duration
	Max     time.Duration
	Factor  float64
	attempt int
}

func New(initial, max time.Duration) *Exp {
	if initial <= 0 {
		initial = time.Second
	}
	if max < initial {
		max = initial
	}
	return &Exp{Initial: initial, Max: max, Factor: 2}
}

func (e *Exp) Next() time.Duration {
	if e.attempt == 0 {
		e.attempt = 1
		return e.Initial
	}
	d := float64(e.Initial) * math.Pow(e.Factor, float64(e.attempt-1))
	e.attempt++
	if d > float64(e.Max) {
		return e.Max
	}
	return time.Duration(d)
}

func (e *Exp) Reset() { e.attempt = 0 }
