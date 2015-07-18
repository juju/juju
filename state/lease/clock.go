// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"
)

// SystemClock exposes wall-clock time as returned by time.Now.
type SystemClock struct{}

// Now is part of the Clock interface.
func (SystemClock) Now() time.Time {
	return time.Now()
}

// Alarm returns a channel that will send a value at some point after
// the supplied time.
func (clock SystemClock) Alarm(t time.Time) <-chan time.Time {
	return time.After(t.Sub(clock.Now()))
}
