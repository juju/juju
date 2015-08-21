// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniteravailability_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniteravailability"
)

type lockSuite struct {
	lock *uniteravailability.RWAbortableLock
}

var _ = gc.Suite(&lockSuite{})

func (s *lockSuite) SetUpTest(c *gc.C) {
	s.lock = uniteravailability.NewRWAbortableLock()
}

func (s *lockSuite) TestAbortNoBlocks(c *gc.C) {
	s.lock.Abort()
}

func (s *lockSuite) TestAcquireAndReleaseLock(c *gc.C) {
	err := s.lock.Lock()
	c.Assert(err, jc.ErrorIsNil)

	s.lock.Unlock()

	err = s.lock.Lock()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *lockSuite) TestWriteLockBlockedByWriteLock(c *gc.C) {
	err := s.lock.Lock()
	c.Assert(err, jc.ErrorIsNil)

	gotLock := make(chan struct{})
	go func() {
		err = s.lock.Lock()
		gotLock <- struct{}{}
	}()

	select {
	case <-gotLock:
		c.Fatal("expected to fail acquiring the lock")
	case <-time.After(jujutesting.ShortWait):
	}
}

func (s *lockSuite) TestAbortAcquiringLock(c *gc.C) {
	err := s.lock.Lock()
	c.Assert(err, jc.ErrorIsNil)

	gotLock := make(chan error)
	go func() {
		err = s.lock.Lock()
		gotLock <- err
	}()
	s.lock.Abort()
	select {
	case err := <-gotLock:
		c.Assert(err, gc.Equals, uniteravailability.ErrAbort)
	case <-time.After(jujutesting.ShortWait):
		c.Fatal("expected to receive abort signal")
	}
}

func (s *lockSuite) TestAcquireAndReleaseReadLock(c *gc.C) {
	err := s.lock.RLock()
	c.Assert(err, jc.ErrorIsNil)

	err = s.lock.RLock()
	c.Assert(err, jc.ErrorIsNil)

	s.lock.RUnlock()
	s.lock.RUnlock()

	err = s.lock.Lock()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *lockSuite) TestReadLockBlockedByWriteLock(c *gc.C) {
	err := s.lock.Lock()
	c.Assert(err, jc.ErrorIsNil)

	gotLock := make(chan struct{})
	go func() {
		err = s.lock.RLock()
		gotLock <- struct{}{}
	}()

	select {
	case <-gotLock:
		c.Fatal("expected to fail acquiring the read lock")
	case <-time.After(jujutesting.ShortWait):
	}

}

func (s *lockSuite) TestWriteLockBlockedByReadLock(c *gc.C) {
	err := s.lock.RLock()
	c.Assert(err, jc.ErrorIsNil)

	gotLock := make(chan struct{})
	go func() {
		err = s.lock.Lock()
		gotLock <- struct{}{}
	}()

	select {
	case <-gotLock:
		c.Fatal("expected to fail acquiring the lock")
	case <-time.After(jujutesting.ShortWait):
	}

}
