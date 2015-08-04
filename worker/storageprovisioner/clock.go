// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import "time"

// Clock is an interface for dealing with time, without relying directly
// on one particular definition of time.
type Clock interface {
	// Now returns the current time.
	Now() time.Time

	// After waits for the duration to elapse and then sends the current
	// time on the returned channel.
	After(time.Duration) <-chan time.Time
}

type wallClock struct{}

var WallClock Clock = wallClock{}

func (wallClock) Now() time.Time {
	return time.Now()
}

func (wallClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}
