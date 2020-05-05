// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	coreleadership "github.com/juju/juju/core/leadership"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/leadership"
)

type TrackerSuite struct {
	testing.IsolationSuite
	unitTag names.UnitTag
	claimer *StubClaimer
	clock   *testclock.Clock
}

var _ = gc.Suite(&TrackerSuite{})

const (
	trackerDuration = 30 * time.Second
	leaseDuration   = trackerDuration * 2
)

func (s *TrackerSuite) refreshes(count int) {
	halfDuration := trackerDuration / 2
	halfRefreshes := (2 * count) + 1
	// The worker often checks against the current time
	// and adds delay to that time. Here we advance the clock
	// in small jumps, and then wait a short time to allow the
	// worker to do stuff.
	for i := 0; i < halfRefreshes; i++ {
		s.clock.Advance(halfDuration)
		<-time.After(coretesting.ShortWait)
	}
}

func (s *TrackerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.unitTag = names.NewUnitTag("led-service/123")
	s.clock = testclock.NewClock(time.Date(2016, 10, 9, 12, 0, 0, 0, time.UTC))
	s.claimer = &StubClaimer{
		Stub:     &testing.Stub{},
		releases: make(chan struct{}),
	}
}

func (s *TrackerSuite) unblockRelease(c *gc.C) {
	select {
	case s.claimer.releases <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("did nobody call BlockUntilLeadershipReleased?")
	}
}

func (s *TrackerSuite) newTrackerInner() *leadership.Tracker {
	return leadership.NewTracker(s.unitTag, s.claimer, s.clock, trackerDuration)
}

func (s *TrackerSuite) newTracker() *leadership.Tracker {
	tracker := s.newTrackerInner()
	s.AddCleanup(func(c *gc.C) {
		workertest.CleanKill(c, tracker)
	})
	return tracker
}

func (s *TrackerSuite) newTrackerDirtyKill() *leadership.Tracker {
	tracker := s.newTrackerInner()
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, tracker)
	})
	return tracker
}

func (s *TrackerSuite) TestApplicationName(c *gc.C) {
	tracker := s.newTracker()
	c.Assert(tracker.ApplicationName(), gc.Equals, "led-service")
}

func (s *TrackerSuite) TestOnLeaderSuccess(c *gc.C) {
	tracker := s.newTracker()

	// Check the ticket succeeds.
	assertClaimLeader(c, tracker, true)

	// Stop the tracker before trying to look at its stub.
	workertest.CleanKill(c, tracker)
	s.claimer.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}})
}

func (s *TrackerSuite) TestOnLeaderFailure(c *gc.C) {
	s.claimer.Stub.SetErrors(coreleadership.ErrClaimDenied, nil)
	tracker := s.newTracker()

	// Check the ticket fails.
	assertClaimLeader(c, tracker, false)

	// wait for calls to stabilize before killing the worker and inspecting the calls.
	timeout := time.After(testing.LongWait)
	next := time.After(0)
	for len(s.claimer.Stub.Calls()) < 2 {
		select {
		case <-next:
			next = time.After(testing.ShortWait)
		case <-timeout:
			c.Fatalf("timed out waiting %s for 2 calls", testing.LongWait)
		}
	}
	// Stop the tracker before trying to look at its mocks.
	workertest.CleanKill(c, tracker)

	s.claimer.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}, {
		FuncName: "BlockUntilLeadershipReleased",
		Args: []interface{}{
			"led-service",
		},
	}})
}

func (s *TrackerSuite) TestOnLeaderError(c *gc.C) {
	s.claimer.Stub.SetErrors(errors.New("pow"))
	tracker := s.newTrackerDirtyKill()

	// Check the ticket fails.
	assertClaimLeader(c, tracker, false)

	// Stop the tracker before trying to look at its mocks.
	err := worker.Stop(tracker)
	c.Check(err, gc.ErrorMatches, "leadership failure: pow")
	s.claimer.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}})
}

