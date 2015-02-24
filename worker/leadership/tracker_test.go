// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreleadership "github.com/juju/juju/leadership"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/leadership"
)

type TrackerSuite struct {
	testing.IsolationSuite
	unitTag names.UnitTag
	manager *StubLeadershipManager
}

func (s *TrackerSuite) SetUpTest(c *gc.C) {
	s.unitTag = names.NewUnitTag("led-service/123")
	s.manager = &StubLeadershipManager{
		Stub:     &testing.Stub{},
		releases: make(chan struct{}),
	}
}

func (s *TrackerSuite) TearDownTest(c *gc.C) {
	if s.manager != nil {
		// It's not impossible that there's a goroutine waiting for a
		// BlockUntilLeadershipReleased. Make sure it completes.
		close(s.manager.releases)
		s.manager = nil
	}
}

var _ = gc.Suite(&TrackerSuite{})

func (s *TrackerSuite) TestOnLeaderSuccess(c *gc.C) {
	tracker := leadership.NewTrackerWorker(s.unitTag, s.manager, coretesting.ShortWait)
	defer assertStop(c, tracker)

	// Check ticket gets sent true, and is closed afterwards.
	assertSendOnce(c, tracker)

	// Stop the tracker before trying to look at its stub.
	assertStop(c, tracker)
	s.manager.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", coretesting.ShortWait,
		},
	}})
}

func (s *TrackerSuite) TestOnLeaderFailure(c *gc.C) {
	s.manager.Stub.Errors = []error{coreleadership.ErrClaimDenied, nil}
	tracker := leadership.NewTrackerWorker(s.unitTag, s.manager, coretesting.ShortWait)
	defer assertStop(c, tracker)

	// Check ticket gets closed.
	assertCloseTicket(c, tracker)

	// Stop the tracker before trying to look at its mocks.
	assertStop(c, tracker)

	// Unblock the release goroutine, lest data races.
	select {
	case s.manager.releases <- struct{}{}:
	default:
		c.Fatalf("did nobody call BlockUntilLeadershipReleased?")
	}

	s.manager.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", coretesting.ShortWait,
		},
	}, {
		FuncName: "BlockUntilLeadershipReleased",
		Args: []interface{}{
			"led-service",
		},
	}})
}

func (s *TrackerSuite) TestOnLeaderError(c *gc.C) {
	s.manager.Stub.Errors = []error{errors.New("pow")}
	tracker := leadership.NewTrackerWorker(s.unitTag, s.manager, coretesting.ShortWait)
	defer worker.Stop(tracker)

	// Check ticket gets closed.
	assertCloseTicket(c, tracker)

	// Stop the tracker before trying to look at its mocks.
	err := worker.Stop(tracker)
	c.Check(err, gc.ErrorMatches, "leadership failure: pow")
	s.manager.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", coretesting.ShortWait,
		},
	}})
}

func (s *TrackerSuite) TestLoseLeadership(c *gc.C) {
	s.manager.Stub.Errors = []error{nil, coreleadership.ErrClaimDenied, nil}
	tracker := leadership.NewTrackerWorker(s.unitTag, s.manager, coretesting.ShortWait)
	defer assertStop(c, tracker)

	// Check first ticket gets sent true, and then closed.
	assertSendOnce(c, tracker)

	// Wait long enough for a single refresh, to trigger ErrClaimDenied; then
	// check the next ticket gets closed (without sending true).
	<-time.After(coretesting.ShortWait * 3 / 4)
	assertCloseTicket(c, tracker)

	// Stop the tracker before trying to look at its stub.
	assertStop(c, tracker)

	// Unblock the release goroutine, lest data races.
	select {
	case s.manager.releases <- struct{}{}:
	default:
		c.Fatalf("did nobody call BlockUntilLeadershipReleased?")
	}

	s.manager.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", coretesting.ShortWait,
		},
	}, {
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", coretesting.ShortWait,
		},
	}, {
		FuncName: "BlockUntilLeadershipReleased",
		Args: []interface{}{
			"led-service",
		},
	}})
}

func (s *TrackerSuite) TestGainLeadership(c *gc.C) {
	s.manager.Stub.Errors = []error{coreleadership.ErrClaimDenied, nil, nil}
	tracker := leadership.NewTrackerWorker(s.unitTag, s.manager, coretesting.ShortWait)
	defer assertStop(c, tracker)

	// Check initial ticket gets closed.
	assertCloseTicket(c, tracker)

	// Unblock the release goroutine, and... uh, voodoo sleep a bit...
	select {
	case s.manager.releases <- struct{}{}:
	default:
		c.Fatalf("did nobody call BlockUntilLeadershipReleased?")
	}
	<-time.After(coretesting.ShortWait / 4)

	// ...and issue a new ticket, which we expect to receive true before closing.
	assertSendOnce(c, tracker)

	// Stop the tracker before trying to look at its stub.
	assertStop(c, tracker)
	s.manager.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", coretesting.ShortWait,
		},
	}, {
		FuncName: "BlockUntilLeadershipReleased",
		Args: []interface{}{
			"led-service",
		},
	}, {
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", coretesting.ShortWait,
		},
	}})
}

func assertSendOnce(c *gc.C, tracker leadership.Tracker) {
	ticket := make(leadership.Ticket)
	tracker.ClaimLeader(ticket)
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("value not sent")
	case success, ok := <-ticket:
		c.Check(success, jc.IsTrue)
		c.Check(ok, jc.IsTrue)
	}
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("ticket not closed")
	case _, ok := <-ticket:
		c.Check(ok, jc.IsFalse)
	}
}

func assertCloseTicket(c *gc.C, tracker leadership.Tracker) {
	ticket := make(leadership.Ticket)
	tracker.ClaimLeader(ticket)
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("ticket not closed")
	case _, ok := <-ticket:
		c.Check(ok, jc.IsFalse)
	}
}

func assertStop(c *gc.C, w worker.Worker) {
	c.Assert(worker.Stop(w), jc.ErrorIsNil)
}
