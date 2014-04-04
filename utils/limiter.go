// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
)

type empty struct{}
type limiter chan empty

// Limiter represents a limited resource (eg a semaphore).
type Limiter interface {
	// Acquire another unit of the resource.
	// Acquire returns false to indicate there is no more availability,
	// until another entity calls Release.
	Acquire() bool
	// AcquireWait requests a unit of resource, but blocks until one is
	// available.
	AcquireWait()
	// Release returns a unit of the resource. Calling Release when there
	// are no units Acquired is an error.
	Release() error
}

func NewLimiter(max int) Limiter {
	return make(limiter, max)
}

// Acquire requests some resources that you can return later
// It returns 'true' if there are resources available, but false if they are
// not. Callers are responsible for calling Release if this returns true, but
// should not release if this returns false.
func (l limiter) Acquire() bool {
	e := empty{}
	select {
	case l <- e:
		return true
	default:
		return false
	}
}

// AcquireWait waits for the resource to become available before returning.
func (l limiter) AcquireWait() {
	e := empty{}
	l <- e
}

// Release returns the resource to the available pool.
func (l limiter) Release() error {
	select {
	case <-l:
		return nil
	default:
		return fmt.Errorf("Release without an associated Acquire")
	}
}
