// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"testing"
	"time"

	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) { gc.TestingT(t) }

const (
	StubServiceNm = "stub-service"
	StubUnitNm    = "stub-unit/0"
)

var (
	_                        = gc.Suite(&leadershipSuite{})
	_ LeadershipLeaseManager = (*leaseStub)(nil)
)

type leadershipSuite struct{}

type leaseStub struct {
	ClaimLeaseFn            func(string, string, time.Duration) (string, error)
	ReleaseLeaseFn          func(string, string) error
	LeaseReleasedNotifierFn func(string) <-chan struct{}
}

func (s *leaseStub) ClaimLease(namespace, id string, forDur time.Duration) (string, error) {
	if s.ClaimLeaseFn != nil {
		return s.ClaimLeaseFn(namespace, id, forDur)
	}
	return id, nil
}

func (s *leaseStub) ReleaseLease(namespace, id string) error {
	if s.ReleaseLeaseFn != nil {
		return s.ReleaseLeaseFn(namespace, id)
	}
	return nil
}

func (s *leaseStub) LeaseReleasedNotifier(namespace string) <-chan struct{} {
	if s.LeaseReleasedNotifierFn != nil {
		return s.LeaseReleasedNotifierFn(namespace)
	}
	return nil
}

func (s *leadershipSuite) TestClaimLeadershipTranslation(c *gc.C) {
	stub := &leaseStub{
		ClaimLeaseFn: func(namespace, id string, forDur time.Duration) (string, error) {
			c.Check(namespace, gc.Equals, leadershipNamespace(StubServiceNm))
			c.Check(id, gc.Equals, StubUnitNm)
			c.Check(forDur, gc.Equals, leadershipDuration)
			return id, nil
		},
	}

	leaderMgr := NewLeadershipManager(stub)
	leadDur, err := leaderMgr.ClaimLeadership(StubServiceNm, StubUnitNm)

	c.Check(leadDur, gc.Equals, leadershipDuration)
	c.Check(err, gc.IsNil)
}

func (s *leadershipSuite) TestReleaseLeadershipTranslation(c *gc.C) {

	numStubCalls := 0
	stub := &leaseStub{
		ReleaseLeaseFn: func(namespace, id string) error {
			numStubCalls++
			c.Check(namespace, gc.Equals, leadershipNamespace(StubServiceNm))
			c.Check(id, gc.Equals, StubUnitNm)
			return nil
		},
	}

	leaderMgr := NewLeadershipManager(stub)
	err := leaderMgr.ReleaseLeadership(StubServiceNm, StubUnitNm)

	c.Check(numStubCalls, gc.Equals, 1)
	c.Check(err, gc.IsNil)
}

func (s *leadershipSuite) TestBlockUntilLeadershipReleasedTranslation(c *gc.C) {

	numStubCalls := 0
	stub := &leaseStub{
		LeaseReleasedNotifierFn: func(namespace string) <-chan struct{} {
			numStubCalls++
			c.Check(namespace, gc.Equals, leadershipNamespace(StubServiceNm))
			// Send something pre-emptively so test doesn't block.
			released := make(chan struct{}, 1)
			released <- struct{}{}
			return released
		},
	}

	leaderMgr := NewLeadershipManager(stub)
	err := leaderMgr.BlockUntilLeadershipReleased(StubServiceNm)

	c.Check(numStubCalls, gc.Equals, 1)
	c.Check(err, gc.IsNil)
}
