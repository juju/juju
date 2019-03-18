// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/worker/lease"
)

type LeasesSuite struct {
	testing.IsolationSuite

	appName string
	machine string
	pinArgs []interface{}
}

var _ = gc.Suite(&LeasesSuite{})

func (s *LeasesSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.appName = "redis"
}

func (s *LeasesSuite) TestLeases(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key(s.appName): {
				Holder:   "redis/0",
				Expiry:   offset(time.Second),
				Trapdoor: corelease.LockedTrapdoor,
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		leases := getReader(c, manager).Leases()
		c.Check(leases, gc.DeepEquals, map[string]string{s.appName: "redis/0"})
	})
}

func getReader(c *gc.C, manager *lease.Manager) corelease.Reader {
	reader, err := manager.Reader("namespace", "modelUUID")
	c.Assert(err, jc.ErrorIsNil)
	return reader
}
