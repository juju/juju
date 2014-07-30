// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// +build !windows

package container_test

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/container"
	coretesting "github.com/juju/juju/testing"
)

type flockSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&flockSuite{})

func (s *flockSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
}

// This test also happens to test that locks can get created when the
// lock directory doesn't exist.
func (s *flockSuite) TestValidNamesLockDir(c *gc.C) {

	for _, name := range []string{
		"a",
		"longer",
		"longer-with.special-characters",
	} {
		dir := c.MkDir()
		_, err := container.NewLock(dir, name)
		c.Assert(err, gc.IsNil)
	}
}

func (s *flockSuite) TestInvalidNames(c *gc.C) {

	for _, name := range []string{
		".start",
		"-start",
		"NoCapitals",
		"no+plus",
		"no/slash",
		"no\\backslash",
		"no$dollar",
		"no:colon",
	} {
		dir := c.MkDir()
		_, err := container.NewLock(dir, name)
		c.Assert(err, gc.ErrorMatches, "Invalid lock name .*")
	}
}

func (s *flockSuite) TestNewLockWithExistingDir(c *gc.C) {
	dir := c.MkDir()
	err := os.MkdirAll(dir, 0755)
	c.Assert(err, gc.IsNil)
	_, err = container.NewLock(dir, "special")
	c.Assert(err, gc.IsNil)
}

func (s *flockSuite) TestLockBlocks(c *gc.C) {

	dir := c.MkDir()
	lock1, err := container.NewLock(dir, "testing")
	c.Assert(err, gc.IsNil)
	c.Assert(container.IsLocked(lock1), gc.Equals, false)
	lock2, err := container.NewLock(dir, "testing")
	c.Assert(err, gc.IsNil)
	c.Assert(container.IsLocked(lock2), gc.Equals, false)

	acquired := make(chan struct{})
	err = lock1.Lock("")
	c.Assert(err, gc.IsNil)
	c.Assert(container.IsLocked(lock1), gc.Equals, true)

	go func() {
		lock2.Lock("")
		c.Assert(container.IsLocked(lock2), gc.Equals, true)
		acquired <- struct{}{}
		close(acquired)
	}()

	// Waiting for something not to happen is inherently hard...
	select {
	case <-acquired:
		c.Fatalf("Unexpected lock acquisition")
	case <-time.After(coretesting.ShortWait):
		// all good
	}

	err = lock1.Unlock()
	c.Assert(err, gc.IsNil)
	c.Assert(container.IsLocked(lock1), gc.Equals, false)

	select {
	case <-acquired:
		// all good
	case <-time.After(coretesting.LongWait):
		c.Fatalf("Expected lock acquisition")
	}
}

func (s *flockSuite) TestUnlock(c *gc.C) {
	dir := c.MkDir()
	lock, err := container.NewLock(dir, "testing")
	c.Assert(err, gc.IsNil)
	err = lock.Lock("test")
	c.Assert(err, gc.IsNil)
	c.Assert(container.IsLocked(lock), gc.Equals, true)

	err = lock.Unlock()
	c.Assert(err, gc.IsNil)
	c.Assert(container.IsLocked(lock), gc.Equals, false)
}

func (s *flockSuite) TestStress(c *gc.C) {
	const lockAttempts = 200
	const concurrentLocks = 10

	var counter = new(int64)
	// Use atomics to update lockState to make sure the lock isn't held by
	// someone else. A value of 1 means locked, 0 means unlocked.
	var lockState = new(int32)
	var done = make(chan struct{})
	defer close(done)

	dir := c.MkDir()

	var stress = func(name string) {
		defer func() { done <- struct{}{} }()
		lock, err := container.NewLock(dir, "testing")
		if err != nil {
			c.Errorf("Failed to create a new lock")
			return
		}
		for i := 0; i < lockAttempts; i++ {
			err = lock.Lock(name)
			c.Assert(err, gc.IsNil)
			state := atomic.AddInt32(lockState, 1)
			c.Assert(state, gc.Equals, int32(1))
			// Tell the go routine scheduler to give a slice to someone else
			// while we have this locked.
			runtime.Gosched()
			// need to decrement prior to unlock to avoid the race of someone
			// else grabbing the lock before we decrement the state.
			atomic.AddInt32(lockState, -1)
			err = lock.Unlock()
			c.Assert(err, gc.IsNil)
			// increment the general counter
			atomic.AddInt64(counter, 1)
		}
	}

	for i := 0; i < concurrentLocks; i++ {
		go stress(fmt.Sprintf("Lock %d", i))
	}
	for i := 0; i < concurrentLocks; i++ {
		<-done
	}
	c.Assert(*counter, gc.Equals, int64(lockAttempts*concurrentLocks))
}
