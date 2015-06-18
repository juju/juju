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
	logger.Debugf("status hook waiting for %v", waitDuration)
	return time.After(waitDuration)
}
