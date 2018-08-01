package sync

import (
	"time"
)

// RunWithTimeout runs the specified function and returns true if it completes before timeout, else false.
func RunWithTimeout(timeout time.Duration, f func()) bool {
	ch := make(chan struct{})
	go func() {
		f()
		close(ch)
	}()
	select {
	case <-ch:
		return true
	case <-time.After(timeout):
	}
	return false
}
