// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
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
		Clock:     struct{ clock.Clock }{},
		Secretary: struct{ leadership.Secretary }{},
	})
	c.Check(err, gc.ErrorMatches, "nil Client not valid")
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(manager, gc.IsNil)
}

func (s *ValidationSuite) TestMissingClock(c *gc.C) {
	manager, err := leadership.NewManager(leadership.ManagerConfig{
		Client:    struct{ lease.Client }{},
		Secretary: struct{ leadership.Secretary }{},
	})
	c.Check(err, gc.ErrorMatches, "nil Clock not valid")
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(manager, gc.IsNil)
}

func (s *ValidationSuite) TestMissingSecretary(c *gc.C) {
	manager, err := leadership.NewManager(leadership.ManagerConfig{
		Client: struct{ lease.Client }{},
		Clock:  struct{ clock.Clock }{},
	})
	c.Check(err, gc.ErrorMatches, "nil Secretary not valid")
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(manager, gc.IsNil)
}

func (s *ValidationSuite) TestClaimLeadership_ServiceName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		err := manager.ClaimLeadership("INVALID", "bar/0", time.Minute)
		c.Check(err, gc.ErrorMatches, `cannot claim lease: lease name: INVALID not valid`)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	})
}

func (s *ValidationSuite) TestClaimLeadership_UnitName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		err := manager.ClaimLeadership("foo", "INVALID", time.Minute)
		c.Check(err, gc.ErrorMatches, `cannot claim lease: holder name: INVALID not valid`)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	})
}

func (s *ValidationSuite) TestClaimLeadership_Duration(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		err := manager.ClaimLeadership("foo", "bar/0", 0)
		c.Check(err, gc.ErrorMatches, `cannot claim lease: duration 0 not valid`)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	})
}

func (s *ValidationSuite) TestLeadershipCheck_ServiceName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		token := manager.LeadershipCheck("INVALID", "bar/0")
		err := token.Check(nil)
		c.Check(err, gc.ErrorMatches, `cannot check lease: lease name: INVALID not valid`)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	})
}

func (s *ValidationSuite) TestLeadershipCheck_UnitName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		token := manager.LeadershipCheck("foo", "INVALID")
		err := token.Check(nil)
		c.Check(err, gc.ErrorMatches, `cannot check lease: holder name: INVALID not valid`)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
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
		err := token.Check(&bad)
		c.Check(err, gc.ErrorMatches, `expected \*\[\]txn.Op; \*string not valid`)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	})
}

func (s *ValidationSuite) TestBlockUntilLeadershipReleased_ServiceName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		err := manager.BlockUntilLeadershipReleased("INVALID")
		c.Check(err, gc.ErrorMatches, `cannot wait for lease expiry: lease name: INVALID not valid`)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	})
}
