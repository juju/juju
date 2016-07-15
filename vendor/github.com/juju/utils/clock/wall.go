// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clock

import (
	"time"
)

// WallClock exposes wall-clock time via the Clock interface.
var WallClock wallClock

// WallClock exposes wall-clock time as returned by time.Now.
type wallClock struct{}

// Now is part of the Clock interface.
func (wallClock) Now() time.Time {
	return time.Now()
}

// Alarm returns a channel that will send a value at some point after
// the supplied time.
func (wallClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

func (wallClock) AfterFunc(d time.Duration, f func()) Timer {
	return time.AfterFunc(d, f)
}
