// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/worker/lease"
)

// Tests that check the manager handles leases across namespaces and
// models correctly.

type CrossSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CrossSuite{})

func (s *CrossSuite) TestClaimAcrossNamespaces(c *gc.C) {
	lease1 := key("ns1", "model", "summerisle")
	lease2 := key("ns2", "model", "summerisle")
	fix := Fixture{
		expectCalls: []call{{
			method: "ClaimLease",
			args: []interface{}{
				lease1,
				corelease.Request{"sgt-howie", time.Minute},
			},
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[lease1] = corelease.Info{
					Holder: "sgt-howie",
					Expiry: offset(time.Second),
				}
			},
		}, {
			method: "ClaimLease",
			args: []interface{}{
				lease2,
				corelease.Request{"rowan", time.Minute},
			},
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[lease2] = corelease.Info{
					Holder: "rowan",
					Expiry: offset(time.Second),
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testing.Clock) {
		claimer1, err := manager.Claimer("ns1", "model")
		c.Assert(err, jc.ErrorIsNil)
		claimer2, err := manager.Claimer("ns2", "model")
		c.Assert(err, jc.ErrorIsNil)

		err = claimer1.Claim("summerisle", "sgt-howie", time.Minute)
		c.Assert(err, jc.ErrorIsNil)
		err = claimer2.Claim("summerisle", "rowan", time.Minute)
		c.Assert(err, jc.ErrorIsNil)

		err = claimer1.Claim("summerisle", "lord-summerisle", time.Minute)
		c.Assert(err, gc.Equals, corelease.ErrClaimDenied)
	})
}

func (s *CrossSuite) TestClaimAcrossModels(c *gc.C) {
	lease1 := key("ns", "m1", "summerisle")
	lease2 := key("ns", "m2", "summerisle")
	fix := Fixture{
		expectCalls: []call{{
			method: "ClaimLease",
			args: []interface{}{
				lease1,
				corelease.Request{"sgt-howie", time.Minute},
			},
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[lease1] = corelease.Info{
					Holder: "sgt-howie",
					Expiry: offset(time.Second),
				}
			},
		}, {
			method: "ClaimLease",
			args: []interface{}{
				lease2,
				corelease.Request{"rowan", time.Minute},
			},
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[lease2] = corelease.Info{
					Holder: "rowan",
					Expiry: offset(time.Second),
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testing.Clock) {
		claimer1, err := manager.Claimer("ns", "m1")
		c.Assert(err, jc.ErrorIsNil)
		claimer2, err := manager.Claimer("ns", "m2")
		c.Assert(err, jc.ErrorIsNil)

		err = claimer1.Claim("summerisle", "sgt-howie", time.Minute)
		c.Assert(err, jc.ErrorIsNil)
		err = claimer2.Claim("summerisle", "rowan", time.Minute)
		c.Assert(err, jc.ErrorIsNil)

		err = claimer1.Claim("summerisle", "lord-summerisle", time.Minute)
		c.Assert(err, gc.Equals, corelease.ErrClaimDenied)
	})
}

func (s *CrossSuite) TestWaitAcrossNamespaces(c *gc.C) {
	lease1 := key("ns1", "model", "summerisle")
	lease2 := key("ns2", "model", "summerisle")
	fix := Fixture{
		leases: map[corelease.Key]corelease.Info{
			lease1: {
				Holder: "sgt-howie",
				Expiry: offset(time.Second),
			},
			lease2: {
				Holder: "willow",
				Expiry: offset(time.Minute),
			},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{lease1},
			callback: func(leases map[corelease.Key]corelease.Info) {
				delete(leases, lease1)
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testing.Clock) {
		b1 := newBlockTest(manager, lease1)
		b2 := newBlockTest(manager, lease2)

		b1.assertBlocked(c)
		b2.assertBlocked(c)

		clock.Advance(time.Second)

		err := b1.assertUnblocked(c)
		c.Assert(err, jc.ErrorIsNil)
		b2.assertBlocked(c)
	})
}

func (s *CrossSuite) TestWaitAcrossModels(c *gc.C) {
	lease1 := key("ns", "m1", "summerisle")
	lease2 := key("ns", "m2", "summerisle")
	fix := Fixture{
		leases: map[corelease.Key]corelease.Info{
			lease1: {
				Holder: "sgt-howie",
				Expiry: offset(time.Second),
			},
			lease2: {
				Holder: "willow",
				Expiry: offset(time.Minute),
			},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{lease1},
			callback: func(leases map[corelease.Key]corelease.Info) {
				delete(leases, lease1)
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testing.Clock) {
		b1 := newBlockTest(manager, lease1)
		b2 := newBlockTest(manager, lease2)

		b1.assertBlocked(c)
		b2.assertBlocked(c)

		clock.Advance(time.Second)

		err := b1.assertUnblocked(c)
		c.Assert(err, jc.ErrorIsNil)
		b2.assertBlocked(c)
	})
}

func (s *CrossSuite) TestCheckAcrossNamespaces(c *gc.C) {
	lease1 := key("ns1", "model", "summerisle")
	fix := Fixture{
		leases: map[corelease.Key]corelease.Info{
			lease1: {
				Holder:   "sgt-howie",
				Expiry:   offset(time.Second),
				Trapdoor: corelease.LockedTrapdoor,
			},
		},
		expectCalls: []call{{
			method: "Refresh",
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testing.Clock) {
		checker1, err := manager.Checker("ns1", "model")
		c.Assert(err, jc.ErrorIsNil)
		checker2, err := manager.Checker("ns2", "model")
		c.Assert(err, jc.ErrorIsNil)

		t1 := checker1.Token("summerisle", "sgt-howie")
		c.Assert(t1.Check(nil), gc.Equals, nil)

		t2 := checker2.Token("summerisle", "sgt-howie")
		err = t2.Check(nil)
		c.Assert(errors.Cause(err), gc.Equals, corelease.ErrNotHeld)
	})
}

func (s *CrossSuite) TestCheckAcrossModels(c *gc.C) {
	lease1 := key("ns", "m1", "summerisle")
	fix := Fixture{
		leases: map[corelease.Key]corelease.Info{
			lease1: {
				Holder:   "sgt-howie",
				Expiry:   offset(time.Second),
				Trapdoor: corelease.LockedTrapdoor,
			},
		},
		expectCalls: []call{{
			method: "Refresh",
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testing.Clock) {
		checker1, err := manager.Checker("ns", "m1")
		c.Assert(err, jc.ErrorIsNil)
		checker2, err := manager.Checker("ns", "m2")
		c.Assert(err, jc.ErrorIsNil)

		t1 := checker1.Token("summerisle", "sgt-howie")
		c.Assert(t1.Check(nil), gc.Equals, nil)

		t2 := checker2.Token("summerisle", "sgt-howie")
		err = t2.Check(nil)
		c.Assert(errors.Cause(err), gc.Equals, corelease.ErrNotHeld)
	})
}

func (s *CrossSuite) TestDifferentNamespaceValidation(c *gc.C) {
	clock := testing.NewClock(defaultClockStart)
	store := NewStore(nil, nil)
	manager, err := lease.NewManager(lease.ManagerConfig{
		Clock: clock,
		Store: store,
		Secretary: func(namespace string) (lease.Secretary, error) {
			switch namespace {
			case "ns1":
				return Secretary{}, nil
			case "ns2":
				return OtherSecretary{}, nil
			default:
				return nil, errors.Errorf("bad namespace!")
			}
		},
		MaxSleep: defaultMaxSleep,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		manager.Kill()
		err := manager.Wait()
		c.Check(err, jc.ErrorIsNil)
	}()
	defer store.Wait(c)

	_, err = manager.Claimer("something-else", "model")
	c.Assert(err, gc.ErrorMatches, "bad namespace!")

	c1, err := manager.Claimer("ns1", "model")
	c.Assert(err, jc.ErrorIsNil)
	err = c1.Claim("INVALID", "sgt-howie", time.Minute)
	c.Assert(err, gc.ErrorMatches, `cannot claim lease "INVALID": name not valid`)

	c2, err := manager.Claimer("ns2", "model")
	c.Assert(err, jc.ErrorIsNil)
	err = c2.Claim("INVALID", "sgt-howie", time.Minute)
	c.Assert(err, gc.ErrorMatches, `cannot claim lease "INVALID": lease name not valid`)
}

type OtherSecretary struct{}

// CheckLease is part of the lease.Secretary interface.
func (OtherSecretary) CheckLease(name string) error {
	return errors.NotValidf("lease name")
}

// CheckHolder is part of the lease.Secretary interface.
func (OtherSecretary) CheckHolder(name string) error {
	return errors.NotValidf("holder name")
}

// CheckDuration is part of the lease.Secretary interface.
func (OtherSecretary) CheckDuration(duration time.Duration) error {
	if duration != time.Hour {
		return errors.NotValidf("time")
	}
	return nil
}
