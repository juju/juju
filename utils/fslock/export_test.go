// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fslock

import (
	"time"
)

// SetLockWaitDelay updates the package lockWaitDelay for testing purposes.
func SetLockWaitDelay(delay time.Duration) time.Duration {
	oldValue := lockWaitDelay
	lockWaitDelay = delay
	return oldValue
}
