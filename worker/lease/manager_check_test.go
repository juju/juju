// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/lease"
)

type TokenSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&TokenSuite{})

func (s *TokenSuite) TestSuccess(c *gc.C) {
	fix := &Fixture{
		leases: map[string]corelease.Info{
			"redis": corelease.Info{
				Holder:   "redis/0",
				Expiry:   offset(time.Second),
				Trapdoor: corelease.LockedTrapdoor,
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *coretesting.Clock) {
		token := manager.Token("redis", "redis/0")
		err := token.Check(nil)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *TokenSuite) TestMissingRefresh_Success(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
			callback: func(leases map[string]corelease.Info) {
				leases["redis"] = corelease.Info{
					Holder:   "redis/0",
					Expiry:   offset(time.Second),
					Trapdoor: corelease.LockedTrapdoor,
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *coretesting.Clock) {
		token := manager.Token("redis", "redis/0")
		err := token.Check(nil)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *TokenSuite) TestOtherHolderRefresh_Success(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
			callback: func(leases map[string]corelease.Info) {
				leases["redis"] = corelease.Info{
					Holder:   "redis/0",
					Expiry:   offset(time.Second),
					Trapdoor: corelease.LockedTrapdoor,
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *coretesting.Clock) {
		token := manager.Token("redis", "redis/0")
		err := token.Check(nil)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *TokenSuite) TestRefresh_Failure_Missing(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *coretesting.Clock) {
		token := manager.Token("redis", "redis/0")
		err := token.Check(nil)
		c.Check(errors.Cause(err), gc.Equals, corelease.ErrNotHeld)
	})
}

func (s *TokenSuite) TestRefresh_Failure_OtherHolder(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
			callback: func(leases map[string]corelease.Info) {
				leases["redis"] = corelease.Info{
					Holder:   "redis/1",
					Expiry:   offset(time.Second),
					Trapdoor: corelease.LockedTrapdoor,
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *coretesting.Clock) {
		token := manager.Token("redis", "redis/0")
		err := token.Check(nil)
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
	fix.RunTest(c, func(manager *lease.Manager, _ *coretesting.Clock) {
		token := manager.Token("redis", "redis/0")
		c.Check(token.Check(nil), gc.ErrorMatches, "lease manager stopped")
		err := manager.Wait()
		c.Check(err, gc.ErrorMatches, "crunch squish")
	})
}
