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

// updateStatusSignal will fire every statusPollInterval
func (u *Uniter) updateStatusSignal(now, lastSignal time.Time, interval time.Duration) <-chan time.Time {
	return updateStatusTimer(now, lastSignal, interval)
}

// updateStatusTimer is a separate function to ease testing
var updateStatusTimer = func(now, lastSignal time.Time, interval time.Duration) <-chan time.Time {
	waitDuration := interval - now.Sub(lastSignal)
	return time.After(waitDuration)
}
