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

var _ = gc.Suite(&TrackerSuite{})

const (
	trackerDuration = coretesting.ShortWait
	leaseDuration   = trackerDuration * 2
)

func refreshes(count int) time.Duration {
	halfRefreshes := (2 * count) + 1
	twiceDuration := trackerDuration * time.Duration(halfRefreshes)
	return twiceDuration / 2
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

func (s *TrackerSuite) unblockRelease(c *gc.C) {
	select {
	case s.manager.releases <- struct{}{}:
	default:
		c.Fatalf("did nobody call BlockUntilLeadershipReleased?")
	}
}

func (s *TrackerSuite) TestOnLeaderSuccess(c *gc.C) {
	tracker := leadership.NewTrackerWorker(s.unitTag, s.manager, trackerDuration)
	defer assertStop(c, tracker)

	// Check the ticket succeeds.
	assertClaimLeader(c, tracker, true)

	// Stop the tracker before trying to look at its stub.
	assertStop(c, tracker)
	s.manager.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}})
}

func (s *TrackerSuite) TestOnLeaderFailure(c *gc.C) {
	s.manager.Stub.Errors = []error{coreleadership.ErrClaimDenied, nil}
	tracker := leadership.NewTrackerWorker(s.unitTag, s.manager, trackerDuration)
	defer assertStop(c, tracker)

	// Check the ticket fails.
	assertClaimLeader(c, tracker, false)

	// Stop the tracker before trying to look at its mocks.
	assertStop(c, tracker)

	// Unblock the release goroutine, lest data races.
	s.unblockRelease(c)

	s.manager.CheckCalls(c, []testing.StubCall{{
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
	s.manager.Stub.Errors = []error{errors.New("pow")}
	tracker := leadership.NewTrackerWorker(s.unitTag, s.manager, trackerDuration)
	defer worker.Stop(tracker)

	// Check the ticket fails.
	assertClaimLeader(c, tracker, false)

	// Stop the tracker before trying to look at its mocks.
	err := worker.Stop(tracker)
	c.Check(err, gc.ErrorMatches, "leadership failure: pow")
	s.manager.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeadership",
		Args: []interface{}{
			"led-service", "led-service/123", leaseDuration,
		},
	}})
}

func (s *TrackerSuite) TestLoseLeadership(c *gc.C) {
	s.manager.Stub.Errors = []error{nil, coreleadership.ErrClaimDenied, nil}
	tracker := leadership.NewTrackerWorker(s.unitTag, s.manager, trackerDuration)
	defer assertStop(c, tracker)

	// Check the first ticket succeeds.
	assertClaimLeader(c, tracker, true)

	// Wait long enough for a single refresh, to trigger ErrClaimDenied; then
	// check the next ticket fails.
	<-time.After(refreshes(1))
	assertClaimLeader(c, tracker, false)

	// Stop the tracker before trying to look at its stub.
	assertStop(c, tracker)

	// Unblock the release goroutine, lest data races.
	s.unblockRelease(c)

	s.manager.CheckCalls(c, []testing.StubCall{{
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
	s.manager.Stub.Errors = []error{coreleadership.ErrClaimDenied, nil, nil}
	tracker := leadership.NewTrackerWorker(s.unitTag, s.manager, trackerDuration)
	defer assertStop(c, tracker)

	// Check initial ticket fails.
	assertClaimLeader(c, tracker, false)

	// Unblock the release goroutine...
	s.unblockRelease(c)

	// ...and, uh, voodoo sleep a bit, but not long enough to trigger a refresh...
	<-time.After(refreshes(0))

	// ...then check the next ticket succeeds.
	assertClaimLeader(c, tracker, true)

	// Stop the tracker before trying to look at its stub.
	assertStop(c, tracker)
	s.manager.CheckCalls(c, []testing.StubCall{{
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
	s.manager.Stub.Errors = []error{
		coreleadership.ErrClaimDenied, nil, coreleadership.ErrClaimDenied, nil,
	}
	tracker := leadership.NewTrackerWorker(s.unitTag, s.manager, trackerDuration)
	defer assertStop(c, tracker)

	// Check initial ticket fails.
	assertClaimLeader(c, tracker, false)

	// Unblock the release goroutine...
	s.unblockRelease(c)

	// ...and, uh, voodoo sleep a bit, but not long enough to trigger a refresh...
	<-time.After(refreshes(0))

	// ...then check the next ticket fails again.
	assertClaimLeader(c, tracker, false)

	// This time, sleep long enough that a refresh would trigger if it were
	// going to...
	<-time.After(refreshes(1))

	// ...but it won't, because we Stop the tracker...
	assertStop(c, tracker)

	// ...and clear out the release goroutine before we look at the stub.
	s.unblockRelease(c)

	s.manager.CheckCalls(c, []testing.StubCall{{
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

func assertClaimLeader(c *gc.C, tracker leadership.Tracker, expect bool) {
	// Grab a ticket...
	ticket := tracker.ClaimLeader()

	// ...and check that it gives the expected result every time it's checked.
	assertTicket(c, ticket, expect)
	assertTicket(c, ticket, expect)
}

func assertTicket(c *gc.C, ticket leadership.Ticket, expect bool) {
	// Wait for the ticket to give a value...
	success := make(chan bool)
	go func() { success <- ticket.Wait() }()
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("value not sent")
	case actual := <-success:
		c.Assert(actual, gc.Equals, expect)
	}
}

func assertStop(c *gc.C, w worker.Worker) {
	c.Assert(worker.Stop(w), jc.ErrorIsNil)
}