func (s *TrackerSuite) TestLoseLeadership(c *gc.C) {
	s.claimer.Stub.SetErrors(nil, coreleadership.ErrClaimDenied, nil)
	tracker := s.newTracker()

	// Check the first ticket succeeds.
	assertClaimLeader(c, tracker, true)

	// Wait long enough for a single refresh, to trigger ErrClaimDenied; then
	// check the next ticket fails.
	s.refreshes(1)
	assertClaimLeader(c, tracker, false)

	// Stop the tracker before trying to look at its stub.
	workertest.CleanKill(c, tracker)

	s.claimer.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}, {
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}, {
		FuncName: "BlockUntilLeadershipReleased",
		Args: []interface{}{
			"led-service",
		},
	}})
}

func (s *TrackerSuite) TestGainLeadership(c *gc.C) {
	s.claimer.Stub.SetErrors(coreleadership.ErrClaimDenied, nil, nil)
	tracker := s.newTracker()

	// Check initial ticket fails.
	assertClaimLeader(c, tracker, false)

	// Unblock the release goroutine...
	s.unblockRelease(c)

	// advance the clock a small amount, but not enough to trigger a check
	s.refreshes(0)

	// ...then check the next ticket succeeds.
	assertClaimLeader(c, tracker, true)

	// Stop the tracker before trying to look at its stub.
	workertest.CleanKill(c, tracker)
	s.claimer.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}, {
		FuncName: "BlockUntilLeadershipReleased",
		Args: []interface{}{
			"led-service",
		},
	}, {
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}})
}

func (s *TrackerSuite) TestFailGainLeadership(c *gc.C) {
	s.claimer.Stub.SetErrors(
		coreleadership.ErrClaimDenied, nil, coreleadership.ErrClaimDenied, nil,
	)
	tracker := s.newTracker()

	// Check initial ticket fails.
	assertClaimLeader(c, tracker, false)

	// Unblock the release goroutine...
	s.unblockRelease(c)

	// advance the clock a small amount, but not enough to trigger a check
	s.refreshes(0)

	// ...then check the next ticket fails again.
	assertClaimLeader(c, tracker, false)

	// This time, advance far enough that a refresh would trigger if it were
	// going to...
	s.refreshes(1)

	// ...but it won't, because we Stop the tracker...
	workertest.CleanKill(c, tracker)

	s.claimer.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}, {
		FuncName: "BlockUntilLeadershipReleased",
		Args: []interface{}{
			"led-service",
		},
	}, {
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}, {
		FuncName: "BlockUntilLeadershipReleased",
		Args: []interface{}{
			"led-service",
		},
	}})
}

func (s *TrackerSuite) TestWaitLeaderAlreadyLeader(c *gc.C) {
	tracker := s.newTracker()

	// Check the ticket succeeds.
	assertWaitLeader(c, tracker, true)

	// Stop the tracker before trying to look at its stub.
	workertest.CleanKill(c, tracker)
	s.claimer.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}})
}

func (s *TrackerSuite) TestWaitLeaderBecomeLeader(c *gc.C) {
	s.claimer.Stub.SetErrors(coreleadership.ErrClaimDenied, nil, nil)
	tracker := s.newTracker()

	// Check initial ticket fails.
	assertWaitLeader(c, tracker, false)

	// Unblock the release goroutine...
	s.unblockRelease(c)

	// advance the clock a small amount, but not enough to trigger a check
	s.refreshes(0)

	// ...then check the next ticket succeeds.
	assertWaitLeader(c, tracker, true)

	// Stop the tracker before trying to look at its stub.
	workertest.CleanKill(c, tracker)
	s.claimer.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}, {
		FuncName: "BlockUntilLeadershipReleased",
		Args: []interface{}{
			"led-service",
		},
	}, {
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}})
}

func (s *TrackerSuite) TestWaitLeaderNeverBecomeLeader(c *gc.C) {
	s.claimer.Stub.SetErrors(coreleadership.ErrClaimDenied, nil)
	tracker := s.newTracker()

	// Check initial ticket fails.
	assertWaitLeader(c, tracker, false)

	// Get a new ticket and stop the tracker while it's pending.
	ticket := tracker.WaitLeader()
	workertest.CleanKill(c, tracker)

	// Check the ticket got closed without sending true.
	assertTicket(c, ticket, false)
	assertTicket(c, ticket, false)

	s.claimer.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}, {
		FuncName: "BlockUntilLeadershipReleased",
		Args: []interface{}{
			"led-service",
		},
	}})
}

