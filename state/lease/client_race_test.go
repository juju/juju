// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	txntesting "github.com/juju/txn/testing"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	_ "gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state/lease"
)

// ClientSimpleRaceSuite tests what happens when two clients interfere with
// each other when creating clients and/or leases.
type ClientSimpleRaceSuite struct {
	FixtureSuite
}

var _ = gc.Suite(&ClientSimpleRaceSuite{})

func (s *ClientSimpleRaceSuite) TestNewClient_WorksDespite_CreateClockRace(c *gc.C) {
	config := func(id string) lease.ClientConfig {
		return lease.ClientConfig{
			Id:         id,
			Namespace:  "ns",
			Collection: "leases",
			Mongo:      NewMongo(s.db),
			Clock:      clock.WallClock,
		}
	}
	sutConfig := config("sut")
	sutRunner := sutConfig.Mongo.(*Mongo).runner

	// Set up a hook to create the clock doc (and write some important data to
	// it)  by creating another client before the SUT gets a chance.
	defer txntesting.SetBeforeHooks(c, sutRunner, func() {
		client, err := lease.NewClient(config("blocker"))
		c.Check(err, jc.ErrorIsNil)
		err = client.ClaimLease("somewhere", lease.Request{"someone", time.Minute})
		c.Check(err, jc.ErrorIsNil)
	})()

	// Create a client against an apparently-empty namespace.
	client, err := lease.NewClient(sutConfig)
	c.Check(err, jc.ErrorIsNil)

	// Despite the scramble, it's generated with recent lease data and no error.
	leases := client.Leases()
	info, found := leases["somewhere"]
	c.Check(found, jc.IsTrue)
	c.Check(info.Holder, gc.Equals, "someone")
}

func (s *ClientSimpleRaceSuite) TestClaimLease_BlockedBy_ClaimLease(c *gc.C) {
	sut := s.EasyFixture(c)
	blocker := s.NewFixture(c, FixtureParams{Id: "blocker"})

	// Set up a hook to grab the lease "name" just before the next txn runs.
	defer txntesting.SetBeforeHooks(c, sut.Runner, func() {
		err := blocker.Client.ClaimLease("name", lease.Request{"ha-haa", time.Minute})
		c.Check(err, jc.ErrorIsNil)
	})()

	// Try to grab the lease "name", and fail.
	err := sut.Client.ClaimLease("name", lease.Request{"trying", time.Second})
	c.Check(err, gc.Equals, lease.ErrInvalid)

	// The client that failed has refreshed state (as it had to, in order
	// to discover the reason for the invalidity).
	c.Check("name", sut.Holder(), "ha-haa")
	c.Check("name", sut.Expiry(), sut.Zero.Add(time.Minute))
}

func (s *ClientSimpleRaceSuite) TestClaimLease_Pathological(c *gc.C) {
	sut := s.EasyFixture(c)
	blocker := s.NewFixture(c, FixtureParams{Id: "blocker"})

	// Set up hooks to claim a lease just before every transaction, but remove
	// it again before the SUT goes and looks to figure out what it should do.
	interfere := jujutxn.TestHook{
		Before: func() {
			err := blocker.Client.ClaimLease("name", lease.Request{"ha-haa", time.Second})
			c.Check(err, jc.ErrorIsNil)
		},
		After: func() {
			blocker.Clock.Advance(time.Minute)
			err := blocker.Client.ExpireLease("name")
			c.Check(err, jc.ErrorIsNil)
		},
	}
	defer txntesting.SetTestHooks(
		c, sut.Runner,
		interfere, interfere, interfere,
	)()

	// Try to claim, and watch the poor thing collapse in exhaustion.
	err := sut.Client.ClaimLease("name", lease.Request{"trying", time.Minute})
	c.Check(err, gc.ErrorMatches, "state changing too quickly; try again soon")
}

// ClientTrickyRaceSuite tests what happens when two clients interfere with
// each other when extending and/or expiring leases.
type ClientTrickyRaceSuite struct {
	FixtureSuite
	sut     *Fixture
	blocker *Fixture
}

var _ = gc.Suite(&ClientTrickyRaceSuite{})

