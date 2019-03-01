// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time" // Only used for time types.

	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	txntesting "github.com/juju/txn/testing"
	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
)

// StoreSimpleRaceSuite tests what happens when two stores interfere with
// each other when creating stores and/or leases.
type StoreSimpleRaceSuite struct {
	FixtureSuite
}

var _ = gc.Suite(&StoreSimpleRaceSuite{})

func (s *StoreSimpleRaceSuite) TestClaimLease_BlockedBy_ClaimLease(c *gc.C) {
	sut := s.EasyFixture(c)
	blocker := s.NewFixture(c, FixtureParams{Id: "blocker"})

	// Set up a hook to grab the lease "name" just before the next txn runs.
	defer txntesting.SetBeforeHooks(c, sut.Runner, func() {
		err := blocker.Store.ClaimLease(key("name"), corelease.Request{"ha-haa", time.Minute}, nil)
		c.Check(err, jc.ErrorIsNil)
	})()

	// Try to grab the lease "name", and fail.
	err := sut.Store.ClaimLease(key("name"), corelease.Request{"trying", time.Second}, nil)
	c.Check(err, gc.Equals, corelease.ErrInvalid)

	// The store that failed has refreshed state (as it had to, in order
	// to discover the reason for the invalidity).
	c.Check(key("name"), sut.Holder(), "ha-haa")
	c.Check(key("name"), sut.Expiry(), sut.Zero.Add(time.Minute))
}

func (s *StoreSimpleRaceSuite) TestClaimLease_Pathological(c *gc.C) {
	sut := s.EasyFixture(c)
	blocker := s.NewFixture(c, FixtureParams{Id: "blocker"})

	// Set up hooks to claim a lease just before every transaction, but remove
	// it again before the SUT goes and looks to figure out what it should do.
	interfere := jujutxn.TestHook{
		Before: func() {
			err := blocker.Store.ClaimLease(key("name"), corelease.Request{"ha-haa", time.Second}, nil)
			c.Check(err, jc.ErrorIsNil)
		},
		After: func() {
			blocker.GlobalClock.Advance(time.Minute)
			err := blocker.Store.ExpireLease(key("name"))
			c.Check(err, jc.ErrorIsNil)
		},
	}
	defer txntesting.SetTestHooks(
		c, sut.Runner,
		interfere, interfere, interfere,
	)()

	// Try to claim, and watch the poor thing collapse in exhaustion.
	err := sut.Store.ClaimLease(key("name"), corelease.Request{"trying", time.Minute}, nil)
	c.Check(err, gc.ErrorMatches, "cannot satisfy request: state changing too quickly; try again soon")
}

// StoreTrickyRaceSuite tests what happens when two stores interfere with
// each other when extending and/or expiring leases.
type StoreTrickyRaceSuite struct {
	FixtureSuite
	sut     *Fixture
	blocker *Fixture
}

var _ = gc.Suite(&StoreTrickyRaceSuite{})