func (s *TrackerSuite) TestWaitMinionAlreadyMinion(c *gc.C) {
	s.claimer.Stub.SetErrors(coreleadership.ErrClaimDenied, nil)
	tracker := s.newTracker()

	// Check initial ticket is closed immediately.
	assertWaitLeader(c, tracker, false)

	// Stop the tracker before trying to look at its stub.
	workertest.CleanKill(c, tracker)
	s.claimer.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}, {
		FuncName: "BlockUntilLeadershipReleased",
		Args: []interface{}{
			"led-service",
		},
	}})
}

func (s *TrackerSuite) TestWaitMinionClaimerFails(c *gc.C) {
	s.claimer.Stub.SetErrors(coreleadership.ErrClaimDenied, errors.New("mein leben!"))
	tracker := s.newTrackerDirtyKill()
	s.unblockRelease(c)

	err := workertest.CheckKilled(c, tracker)
	c.Assert(err, gc.ErrorMatches, "error while led-service/123 waiting for led-service leadership release: mein leben!")
}

func (s *TrackerSuite) TestWaitMinionBecomeMinion(c *gc.C) {
	s.claimer.Stub.SetErrors(nil, coreleadership.ErrClaimDenied, nil)
	tracker := s.newTracker()

	// Check the first ticket stays open.
	assertWaitMinion(c, tracker, false)

	// Wait long enough for a single refresh, to trigger ErrClaimDenied; then
	// check the next ticket is closed.
	s.refreshes(1)
	assertWaitMinion(c, tracker, true)

	// Stop the tracker before trying to look at its stub.
	workertest.CleanKill(c, tracker)

	s.claimer.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}, {
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}, {
		FuncName: "BlockUntilLeadershipReleased",
		Args: []interface{}{
			"led-service",
		},
	}})
}

func (s *TrackerSuite) TestWaitMinionNeverBecomeMinion(c *gc.C) {
	tracker := s.newTracker()

	ticket := tracker.WaitMinion()
	s.refreshes(2)

	select {
	case <-ticket.Ready():
		c.Fatalf("got unexpected readiness: %v", ticket.Wait())
	default:
		// fallthrough
	}

	s.claimer.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}, {
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}, {
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}})
}

func assertClaimLeader(c *gc.C, tracker *leadership.Tracker, expect bool) {
	// Grab a ticket...
	ticket := tracker.ClaimLeader()

	// ...and check that it gives the expected result every time it's checked.
	assertTicket(c, ticket, expect)
	assertTicket(c, ticket, expect)
}

func assertWaitLeader(c *gc.C, tracker *leadership.Tracker, expect bool) {
	ticket := tracker.WaitLeader()
	if expect {
		assertTicket(c, ticket, true)
		assertTicket(c, ticket, true)
		return
	}
	select {
	case <-time.After(coretesting.ShortWait):
		// This wait needs to be small, compared to the resolution we run the
		// tests at, so as not to disturb client timing too much.
	case <-ticket.Ready():
		c.Fatalf("got unexpected readiness: %v", ticket.Wait())
	}
}

func assertWaitMinion(c *gc.C, tracker *leadership.Tracker, expect bool) {
	ticket := tracker.WaitMinion()
	if expect {
		assertTicket(c, ticket, false)
		assertTicket(c, ticket, false)
		return
	}
	select {
	case <-time.After(coretesting.ShortWait):
		// This wait needs to be small, compared to the resolution we run the
		// tests at, so as not to disturb client timing too much.
	case <-ticket.Ready():
		c.Fatalf("got unexpected readiness: %v", ticket.Wait())
	}
}

func assertTicket(c *gc.C, ticket coreleadership.Ticket, expect bool) {
	// Wait for the ticket to give a value...
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("value not sent")
	case <-ticket.Ready():
		c.Assert(ticket.Wait(), gc.Equals, expect)
	}
}
