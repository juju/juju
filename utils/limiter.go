// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
)

type empty struct {}
type limiter chan empty

func NewLimiter(max int) limiter {
	return make(limiter, max)
}

// Acquire requests some resources that you can return later
// It returns 'true' if there are resources available, but false if they are
// not. Callers are responsible for calling Release if this returns true, but
// should not release if this returns false
func (l limiter) Acquire() bool {
	e := empty{}
	select {
		case l <- e:
		  return true
		default:
		  return false
	}
}

func (l limiter) Release() error {
	select {
		case <- l:
		  return nil
		default:
		  return fmt.Errorf("Release without an associated Acquire")
	}
}


