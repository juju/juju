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
func updateStatusSignal() <-chan time.Time {
	return time.After(statusPollInterval)
}

// NewUpdateStatusTimer returns a timed signal suitable for update-status hook.
func NewUpdateStatusTimer() func() <-chan time.Time {
	return updateStatusSignal
}
