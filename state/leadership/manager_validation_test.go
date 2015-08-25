// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"time"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/state/leadership"
	"github.com/juju/juju/state/lease"
	coretesting "github.com/juju/juju/testing"
)

type ValidationSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ValidationSuite{})

func (s *ValidationSuite) TestMissingClient(c *gc.C) {
	manager, err := leadership.NewManager(leadership.ManagerConfig{
		Clock: coretesting.NewClock(time.Now()),
	})
	c.Check(err, gc.ErrorMatches, "missing client")
	c.Check(manager, gc.IsNil)
}

func (s *ValidationSuite) TestMissingClock(c *gc.C) {
	manager, err := leadership.NewManager(leadership.ManagerConfig{
		Client: NewClient(nil, nil),
	})
	c.Check(err, gc.ErrorMatches, "missing clock")
	c.Check(manager, gc.IsNil)
}

func (s *ValidationSuite) TestClaimLeadership_ServiceName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		err := manager.ClaimLeadership("foo/0", "bar/0", time.Minute)
		c.Check(err, gc.ErrorMatches, `cannot claim leadership: invalid service name "foo/0"`)
	})
}

func (s *ValidationSuite) TestClaimLeadership_UnitName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		err := manager.ClaimLeadership("foo", "bar", time.Minute)
		c.Check(err, gc.ErrorMatches, `cannot claim leadership: invalid unit name "bar"`)
	})
}

func (s *ValidationSuite) TestClaimLeadership_Duration(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		err := manager.ClaimLeadership("foo", "bar/0", 0)
		c.Check(err, gc.ErrorMatches, `cannot claim leadership: invalid duration 0`)
	})
}

func (s *ValidationSuite) TestLeadershipCheck_ServiceName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		token := manager.LeadershipCheck("foo/0", "bar/0")
		c.Check(token.Check(nil), gc.ErrorMatches, `cannot check leadership: invalid service name "foo/0"`)
	})
}

func (s *ValidationSuite) TestLeadershipCheck_UnitName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		token := manager.LeadershipCheck("foo", "bar")
		c.Check(token.Check(nil), gc.ErrorMatches, `cannot check leadership: invalid unit name "bar"`)
	})
}

func (s *ValidationSuite) TestLeadershipCheck_OutPtr(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
			callback: func(leases map[string]lease.Info) {
				leases["redis"] = lease.Info{
					Holder:   "redis/0",
					Expiry:   offset(time.Second),
					AssertOp: txn.Op{C: "fake", Id: "fake"},
				}
			},
		}},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		bad := "bad"
		token := manager.LeadershipCheck("redis", "redis/0")
		c.Check(token.Check(&bad), gc.ErrorMatches, `expected pointer to \[\]txn.Op`)
	})
}

func (s *ValidationSuite) TestBlockUntilLeadershipReleased_ServiceName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		err := manager.BlockUntilLeadershipReleased("foo/0")
		c.Check(err, gc.ErrorMatches, `cannot wait for leaderlessness: invalid service name "foo/0"`)
	})
}
