// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
)

type LeaderSuite struct {
	testhelpers.IsolationSuite
	testhelpers.Stub
	tracker *StubTracker
	context context.LeadershipContext
}

func TestLeaderSuite(t *testing.T) {
	tc.Run(t, &LeaderSuite{})
}

func (s *LeaderSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.tracker = &StubTracker{
		Stub:            &s.Stub,
		applicationName: "led-application",
	}
	s.context = context.NewLeadershipContext(s.tracker)
}

func (s *LeaderSuite) CheckCalls(c *tc.C, stubCalls []testhelpers.StubCall, f func()) {
	s.Stub = testhelpers.Stub{}
	f()
	s.Stub.CheckCalls(c, stubCalls)
}

func (s *LeaderSuite) TestIsLeaderSuccess(c *tc.C) {
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The first call succeeds...
		s.tracker.results = []StubTicket{true}
		leader, err := s.context.IsLeader()
		c.Check(leader, tc.IsTrue)
		c.Check(err, tc.ErrorIsNil)
	})

	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// ...and so does the second.
		s.tracker.results = []StubTicket{true}
		leader, err := s.context.IsLeader()
		c.Check(leader, tc.IsTrue)
		c.Check(err, tc.ErrorIsNil)
	})
}

func (s *LeaderSuite) TestIsLeaderFailure(c *tc.C) {
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The first call fails...
		s.tracker.results = []StubTicket{false}
		leader, err := s.context.IsLeader()
		c.Check(leader, tc.IsFalse)
		c.Check(err, tc.ErrorIsNil)
	})

	s.CheckCalls(c, nil, func() {
		// ...and the second doesn't even try.
		leader, err := s.context.IsLeader()
		c.Check(leader, tc.IsFalse)
		c.Check(err, tc.ErrorIsNil)
	})
}

func (s *LeaderSuite) TestIsLeaderFailureAfterSuccess(c *tc.C) {
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The first call succeeds...
		s.tracker.results = []StubTicket{true}
		leader, err := s.context.IsLeader()
		c.Check(leader, tc.IsTrue)
		c.Check(err, tc.ErrorIsNil)
	})

	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The second fails...
		s.tracker.results = []StubTicket{false}
		leader, err := s.context.IsLeader()
		c.Check(leader, tc.IsFalse)
		c.Check(err, tc.ErrorIsNil)
	})

	s.CheckCalls(c, nil, func() {
		// The third doesn't even try.
		leader, err := s.context.IsLeader()
		c.Check(leader, tc.IsFalse)
		c.Check(err, tc.ErrorIsNil)
	})
}

type StubTracker struct {
	leadership.Tracker
	*testhelpers.Stub
	applicationName string
	results         []StubTicket
}

func (stub *StubTracker) ApplicationName() string {
	stub.MethodCall(stub, "ApplicationName")
	return stub.applicationName
}

func (stub *StubTracker) ClaimLeader() (result leadership.Ticket) {
	stub.MethodCall(stub, "ClaimLeader")
	result, stub.results = stub.results[0], stub.results[1:]
	return result
}

type StubTicket bool

func (ticket StubTicket) Wait() bool {
	return bool(ticket)
}

func (ticket StubTicket) Ready() <-chan struct{} {
	return alwaysReady
}

var alwaysReady = make(chan struct{})

func init() {
	close(alwaysReady)
}
