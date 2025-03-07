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
	"github.com/juju/juju/internal/worker/lease"
)

type TokenSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&TokenSuite{})

func (s *TokenSuite) TestSuccess(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		token := getChecker(c, manager).Token("redis", "redis/0")
		err := token.Check()
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *TokenSuite) TestFailureMissing(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		token := getChecker(c, manager).Token("redis", "redis/0")
		err := token.Check()
		c.Check(errors.Cause(err), gc.Equals, corelease.ErrNotHeld)
	})
}

func (s *TokenSuite) TestFailureOtherHolder(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/99",
				Expiry: offset(time.Second),
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		token := getChecker(c, manager).Token("redis", "redis/0")
		err := token.Check()
		c.Check(errors.Cause(err), gc.Equals, corelease.ErrNotHeld)
	})
}

func getChecker(c *gc.C, manager *lease.Manager) corelease.Checker {
	checker, err := manager.Checker("namespace", "modelUUID")
	c.Assert(err, jc.ErrorIsNil)
	return checker
}
