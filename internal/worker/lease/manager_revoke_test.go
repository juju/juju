// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/internal/worker/lease"
)

type RevokeSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&RevokeSuite{})

func (s *RevokeSuite) TestHolderSuccess(c *tc.C) {
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
		c.Check(err, tc.ErrorIsNil)
	})
}

func (s *RevokeSuite) TestOtherHolderError(c *tc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getRevoker(c, manager).Revoke("redis", "redis/1")
		c.Check(errors.Cause(err), tc.Equals, corelease.ErrNotHeld)
	})
}

func (s *RevokeSuite) TestMissing(c *tc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getRevoker(c, manager).Revoke("redis", "redis/0")
		c.Check(err, tc.ErrorIsNil)
	})
}

func getRevoker(c *tc.C, manager *lease.Manager) corelease.Revoker {
	revoker, err := manager.Revoker("namespace", "modelUUID")
	c.Assert(err, tc.ErrorIsNil)
	return revoker
}
