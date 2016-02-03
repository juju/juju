// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/lease"
)

type ValidationSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ValidationSuite{})

func (s *ValidationSuite) TestMissingClient(c *gc.C) {
	manager, err := lease.NewManager(lease.ManagerConfig{
		Clock:     struct{ clock.Clock }{},
		Secretary: struct{ lease.Secretary }{},
		MaxSleep:  time.Minute,
	})
	c.Check(err, gc.ErrorMatches, "nil Client not valid")
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(manager, gc.IsNil)
}

func (s *ValidationSuite) TestMissingClock(c *gc.C) {
	manager, err := lease.NewManager(lease.ManagerConfig{
		Client:    struct{ corelease.Client }{},
		Secretary: struct{ lease.Secretary }{},
		MaxSleep:  time.Minute,
	})
	c.Check(err, gc.ErrorMatches, "nil Clock not valid")
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(manager, gc.IsNil)
}

func (s *ValidationSuite) TestMissingSecretary(c *gc.C) {
	manager, err := lease.NewManager(lease.ManagerConfig{
		Client: struct{ corelease.Client }{},
		Clock:  struct{ clock.Clock }{},
	})
	c.Check(err, gc.ErrorMatches, "nil Secretary not valid")
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(manager, gc.IsNil)
}

func (s *ValidationSuite) TestMissingMaxSleep(c *gc.C) {
	manager, err := lease.NewManager(lease.ManagerConfig{
		Client:    NewClient(nil, nil),
		Secretary: struct{ lease.Secretary }{},
		Clock:     coretesting.NewClock(time.Now()),
	})
	c.Check(err, gc.ErrorMatches, "non-positive MaxSleep not valid")
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(manager, gc.IsNil)
}

func (s *ValidationSuite) TestNegativeMaxSleep(c *gc.C) {
	manager, err := lease.NewManager(lease.ManagerConfig{
		Client:    NewClient(nil, nil),
		Clock:     coretesting.NewClock(time.Now()),
		Secretary: struct{ lease.Secretary }{},
		MaxSleep:  -time.Nanosecond,
	})
	c.Check(err, gc.ErrorMatches, "non-positive MaxSleep not valid")
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(manager, gc.IsNil)
}

func (s *ValidationSuite) TestClaim_LeaseName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *coretesting.Clock) {
		err := manager.Claim("INVALID", "bar/0", time.Minute)
		c.Check(err, gc.ErrorMatches, `cannot claim lease "INVALID": name not valid`)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	})
}

func (s *ValidationSuite) TestClaim_HolderName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *coretesting.Clock) {
		err := manager.Claim("foo", "INVALID", time.Minute)
		c.Check(err, gc.ErrorMatches, `cannot claim lease for holder "INVALID": name not valid`)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	})
}

func (s *ValidationSuite) TestClaim_Duration(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *coretesting.Clock) {
		err := manager.Claim("foo", "bar/0", time.Second)
		c.Check(err, gc.ErrorMatches, `cannot claim lease for 1s: time not valid`)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	})
}

func (s *ValidationSuite) TestToken_LeaseName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *coretesting.Clock) {
		token := manager.Token("INVALID", "bar/0")
		err := token.Check(nil)
		c.Check(err, gc.ErrorMatches, `cannot check lease "INVALID": name not valid`)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	})
}

func (s *ValidationSuite) TestToken_HolderName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *coretesting.Clock) {
		token := manager.Token("foo", "INVALID")
		err := token.Check(nil)
		c.Check(err, gc.ErrorMatches, `cannot check holder "INVALID": name not valid`)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	})
}

func (s *ValidationSuite) TestToken_OutPtr(c *gc.C) {
	expectKey := "bad"
	expectErr := errors.New("bad")

	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
			callback: func(leases map[string]corelease.Info) {
				leases["redis"] = corelease.Info{
					Holder: "redis/0",
					Expiry: offset(time.Second),
					Trapdoor: func(gotKey interface{}) error {
						c.Check(gotKey, gc.Equals, &expectKey)
						return expectErr
					},
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *coretesting.Clock) {
		token := manager.Token("redis", "redis/0")
		err := token.Check(&expectKey)
		cause := errors.Cause(err)
		c.Check(cause, gc.Equals, expectErr)
	})
}

func (s *ValidationSuite) TestWaitUntilExpired_LeaseName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *coretesting.Clock) {
		err := manager.WaitUntilExpired("INVALID")
		c.Check(err, gc.ErrorMatches, `cannot wait for lease "INVALID" expiry: name not valid`)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	})
}
