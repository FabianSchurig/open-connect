// Package clock provides an interface so that time-dependent code (claim TTLs,
// preparation timeouts) is testable under deterministic clocks without sleeping.
package clock

import "time"

type Clock interface {
	Now() time.Time
}

type Real struct{}

func (Real) Now() time.Time { return time.Now().UTC() }

// Fixed is a test clock; mutate via Set.
type Fixed struct{ T time.Time }

func (f *Fixed) Now() time.Time   { return f.T }
func (f *Fixed) Set(t time.Time)  { f.T = t.UTC() }
func (f *Fixed) Add(d time.Duration) { f.T = f.T.Add(d) }