func (s *ClientTrickyRaceSuite) SetUpTest(c *gc.C) {
	s.FixtureSuite.SetUpTest(c)
	s.sut = s.EasyFixture(c)
	err := s.sut.Client.ClaimLease("name", lease.Request{"holder", time.Minute})
	c.Assert(err, jc.ErrorIsNil)
	s.blocker = s.NewFixture(c, FixtureParams{Id: "blocker"})
}

func (s *ClientTrickyRaceSuite) TestExtendLease_WorksDespite_ShorterExtendLease(c *gc.C) {

	shorterRequest := 90 * time.Second
	longerRequest := 120 * time.Second

	// Set up hooks to extend the lease by a little, before the SUT's extend
	// gets a chance; and then to verify state after it's applied its retry.
	defer txntesting.SetRetryHooks(c, s.sut.Runner, func() {
		err := s.blocker.Client.ExtendLease("name", lease.Request{"holder", shorterRequest})
		c.Check(err, jc.ErrorIsNil)
	}, func() {
		err := s.blocker.Client.Refresh()
		c.Check(err, jc.ErrorIsNil)
		c.Check("name", s.blocker.Expiry(), s.blocker.Zero.Add(longerRequest))
	})()

	// Extend the lease.
	err := s.sut.Client.ExtendLease("name", lease.Request{"holder", longerRequest})
	c.Check(err, jc.ErrorIsNil)
}

func (s *ClientTrickyRaceSuite) TestExtendLease_WorksDespite_LongerExtendLease(c *gc.C) {

	shorterRequest := 90 * time.Second
	longerRequest := 120 * time.Second

	// Set up hooks to extend the lease by a lot, before the SUT's extend can.
	defer txntesting.SetBeforeHooks(c, s.sut.Runner, func() {
		err := s.blocker.Client.ExtendLease("name", lease.Request{"holder", longerRequest})
		c.Check(err, jc.ErrorIsNil)
	})()

	// Extend the lease by a little.
	err := s.sut.Client.ExtendLease("name", lease.Request{"holder", shorterRequest})
	c.Check(err, jc.ErrorIsNil)

	// The SUT was refreshed, and knows that the lease is really valid for longer.
	c.Check("name", s.sut.Expiry(), s.sut.Zero.Add(longerRequest))
}

func (s *ClientTrickyRaceSuite) TestExtendLease_BlockedBy_ExpireLease(c *gc.C) {

	// Set up a hook to expire the lease before the extend gets a chance.
	defer txntesting.SetBeforeHooks(c, s.sut.Runner, func() {
		s.blocker.Clock.Advance(90 * time.Second)
		err := s.blocker.Client.ExpireLease("name")
		c.Check(err, jc.ErrorIsNil)
	})()

	// Try to extend; check it aborts.
	err := s.sut.Client.ExtendLease("name", lease.Request{"holder", 2 * time.Minute})
	c.Check(err, gc.Equals, lease.ErrInvalid)

	// The SUT has been refreshed, and you can see why the operation was invalid.
	c.Check("name", s.sut.Holder(), "")
}

func (s *ClientTrickyRaceSuite) TestExtendLease_BlockedBy_ExpireThenReclaimDifferentHolder(c *gc.C) {

	// Set up a hook to expire and reclaim the lease before the extend gets a
	// chance.
	defer txntesting.SetBeforeHooks(c, s.sut.Runner, func() {
		s.blocker.Clock.Advance(90 * time.Second)
		err := s.blocker.Client.ExpireLease("name")
		c.Check(err, jc.ErrorIsNil)
		err = s.blocker.Client.ClaimLease("name", lease.Request{"different-holder", time.Minute})
		c.Check(err, jc.ErrorIsNil)
	})()

	// Try to extend; check it aborts.
	err := s.sut.Client.ExtendLease("name", lease.Request{"holder", 2 * time.Minute})
	c.Check(err, gc.Equals, lease.ErrInvalid)

	// The SUT has been refreshed, and you can see why the operation was invalid.
	c.Check("name", s.sut.Holder(), "different-holder")
}

