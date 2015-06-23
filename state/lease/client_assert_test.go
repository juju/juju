// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/state/lease"
)

// ClientAssertSuite tests that AssertOp does what it should.
type ClientAssertSuite struct {
	FixtureSuite
	fix  *Fixture
	info lease.Info
}

var _ = gc.Suite(&ClientAssertSuite{})

func (s *ClientAssertSuite) SetUpTest(c *gc.C) {
	s.FixtureSuite.SetUpTest(c)
	s.fix = s.EasyFixture(c)
	err := s.fix.Client.ClaimLease("name", lease.Request{"holder", time.Minute})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert("name", s.fix.Holder(), "holder")
}

func (s *ClientAssertSuite) TestPassesWhenLeaseHeld(c *gc.C) {
	info := s.fix.Client.Leases()["name"]

	ops := []txn.Op{info.AssertOp}
	err := s.fix.Runner.RunTransaction(ops)
	c.Check(err, jc.ErrorIsNil)
}

func (s *ClientAssertSuite) TestPassesWhenLeaseStillHeldDespiteWriterChange(c *gc.C) {
	info := s.fix.Client.Leases()["name"]

	fix2 := s.NewFixture(c, FixtureParams{Id: "other-client"})
	err := fix2.Client.ExtendLease("name", lease.Request{"holder", time.Hour})
	c.Assert(err, jc.ErrorIsNil)

	ops := []txn.Op{info.AssertOp}
	err = s.fix.Runner.RunTransaction(ops)
	c.Check(err, gc.IsNil)
}

func (s *ClientAssertSuite) TestPassesWhenLeaseStillHeldDespitePassingExpiry(c *gc.C) {
	info := s.fix.Client.Leases()["name"]

	s.fix.Clock.Advance(time.Hour)
	err := s.fix.Client.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	ops := []txn.Op{info.AssertOp}
	err = s.fix.Runner.RunTransaction(ops)
	c.Check(err, gc.IsNil)
}

func (s *ClientAssertSuite) TestAbortsWhenLeaseVacant(c *gc.C) {
	info := s.fix.Client.Leases()["name"]

	s.fix.Clock.Advance(time.Hour)
	err := s.fix.Client.ExpireLease("name")
	c.Assert(err, jc.ErrorIsNil)

	ops := []txn.Op{info.AssertOp}
	err = s.fix.Runner.RunTransaction(ops)
	c.Check(err, gc.Equals, txn.ErrAborted)
}