func (s *StoreTrickyRaceSuite) SetUpTest(c *gc.C) {
	s.FixtureSuite.SetUpTest(c)
	s.sut = s.EasyFixture(c)
	err := s.sut.Store.ClaimLease(key("name"), corelease.Request{"holder", time.Minute}, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.blocker = s.NewFixture(c, FixtureParams{Id: "blocker"})
}

func (s *StoreTrickyRaceSuite) TestExtendLease_WorksDespite_ShorterExtendLease(c *gc.C) {

	shorterRequest := 90 * time.Second
	longerRequest := 120 * time.Second

	// Set up hooks to extend the lease by a little, before the SUT's extend
	// gets a chance; and then to verify state after it's applied its retry.
	defer txntesting.SetRetryHooks(c, s.sut.Runner, func() {
		err := s.blocker.Store.ExtendLease(key("name"), corelease.Request{"holder", shorterRequest}, nil)
		c.Check(err, jc.ErrorIsNil)
	}, func() {
		err := s.blocker.Store.Refresh()
		c.Check(err, jc.ErrorIsNil)
		c.Check(key("name"), s.blocker.Expiry(), s.blocker.Zero.Add(longerRequest))
	})()

	// Extend the lease.
	err := s.sut.Store.ExtendLease(key("name"), corelease.Request{"holder", longerRequest}, nil)
	c.Check(err, jc.ErrorIsNil)
}

func (s *StoreTrickyRaceSuite) TestExtendLease_WorksDespite_LongerExtendLease(c *gc.C) {

	shorterRequest := 90 * time.Second
	longerRequest := 120 * time.Second

	// Set up hooks to extend the lease by a lot, before the SUT's extend can.
	defer txntesting.SetBeforeHooks(c, s.sut.Runner, func() {
		err := s.blocker.Store.ExtendLease(key("name"), corelease.Request{"holder", longerRequest}, nil)
		c.Check(err, jc.ErrorIsNil)
	})()

	// Extend the lease by a little.
	err := s.sut.Store.ExtendLease(key("name"), corelease.Request{"holder", shorterRequest}, nil)
	c.Check(err, jc.ErrorIsNil)

	// The SUT was refreshed, and knows that the lease is really valid for longer.
	c.Check(key("name"), s.sut.Expiry(), s.sut.Zero.Add(longerRequest))
}

func (s *StoreTrickyRaceSuite) TestExtendLease_BlockedBy_ExpireLease(c *gc.C) {

	// Set up a hook to expire the lease before the extend gets a chance.
	defer txntesting.SetBeforeHooks(c, s.sut.Runner, func() {
		s.blocker.GlobalClock.Advance(90 * time.Second)
		err := s.blocker.Store.ExpireLease(key("name"))
		c.Check(err, jc.ErrorIsNil)
	})()

	// Try to extend; check it aborts.
	err := s.sut.Store.ExtendLease(key("name"), corelease.Request{"holder", 2 * time.Minute}, nil)
	c.Check(err, gc.Equals, corelease.ErrInvalid)

	// The SUT has been refreshed, and you can see why the operation was invalid.
	c.Check(key("name"), s.sut.Holder(), "")
}

func (s *StoreTrickyRaceSuite) TestExtendLease_BlockedBy_ExpireThenReclaimDifferentHolder(c *gc.C) {

	// Set up a hook to expire and reclaim the lease before the extend gets a
	// chance.
	defer txntesting.SetBeforeHooks(c, s.sut.Runner, func() {
		s.blocker.GlobalClock.Advance(90 * time.Second)
		err := s.blocker.Store.ExpireLease(key("name"))
		c.Check(err, jc.ErrorIsNil)
		err = s.blocker.Store.ClaimLease(key("name"), corelease.Request{"different-holder", time.Minute}, nil)
		c.Check(err, jc.ErrorIsNil)
	})()

	// Try to extend; check it aborts.
	err := s.sut.Store.ExtendLease(key("name"), corelease.Request{"holder", 2 * time.Minute}, nil)
	c.Check(err, gc.Equals, corelease.ErrInvalid)

	// The SUT has been refreshed, and you can see why the operation was invalid.
	c.Check(key("name"), s.sut.Holder(), "different-holder")
}

func (s *StoreTrickyRaceSuite) TestExtendLease_WorksDespite_ExpireThenReclaimSameHolder(c *gc.C) {

	// Set up hooks to expire and reclaim the lease before the extend gets a
	// chance; and to verify that the second attempt successfully extends.
	defer txntesting.SetRetryHooks(c, s.sut.Runner, func() {
		s.blocker.GlobalClock.Advance(90 * time.Second)
		s.blocker.LocalClock.Advance(90 * time.Second)
		err := s.blocker.Store.ExpireLease(key("name"))
		c.Check(err, jc.ErrorIsNil)
		err = s.blocker.Store.ClaimLease(key("name"), corelease.Request{"holder", time.Minute}, nil)
		c.Check(err, jc.ErrorIsNil)
	}, func() {
		err := s.blocker.Store.Refresh()
		c.Check(err, jc.ErrorIsNil)
		c.Check(key("name"), s.blocker.Expiry(), s.blocker.Zero.Add(5*time.Minute))
	})()

	// Try to extend; check it worked.
	err := s.sut.Store.ExtendLease(key("name"), corelease.Request{"holder", 5 * time.Minute}, nil)
	c.Check(err, jc.ErrorIsNil)
}

func (s *StoreTrickyRaceSuite) TestExtendLease_Pathological(c *gc.C) {

	// Set up hooks to remove the lease just before every transaction, but
	// replace it before the SUT goes and looks to figure out what it should do.
	interfere := jujutxn.TestHook{
		Before: func() {
			s.blocker.GlobalClock.Advance(time.Minute + time.Second)
			s.blocker.LocalClock.Advance(time.Minute + time.Second)
			err := s.blocker.Store.ExpireLease(key("name"))
			c.Check(err, jc.ErrorIsNil)
		},
		After: func() {
			err := s.blocker.Store.ClaimLease(key("name"), corelease.Request{"holder", time.Second}, nil)
			c.Check(err, jc.ErrorIsNil)
		},
	}
	defer txntesting.SetTestHooks(
		c, s.sut.Runner,
		interfere, interfere, interfere,
	)()

	// Try to extend, and watch the poor thing collapse in exhaustion.
	err := s.sut.Store.ExtendLease(key("name"), corelease.Request{"holder", 3 * time.Minute}, nil)
	c.Check(err, gc.ErrorMatches, "cannot satisfy request: state changing too quickly; try again soon")
}

func (s *StoreTrickyRaceSuite) TestExpireLease_BlockedBy_ExtendLease(c *gc.C) {

	// Set up a hook to extend the lease before the expire gets a chance.
	defer txntesting.SetBeforeHooks(c, s.sut.Runner, func() {
		s.blocker.GlobalClock.Advance(90 * time.Second)
		s.blocker.LocalClock.Advance(90 * time.Second)
		err := s.blocker.Store.ExtendLease(key("name"), corelease.Request{"holder", 30 * time.Second}, nil)
		c.Check(err, jc.ErrorIsNil)
	})()

	// Try to expire; check it aborts.
	s.sut.GlobalClock.Advance(90 * time.Second)
	err := s.sut.Store.ExpireLease(key("name"))
	c.Check(err, gc.Equals, corelease.ErrInvalid)

	// The SUT has been refreshed, and you can see why the operation was invalid.
	s.sut.LocalClock.Advance(90 * time.Second)
	c.Check(key("name"), s.sut.Expiry(), s.sut.Zero.Add(2*time.Minute))
}

func (s *StoreTrickyRaceSuite) TestExpireLease_BlockedBy_ExpireLease(c *gc.C) {

	// Set up a hook to expire the lease before the SUT gets a chance.
	defer txntesting.SetBeforeHooks(c, s.sut.Runner, func() {
		s.blocker.GlobalClock.Advance(90 * time.Second)
		err := s.blocker.Store.ExpireLease(key("name"))
		c.Check(err, jc.ErrorIsNil)
	})()

	// Try to expire; check it aborts.
	s.sut.GlobalClock.Advance(90 * time.Second)
	err := s.sut.Store.ExpireLease(key("name"))
	c.Check(err, gc.Equals, corelease.ErrInvalid)

	// The SUT has been refreshed, and you can see why the operation was invalid.
	c.Check(key("name"), s.sut.Holder(), "")
}

func (s *StoreTrickyRaceSuite) TestExpireLease_BlockedBy_ExpireThenReclaim(c *gc.C) {

	// Set up a hook to expire the lease and then reclaim it.
	defer txntesting.SetBeforeHooks(c, s.sut.Runner, func() {
		s.blocker.GlobalClock.Advance(90 * time.Second)
		err := s.blocker.Store.ExpireLease(key("name"))
		c.Check(err, jc.ErrorIsNil)
		err = s.blocker.Store.ClaimLease(key("name"), corelease.Request{"holder", time.Minute}, nil)
		c.Check(err, jc.ErrorIsNil)
	})()

	// Try to expire; check it aborts.
	s.sut.GlobalClock.Advance(90 * time.Second)
	err := s.sut.Store.ExpireLease(key("name"))
	c.Check(err, gc.Equals, corelease.ErrInvalid)

	// The SUT has been refreshed, and you can see why the operation was invalid.
	s.sut.LocalClock.Advance(90 * time.Second)
	c.Check(key("name"), s.sut.Expiry(), s.sut.Zero.Add(150*time.Second))
}
