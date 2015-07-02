// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"time"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/leadership"
)

type ValidationSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ValidationSuite{})

func (s *ValidationSuite) TestMissingClient(c *gc.C) {
	manager, err := leadership.NewManager(leadership.ManagerConfig{
		Clock: NewClock(time.Now()),
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
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *Clock) {
		err := manager.ClaimLeadership("foo/0", "bar/0", time.Minute)
		c.Check(err, gc.ErrorMatches, `cannot claim leadership: invalid service name "foo/0"`)
	})
}

func (s *ValidationSuite) TestClaimLeadership_UnitName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *Clock) {
		err := manager.ClaimLeadership("foo", "bar", time.Minute)
		c.Check(err, gc.ErrorMatches, `cannot claim leadership: invalid unit name "bar"`)
	})
}

func (s *ValidationSuite) TestClaimLeadership_Duration(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *Clock) {
		err := manager.ClaimLeadership("foo", "bar/0", 0)
		c.Check(err, gc.ErrorMatches, `cannot claim leadership: invalid duration 0`)
	})
}

func (s *ValidationSuite) TestCheckLeadership_ServiceName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *Clock) {
		token, err := manager.CheckLeadership("foo/0", "bar/0")
		c.Check(err, gc.ErrorMatches, `cannot check leadership: invalid service name "foo/0"`)
		c.Check(token, gc.IsNil)
	})
}

func (s *ValidationSuite) TestCheckLeadership_UnitName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *Clock) {
		token, err := manager.CheckLeadership("foo", "bar")
		c.Check(err, gc.ErrorMatches, `cannot check leadership: invalid unit name "bar"`)
		c.Check(token, gc.IsNil)
	})
}

func (s *ValidationSuite) TestBlockUntilLeadershipReleased_ServiceName(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *Clock) {
		err := manager.BlockUntilLeadershipReleased("foo/0")
		c.Check(err, gc.ErrorMatches, `cannot wait for leaderlessness: invalid service name "foo/0"`)
	})
}
