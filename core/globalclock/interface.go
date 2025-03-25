// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclock

import (
	"time"

	"github.com/juju/juju/internal/errors"
)

var (
	// ErrOutOfSyncUpdate is returned by Updater.Advance when the
	// clock value has been changed since the last read.
	ErrOutOfSyncUpdate = errors.ConstError("clock update attempt by out-of-sync caller, retry")

	// ErrTimeout is returned by Updater.Advance if the attempt to
	// update the global clock timed out - in that case the advance
	// should be tried again.
	ErrTimeout = errors.ConstError("clock update timed out, retry")
)

// Updater provides a means of updating the global clock time.
type Updater interface {
	// Advance adds the given duration to the global clock, ensuring
	// that the clock has not been updated concurrently.
	//
	// Advance will return ErrOutOfSyncUpdate an attempt is made to advance the
	// clock from a last known time not equal to the authoritative global time.
	// In this case, the updater will refresh its view of the clock,
	// and the caller can attempt Advance later.
	//
	// If Advance returns any error other than ErrOutOfSyncUpdate or
	// ErrTimeout the Updater should be considered invalid, and the
	// caller should obtain a new Updater. Failing to do so could lead
	// to non-monotonic time, since there is no way of knowing in
	// general whether or not the clock was updated.
	Advance(d time.Duration, stop <-chan struct{}) error
}
