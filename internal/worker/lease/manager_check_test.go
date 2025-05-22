// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/lease"
)

type TokenSuite struct {
	testhelpers.IsolationSuite
}

func TestTokenSuite(t *testing.T) {
	tc.Run(t, &TokenSuite{})
}

func (s *TokenSuite) TestSuccess(c *tc.C) {
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
		c.Check(err, tc.ErrorIsNil)
	})
}

func (s *TokenSuite) TestFailureMissing(c *tc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		token := getChecker(c, manager).Token("redis", "redis/0")
		err := token.Check()
		c.Check(errors.Cause(err), tc.Equals, corelease.ErrNotHeld)
	})
}

func (s *TokenSuite) TestFailureOtherHolder(c *tc.C) {
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
		c.Check(errors.Cause(err), tc.Equals, corelease.ErrNotHeld)
	})
}

func getChecker(c *tc.C, manager *lease.Manager) corelease.Checker {
	checker, err := manager.Checker("namespace", "modelUUID")
	c.Assert(err, tc.ErrorIsNil)
	return checker
}
