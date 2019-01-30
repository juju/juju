// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/worker/lease"
)

type TokenSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&TokenSuite{})

func (s *TokenSuite) TestSuccess(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder:   "redis/0",
				Expiry:   offset(time.Second),
				Trapdoor: corelease.LockedTrapdoor,
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		token := getChecker(c, manager).Token("redis", "redis/0")
		err := token.Check(0, nil)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *TokenSuite) TestMissingRefresh_Success(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[key("redis")] = corelease.Info{
					Holder:   "redis/0",
					Expiry:   offset(time.Second),
					Trapdoor: corelease.LockedTrapdoor,
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		token := getChecker(c, manager).Token("redis", "redis/0")
		err := token.Check(0, nil)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *TokenSuite) TestOtherHolderRefresh_Success(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[key("redis")] = corelease.Info{
					Holder:   "redis/0",
					Expiry:   offset(time.Second),
					Trapdoor: corelease.LockedTrapdoor,
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		token := getChecker(c, manager).Token("redis", "redis/0")
		err := token.Check(0, nil)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *TokenSuite) TestRefresh_Failure_Missing(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		token := getChecker(c, manager).Token("redis", "redis/0")
		err := token.Check(0, nil)
		c.Check(errors.Cause(err), gc.Equals, corelease.ErrNotHeld)
	})
}

func (s *TokenSuite) TestRefresh_Failure_OtherHolder(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[key("redis")] = corelease.Info{
					Holder:   "redis/1",
					Expiry:   offset(time.Second),
					Trapdoor: corelease.LockedTrapdoor,
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		token := getChecker(c, manager).Token("redis", "redis/0")
		err := token.Check(0, nil)
		c.Check(errors.Cause(err), gc.Equals, corelease.ErrNotHeld)
	})
}

func (s *TokenSuite) TestRefresh_Error(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
			err:    errors.New("crunch squish"),
		}},
		expectDirty: true,
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		token := getChecker(c, manager).Token("redis", "redis/0")
		c.Check(token.Check(0, nil), gc.ErrorMatches, "lease manager stopped")
		err := manager.Wait()
		c.Check(err, gc.ErrorMatches, "crunch squish")
	})
}

func getChecker(c *gc.C, manager *lease.Manager) corelease.Checker {
	checker, err := manager.Checker("namespace", "modelUUID")
	c.Assert(err, jc.ErrorIsNil)
	return checker
}
