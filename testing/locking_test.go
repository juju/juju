// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"errors"
	"sync"

	gc "launchpad.net/gocheck"
)

type LockingSuite struct{}

var _ = gc.Suite(&LockingSuite{})

func (LockingSuite) TestTestLockingFunctionPassesCorrectLock(c *gc.C) {
	lock := sync.Mutex{}
	function := func() {
		lock.Lock()
		lock.Unlock()
	}
	// TestLockingFunction succeeds.
	TestLockingFunction(&lock, function)
}

func (LockingSuite) TestTestLockingFunctionDetectsDisobeyedLock(c *gc.C) {
	lock := sync.Mutex{}
	function := func() {}
	c.Check(
		func() { TestLockingFunction(&lock, function) },
		gc.Panics,
		errors.New("function did not obey lock"))
}

func (LockingSuite) TestTestLockingFunctionDetectsFailureToReleaseLock(c *gc.C) {
	lock := sync.Mutex{}
	defer lock.Unlock()
	function := func() {
		lock.Lock()
	}
	c.Check(
		func() { TestLockingFunction(&lock, function) },
		gc.Panics,
		errors.New("function did not release lock"))
}
