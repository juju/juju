// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"math/rand"
	"time"

	"github.com/juju/juju/internal/worker/uniter/remotestate"
)

type waitDuration time.Duration

func (w waitDuration) After() <-chan time.Time {
	// TODO(fwereade): 2016-03-17 lp:1558657
	return time.After(time.Duration(w))
}

// NewUpdateStatusTimer returns a func returning timed signal suitable for update-status hook.
func NewUpdateStatusTimer() remotestate.UpdateStatusTimerFunc {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	return func(wait time.Duration) remotestate.Waiter {
		// Actual time to wait is randomised to be +/-20%
		// of the nominal value.
		lower := 0.8 * float64(wait)
		window := 0.4 * float64(wait)
		offset := float64(r.Int63n(int64(window)))
		wait = time.Duration(lower + offset)

		return waitDuration(wait)
	}
}
