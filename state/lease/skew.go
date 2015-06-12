// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"
)

// skew holds information about a remote writer's idea of the current time.
type Skew struct {
	// LastWrite is the most recent time known to have been written by
	// the skewed writer.
	LastWrite time.Time

	// ReadAfter is the earliest local time after which the skewed writer agrees
	// the time is no earlier than LastWrite.
	ReadAfter time.Time

	// ReadBefore is the latest local time after which the skewed writer agrees
	// the time is no earlier than LastWrite.
	ReadBefore time.Time
}

// Earliest returns the earliest local time at which we're confident the skewed
// writer will NOT have passed the supplied remote time.
func (skew Skew) Earliest(remote time.Time) (local time.Time) {
	delta := remote.Sub(skew.LastWrite)
	return skew.ReadAfter.Add(delta)
}

// Latest returns the latest local time at which we're confident the skewed
// writer will have passed the supplied remote time.
func (skew Skew) Latest(remote time.Time) (local time.Time) {
	delta := remote.Sub(skew.LastWrite)
	return skew.ReadBefore.Add(delta)
}
