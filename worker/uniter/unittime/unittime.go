// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unittime

import (
	"time"
)

var (
	timeNow = time.Now
)

// TODO (mattyw) unexport UnitTimeCounter
// UnitTimeCounter is used to count how long a unit has been running
// for the sake of generating the unit time metric
type UnitTimeCounter struct {
	start       time.Time
	elapsedTime time.Duration
	running     bool
}

// Value returns the current duration the timer has been running
func (u *UnitTimeCounter) Value() time.Duration {
	if u.running {
		now := timeNow().UTC()
		return u.elapsedTime + now.Sub(u.start)
	}
	return u.elapsedTime
}

// Stop stops the counter from running
func (u *UnitTimeCounter) Stop() {
	now := timeNow().UTC()
	u.elapsedTime += now.Sub(u.start)
	u.running = false
}

// Start starts the timer if it isn't running.
// If the timer is already running it is noop
func (u *UnitTimeCounter) Start() {
	if !u.running {
		u.start = timeNow().UTC()
		u.running = true
	}
}

// Running returns true if the timer is currently running,
// otherwise false
func (u *UnitTimeCounter) Running() bool {
	return u.running
}
