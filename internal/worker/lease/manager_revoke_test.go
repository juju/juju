// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/internal/worker/lease"
)

type RevokeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RevokeSuite{})

func (s *RevokeSuite) TestHolderSuccess(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "RevokeLease",
			args: []interface{}{
				corelease.Key{
					Namespace: "namespace",
					ModelUUID: "modelUUID",
					Lease:     "redis",
				},
				"redis/0",
			},
		}},
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getRevoker(c, manager).Revoke("redis", "redis/0")
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *RevokeSuite) TestOtherHolderError(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getRevoker(c, manager).Revoke("redis", "redis/1")
		c.Check(errors.Cause(err), gc.Equals, corelease.ErrNotHeld)
	})
}

func (s *RevokeSuite) TestMissing(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getRevoker(c, manager).Revoke("redis", "redis/0")
		c.Check(err, jc.ErrorIsNil)
	})
}

func getRevoker(c *gc.C, manager *lease.Manager) corelease.Revoker {
	revoker, err := manager.Revoker("namespace", "modelUUID")
	c.Assert(err, jc.ErrorIsNil)
	return revoker
}
