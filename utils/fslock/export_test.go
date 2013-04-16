package fslock

import (
	"time"
)

// SetLockWaitDelay updates the package lockWaitDelay for testing purposes.
func SetLockWaitDelay(delay time.Duration) {
	lockWaitDelay = delay
}
