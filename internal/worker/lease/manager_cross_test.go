// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/trace"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/lease"
)

// Tests that check the manager handles leases across namespaces and
// models correctly.

type CrossSuite struct {
	testhelpers.IsolationSuite
}

func TestCrossSuite(t *stdtesting.T) { tc.Run(t, &CrossSuite{}) }
func (s *CrossSuite) testClaims(c *tc.C, lease1, lease2 corelease.Key) {
	fix := Fixture{
		expectCalls: []call{{
			method: "ClaimLease",
			args: []interface{}{
				lease1,
				corelease.Request{Holder: "sgt-howie", Duration: time.Minute},
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
				corelease.Request{Holder: "rowan", Duration: time.Minute},
			},
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[lease2] = corelease.Info{
					Holder: "rowan",
					Expiry: offset(time.Second),
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		claimer1, err := manager.Claimer(lease1.Namespace, lease1.ModelUUID)
		c.Assert(err, tc.ErrorIsNil)
		claimer2, err := manager.Claimer(lease2.Namespace, lease2.ModelUUID)
		c.Assert(err, tc.ErrorIsNil)

		err = claimer1.Claim(lease1.Lease, "sgt-howie", time.Minute)
		c.Assert(err, tc.ErrorIsNil)
		err = claimer2.Claim(lease2.Lease, "rowan", time.Minute)
		c.Assert(err, tc.ErrorIsNil)

		err = claimer1.Claim(lease1.Lease, "lord-summerisle", time.Minute)
		c.Assert(err, tc.Equals, corelease.ErrClaimDenied)
	})
}

func (s *CrossSuite) TestClaimAcrossNamespaces(c *tc.C) {
	s.testClaims(c,
		key("ns1", "model", "summerisle"),
		key("ns2", "model", "summerisle"),
	)
}

func (s *CrossSuite) TestClaimAcrossModels(c *tc.C) {
	s.testClaims(c,
		key("ns", "model1", "summerisle"),
		key("ns", "model2", "summerisle"),
	)
}

func (s *CrossSuite) testWaits(c *tc.C, lease1, lease2 corelease.Key) {
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
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		b1 := newBlockTest(c, manager, lease1)
		b2 := newBlockTest(c, manager, lease2)

		b1.assertBlocked(c)
		b2.assertBlocked(c)

		clock.Advance(2 * time.Second)

		err := b1.assertUnblocked(c)
		c.Assert(err, tc.ErrorIsNil)
		b2.assertBlocked(c)
	})
}

func (s *CrossSuite) TestWaitAcrossNamespaces(c *tc.C) {
	s.testWaits(c,
		key("ns1", "model", "summerisle"),
		key("ns2", "model", "summerisle"),
	)
}

func (s *CrossSuite) TestWaitAcrossModels(c *tc.C) {
	s.testWaits(c,
		key("ns", "model1", "summerisle"),
		key("ns", "model2", "summerisle"),
	)
}

func (s *CrossSuite) testChecks(c *tc.C, lease1, lease2 corelease.Key) {
	fix := Fixture{
		leases: map[corelease.Key]corelease.Info{
			lease1: {
				Holder: "sgt-howie",
				Expiry: offset(time.Second),
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		checker1, err := manager.Checker(lease1.Namespace, lease1.ModelUUID)
		c.Assert(err, tc.ErrorIsNil)
		checker2, err := manager.Checker(lease2.Namespace, lease2.ModelUUID)
		c.Assert(err, tc.ErrorIsNil)

		t1 := checker1.Token(lease1.Lease, "sgt-howie")
		c.Assert(t1.Check(), tc.Equals, nil)

		t2 := checker2.Token(lease2.Lease, "sgt-howie")
		err = t2.Check()
		c.Assert(errors.Cause(err), tc.Equals, corelease.ErrNotHeld)
	})
}

func (s *CrossSuite) TestCheckAcrossNamespaces(c *tc.C) {
	s.testChecks(c,
		key("ns1", "model", "summerisle"),
		key("ns2", "model", "summerisle"),
	)
}

func (s *CrossSuite) TestCheckAcrossModels(c *tc.C) {
	s.testChecks(c,
		key("ns", "model1", "summerisle"),
		key("ns", "model2", "summerisle"),
	)
}

func (s *CrossSuite) TestDifferentNamespaceValidation(c *tc.C) {
	clock := testclock.NewClock(defaultClockStart)
	store := NewStore(nil, nil, clock)
	manager, err := lease.NewManager(lease.ManagerConfig{
		Clock: clock,
		Store: store,
		SecretaryFinder: FuncSecretaryFinder(func(namespace string) (corelease.Secretary, error) {
			switch namespace {
			case "ns1":
				return Secretary{}, nil
			case "ns2":
				return OtherSecretary{}, nil
			default:
				return nil, errors.Errorf("bad namespace!")
			}
		}),
		MaxSleep:             defaultMaxSleep,
		Logger:               loggertesting.WrapCheckLog(c),
		PrometheusRegisterer: noopRegisterer{},
		Tracer:               trace.NoopTracer{},
	})
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		manager.Kill()
		err := manager.Wait()
		c.Check(err, tc.ErrorIsNil)
	}()
	defer store.Wait(c)

	_, err = manager.Claimer("something-else", "model")
	c.Assert(err, tc.ErrorMatches, "bad namespace!")

	c1, err := manager.Claimer("ns1", "model")
	c.Assert(err, tc.ErrorIsNil)
	err = c1.Claim("INVALID", "sgt-howie", time.Minute)
	c.Assert(err, tc.ErrorMatches, `cannot claim lease "INVALID": name not valid`)

	c2, err := manager.Claimer("ns2", "model")
	c.Assert(err, tc.ErrorIsNil)
	err = c2.Claim("INVALID", "sgt-howie", time.Minute)
	c.Assert(err, tc.ErrorMatches, `cannot claim lease "INVALID": lease name not valid`)
}

type OtherSecretary struct{}

// CheckLease is part of the lease.Secretary interface.
func (OtherSecretary) CheckLease(_ corelease.Key) error {
	return errors.NotValidf("lease name")
}

// CheckHolder is part of the lease.Secretary interface.
func (OtherSecretary) CheckHolder(_ string) error {
	return errors.NotValidf("holder name")
}

// CheckDuration is part of the lease.Secretary interface.
func (OtherSecretary) CheckDuration(duration time.Duration) error {
	if duration != time.Hour {
		return errors.NotValidf("time")
	}
	return nil
}
