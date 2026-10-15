package core

// The clock and the one-shot timers behind the privacy promises (the counterpart of
// Core/Scheduler.swift): the idle reset of the keystroke buffer, the expiry of the undo list.
// An interface, so tests drive time by hand instead of sleeping.

import "time"

// Timer is a one-shot timer that can be called off.
type Timer interface {
	// Stop prevents the callback from running if it has not started. It reports whether it
	// did.
	Stop() bool
}

// Scheduler is the clock and timer source.
type Scheduler interface {
	// Now is monotonic time in seconds; only differences mean anything.
	Now() float64
	// AfterFunc calls f once, d from now, on a goroutine of its own. Callers that are not safe
	// for concurrent use (the engine) must hand the work over to their own goroutine from f.
	AfterFunc(d time.Duration, f func()) Timer
}

// RealScheduler is the system clock and time.AfterFunc.
type RealScheduler struct{}

var processStart = time.Now()

// Now is seconds since the process started, on the monotonic clock.
func (RealScheduler) Now() float64 { return time.Since(processStart).Seconds() }

func (RealScheduler) AfterFunc(d time.Duration, f func()) Timer { return time.AfterFunc(d, f) }
