// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time" // Only used for time types.

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/lease"
	jujutxn "github.com/juju/txn"
)

// StoreAssertSuite tests that AssertOp does what it should.
type StoreAssertSuite struct {
	FixtureSuite
	fix  *Fixture
	info lease.Info
}

var _ = gc.Suite(&StoreAssertSuite{})

func key(name string) lease.Key {
	return lease.Key{
		Namespace: "default-namespace",
		ModelUUID: "model-uuid",
		Lease:     name,
	}
}

func (s *StoreAssertSuite) SetUpTest(c *gc.C) {
	s.FixtureSuite.SetUpTest(c)
	s.fix = s.EasyFixture(c)
	key := lease.Key{"default-namespace", "model-uuid", "name"}
	err := s.fix.Store.ClaimLease(key, lease.Request{"holder", time.Minute}, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(key, s.fix.Holder(), "holder")
}

func (s *StoreAssertSuite) TestPassesWhenLeaseHeld(c *gc.C) {
	info := s.fix.Store.Leases()[key("name")]

	var ops []txn.Op
	err := info.Trapdoor(0, &ops)
	c.Check(err, jc.ErrorIsNil)
	err = s.fix.Runner.RunTransaction(&jujutxn.Transaction{Ops: ops})
	c.Check(err, jc.ErrorIsNil)
}

func (s *StoreAssertSuite) TestPassesWhenLeaseStillHeldDespiteWriterChange(c *gc.C) {
	info := s.fix.Store.Leases()[key("name")]

	fix2 := s.NewFixture(c, FixtureParams{Id: "other-store"})
	err := fix2.Store.ExtendLease(key("name"), lease.Request{"holder", time.Hour}, nil)
	c.Assert(err, jc.ErrorIsNil)

	var ops []txn.Op
	err = info.Trapdoor(0, &ops)
	c.Check(err, jc.ErrorIsNil)
	err = s.fix.Runner.RunTransaction(&jujutxn.Transaction{Ops: ops})
	c.Check(err, gc.IsNil)
}

func (s *StoreAssertSuite) TestPassesWhenLeaseStillHeldDespitePassingExpiry(c *gc.C) {
	info := s.fix.Store.Leases()[key("name")]

	s.fix.GlobalClock.Advance(time.Hour)
	err := s.fix.Store.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	var ops []txn.Op
	err = info.Trapdoor(0, &ops)
	c.Check(err, jc.ErrorIsNil)
	err = s.fix.Runner.RunTransaction(&jujutxn.Transaction{Ops: ops})
	c.Check(err, gc.IsNil)
}

func (s *StoreAssertSuite) TestAbortsWhenLeaseVacant(c *gc.C) {
	info := s.fix.Store.Leases()[key("name")]

	s.fix.GlobalClock.Advance(time.Hour)
	err := s.fix.Store.ExpireLease(key("name"))
	c.Assert(err, jc.ErrorIsNil)

	var ops []txn.Op
	err = info.Trapdoor(0, &ops)
	c.Check(err, jc.ErrorIsNil)
	err = s.fix.Runner.RunTransaction(&jujutxn.Transaction{Ops: ops})
	c.Check(err, gc.Equals, txn.ErrAborted)
}
