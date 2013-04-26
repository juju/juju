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
