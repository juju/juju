// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclock

import (
	"time"

	"github.com/juju/errors"
)

var (
	// ErrConcurrentUpdate is returned by Updater.AddTime when the
	// clock value has been changed since the last read.
	ErrConcurrentUpdate = errors.New("clock was updated concurrently, retry")
)

// Updater provides a means of updating the global clock time.
type Updater interface {
	// AddTime adds the given duration to the global clock, ensuring
	// that the clock has not been updated concurrently.
	//
	// AddTime will return ErrConcurrentUpdate if another updater
	// updates the clock concurrently. In this case, the updater
	// will refresh its view of the clock, and the caller can
	// attempt AddTime later.
	//
	// If AddTime returns any error other than ErrConcurrentUpdate,
	// the Updater should be considered invalid, and the caller
	// should obtain a new Updater. Failing to do so could lead
	// to non-monotonic time, since there is no way of knowing in
	// general whether or not the database was updated.
	AddTime(d time.Duration) error
}