func (s *ClientTrickyRaceSuite) TestExtendLease_WorksDespite_ExpireThenReclaimSameHolder(c *gc.C) {

	// Set up hooks to expire and reclaim the lease before the extend gets a
	// chance; and to verify that the second attempt successfully extends.
	defer txntesting.SetRetryHooks(c, s.sut.Runner, func() {
		s.blocker.Clock.Advance(90 * time.Second)
		err := s.blocker.Client.ExpireLease("name")
		c.Check(err, jc.ErrorIsNil)
		err = s.blocker.Client.ClaimLease("name", lease.Request{"holder", time.Minute})
		c.Check(err, jc.ErrorIsNil)
	}, func() {
		err := s.blocker.Client.Refresh()
		c.Check(err, jc.ErrorIsNil)
		c.Check("name", s.blocker.Expiry(), s.blocker.Zero.Add(5*time.Minute))
	})()

	// Try to extend; check it worked.
	err := s.sut.Client.ExtendLease("name", lease.Request{"holder", 5 * time.Minute})
	c.Check(err, jc.ErrorIsNil)
}

func (s *ClientTrickyRaceSuite) TestExtendLease_Pathological(c *gc.C) {

	// Set up hooks to remove the lease just before every transaction, but
	// replace it before the SUT goes and looks to figure out what it should do.
	interfere := jujutxn.TestHook{
		Before: func() {
			s.blocker.Clock.Advance(time.Minute + time.Second)
			err := s.blocker.Client.ExpireLease("name")
			c.Check(err, jc.ErrorIsNil)
		},
		After: func() {
			err := s.blocker.Client.ClaimLease("name", lease.Request{"holder", time.Second})
			c.Check(err, jc.ErrorIsNil)
		},
	}
	defer txntesting.SetTestHooks(
		c, s.sut.Runner,
		interfere, interfere, interfere,
	)()

	// Try to extend, and watch the poor thing collapse in exhaustion.
	err := s.sut.Client.ExtendLease("name", lease.Request{"holder", time.Minute})
	c.Check(err, gc.ErrorMatches, "state changing too quickly; try again soon")
}

func (s *ClientTrickyRaceSuite) TestExpireLease_BlockedBy_ExtendLease(c *gc.C) {

	// Set up a hook to extend the lease before the expire gets a chance.
	defer txntesting.SetBeforeHooks(c, s.sut.Runner, func() {
		s.blocker.Clock.Advance(90 * time.Second)
		err := s.blocker.Client.ExtendLease("name", lease.Request{"holder", 30 * time.Second})
		c.Check(err, jc.ErrorIsNil)
	})()

	// Try to expire; check it aborts.
	s.sut.Clock.Advance(90 * time.Second)
	err := s.sut.Client.ExpireLease("name")
	c.Check(err, gc.Equals, lease.ErrInvalid)

	// The SUT has been refreshed, and you can see why the operation was invalid.
	c.Check("name", s.sut.Expiry(), s.sut.Zero.Add(2*time.Minute))
}

func (s *ClientTrickyRaceSuite) TestExpireLease_BlockedBy_ExpireLease(c *gc.C) {

	// Set up a hook to expire the lease before the SUT gets a chance.
	defer txntesting.SetBeforeHooks(c, s.sut.Runner, func() {
		s.blocker.Clock.Advance(90 * time.Second)
		err := s.blocker.Client.ExpireLease("name")
		c.Check(err, jc.ErrorIsNil)
	})()

	// Try to expire; check it aborts.
	s.sut.Clock.Advance(90 * time.Second)
	err := s.sut.Client.ExpireLease("name")
	c.Check(err, gc.Equals, lease.ErrInvalid)

	// The SUT has been refreshed, and you can see why the operation was invalid.
	c.Check("name", s.sut.Holder(), "")
}

func (s *ClientTrickyRaceSuite) TestExpireLease_BlockedBy_ExpireThenReclaim(c *gc.C) {

	// Set up a hook to expire the lease and then reclaim it.
	defer txntesting.SetBeforeHooks(c, s.sut.Runner, func() {
		s.blocker.Clock.Advance(90 * time.Second)
		err := s.blocker.Client.ExpireLease("name")
		c.Check(err, jc.ErrorIsNil)
		err = s.blocker.Client.ClaimLease("name", lease.Request{"holder", time.Minute})
		c.Check(err, jc.ErrorIsNil)
	})()

	// Try to expire; check it aborts.
	s.sut.Clock.Advance(90 * time.Second)
	err := s.sut.Client.ExpireLease("name")
	c.Check(err, gc.Equals, lease.ErrInvalid)

	// The SUT has been refreshed, and you can see why the operation was invalid.
	c.Check("name", s.sut.Expiry(), s.sut.Zero.Add(150*time.Second))
}
