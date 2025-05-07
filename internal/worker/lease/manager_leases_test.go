// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"fmt"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/internal/worker/lease"
)

type LeasesSuite struct {
	testing.IsolationSuite

	appName string
}

var _ = tc.Suite(&LeasesSuite{})

func (s *LeasesSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.appName = "redis"
}

func (s *LeasesSuite) TestLeases(c *tc.C) {
	leases := map[corelease.Key]corelease.Info{
		key(s.appName): {
			Holder: "redis/0",
			Expiry: offset(time.Second),
		},
	}
	// Add enough leases for other models and namespaces to ensure
	// that we would definitely fail if the Leases method does the
	// wrong thing.
	bad := corelease.Info{
		Holder: "redis/1",
		Expiry: offset(time.Second),
	}
	for i := 0; i < 100; i++ {
		otherNS := fmt.Sprintf("ns%d", i)
		leases[key(otherNS, "modelUUID", s.appName)] = bad
		otherModel := fmt.Sprintf("model%d", i)
		leases[key("namespace", otherModel, s.appName)] = bad
	}

	fix := &Fixture{leases: leases}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		leases, err := getReader(c, manager).Leases()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(leases, tc.DeepEquals, map[string]string{s.appName: "redis/0"})
	})
}

func getReader(c *tc.C, manager *lease.Manager) corelease.Reader {
	reader, err := manager.Reader("namespace", "modelUUID")
	c.Assert(err, jc.ErrorIsNil)
	return reader
}
