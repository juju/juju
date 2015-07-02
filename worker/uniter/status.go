// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"time"
)

const (
	// interval at which the unit's status should be polled
	statusPollInterval = 5 * time.Minute
)

// updateStatusSignal returns a time channel that fires after a given interval.
func updateStatusSignal(now, lastSignal time.Time, interval time.Duration) <-chan time.Time {
	waitDuration := interval - now.Sub(lastSignal)
	return time.After(waitDuration)
}

// NewUpdateStatusTimer returns a timed signal suitable for update-status hook.
func NewUpdateStatusTimer() TimedSignal {
	return updateStatusSignal
}
