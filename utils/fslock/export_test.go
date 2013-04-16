package lockdir

import (
	"time"
)

func SetLockWaitDelay(delay time.Duration) {
	lockWaitDelay = delay
}
