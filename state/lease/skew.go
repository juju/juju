// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"
)

// Skew holds information about a remote writer's idea of the current time.
type Skew struct {

	// LastWrite is the most recent remote time known to have been written
	// by the skewed writer.
	LastWrite time.Time

	// ReadAfter should be the latest known local time before LastWrite
	// was read.
	ReadAfter time.Time

	// ReadBefore should be the earliest known local time after LastWrite
	// was read.
	ReadBefore time.Time
}

// Earliest returns the earliest local time after which we can be confident
// that the remote writer will agree the supplied time is in the past.
func (skew Skew) Earliest(remote time.Time) (local time.Time) {
	if skew.isZero() {
		return remote
	}
	delta := remote.Sub(skew.LastWrite)
	return skew.ReadAfter.Add(delta)
}

// Latest returns the latest local time after which we can be confident that
// the remote writer will agree the supplied time is in the past.
func (skew Skew) Latest(remote time.Time) (local time.Time) {
	if skew.isZero() {
		return remote
	}
	delta := remote.Sub(skew.LastWrite)
	return skew.ReadBefore.Add(delta)
}

// isZero lets us shortcut Earliest and Latest when the skew represents a
// perfect unskewed clock (such as for a local writer).
func (skew Skew) isZero() bool {
	return skew.LastWrite.IsZero() && skew.ReadAfter.IsZero() && skew.ReadBefore.IsZero()
}
