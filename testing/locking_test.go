// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"errors"
	"sync"

	. "launchpad.net/gocheck"
)

type LockingSuite struct{}

var _ = Suite(&LockingSuite{})

func (LockingSuite) TestTestLockingFunctionPassesCorrectLock(c *C) {
	lock := sync.Mutex{}
	function := func() {
		lock.Lock()
		lock.Unlock()
	}
	// TestLockingFunction succeeds.
	TestLockingFunction(&lock, function)
}

func (LockingSuite) TestTestLockingFunctionDetectsDisobeyedLock(c *C) {
	lock := sync.Mutex{}
	function := func() {}
	c.Check(
		func() { TestLockingFunction(&lock, function) },
		Panics,
		errors.New("function did not obey lock"))
}

func (LockingSuite) TestTestLockingFunctionDetectsFailureToReleaseLock(c *C) {
	lock := sync.Mutex{}
	defer lock.Unlock()
	function := func() {
		lock.Lock()
	}
	c.Check(
		func() { TestLockingFunction(&lock, function) },
		Panics,
		errors.New("function did not release lock"))
}
