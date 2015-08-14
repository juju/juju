// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&singletonLeaseSuite{})

type singletonLeaseSuite struct {
	coretesting.BaseSuite
}

func (s *singletonLeaseSuite) TestSingletonNewLeaseManager(c *gc.C) {
	assertSingletonInactive(c)

	var called bool
	manager, err := NewLeaseManager(&stubLeasePersistor{
		WriteTokenFn: func(string, Token) error {
			called = true
			return nil
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = Manager().ClaimLease("foo", "bar", 0)
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)

	manager.Kill()
	c.Assert(manager.Wait(), jc.ErrorIsNil)
	assertSingletonInactive(c)
}

func (s *singletonLeaseSuite) TestSingletonSingular(c *gc.C) {
	var called bool
	manager, err := NewLeaseManager(&stubLeasePersistor{
		WriteTokenFn: func(string, Token) error {
			called = true
			return nil
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		manager.Kill()
		c.Assert(manager.Wait(), jc.ErrorIsNil)
	}()
	_, err = NewLeaseManager(&stubLeasePersistor{})
	c.Check(err, gc.Equals, ErrLeaseManagerRunning)

	// Ensure the first lease manager is still in tact.
	_, err = Manager().ClaimLease("foo", "bar", 0)
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
}

func assertSingletonInactive(c *gc.C) {
	_, err := Manager().ClaimLease("foo", "bar", 0)
	c.Assert(err, gc.Equals, ErrNoLeaseManager)
}
