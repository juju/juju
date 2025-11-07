// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"context"
	"errors"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreleadership "github.com/juju/juju/core/leadership"
	"github.com/juju/juju/internal/worker/leadership"
	coretesting "github.com/juju/juju/testing"
)

type TrackerSuite struct {
	testing.IsolationSuite
	unitTag names.UnitTag

	claimer *MockClaimer
	clock   testclock.AdvanceableClock

	claimLeaderErrors        []error
	blockUntilReleasedErrors []error
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
	s.clock = testclock.NewDilatedWallClock(coretesting.ShortWait)
}

func (s *TrackerSuite) expectClaimLeadership() {
	s.claimer.EXPECT().ClaimLeadership("led-service", "led-service/123", leaseDuration).
		DoAndReturn(func(appName, unitName string, leaseDuration time.Duration) error {
			var next error
			if len(s.claimLeaderErrors) > 0 {
				next = s.claimLeaderErrors[0]
				s.claimLeaderErrors = s.claimLeaderErrors[1:]
			}
			return next
		}).AnyTimes()
}

func (s *TrackerSuite) maybeExpectBlockUntilLeadershipReleased(releases chan struct{}) {
	s.claimer.EXPECT().BlockUntilLeadershipReleased("led-service", gomock.Any()).
		DoAndReturn(func(appName string, cancel <-chan struct{}) error {
			select {
			case <-cancel:
				return coreleadership.ErrBlockCancelled
			case <-releases:
			}
			var next error
			if len(s.blockUntilReleasedErrors) > 0 {
				next = s.blockUntilReleasedErrors[0]
				s.blockUntilReleasedErrors = s.blockUntilReleasedErrors[1:]
			}
			return next
		}).AnyTimes()
}

func (s *TrackerSuite) unblockRelease(c *gc.C, releases chan struct{}) {
	select {
	case releases <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("did nobody call BlockUntilLeadershipReleased?")
	}
}

func (s *TrackerSuite) newTrackerInner() *leadership.Tracker {
	s.expectClaimLeadership()

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
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.claimer = NewMockClaimer(ctrl)
	releases := make(chan struct{})
	s.maybeExpectBlockUntilLeadershipReleased(releases)

	tracker := s.newTracker()
	c.Assert(tracker.ApplicationName(), gc.Equals, "led-service")
}

func (s *TrackerSuite) TestOnLeaderSuccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.claimer = NewMockClaimer(ctrl)
	releases := make(chan struct{})
	s.maybeExpectBlockUntilLeadershipReleased(releases)

	tracker := s.newTracker()

	// Check the ticket succeeds.
	assertClaimLeader(c, tracker, true)

	workertest.CleanKill(c, tracker)
}

func (s *TrackerSuite) TestOnLeaderFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.claimer = NewMockClaimer(ctrl)
	releases := make(chan struct{})
	s.maybeExpectBlockUntilLeadershipReleased(releases)

	s.claimLeaderErrors = []error{coreleadership.ErrClaimDenied}
	tracker := s.newTracker()

	// Check the ticket fails.
	assertClaimLeader(c, tracker, false)

	workertest.CleanKill(c, tracker)
}

func (s *TrackerSuite) TestOnLeaderError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.claimer = NewMockClaimer(ctrl)
	releases := make(chan struct{})
	s.maybeExpectBlockUntilLeadershipReleased(releases)

	s.claimLeaderErrors = []error{errors.New("pow")}
	tracker := s.newTrackerDirtyKill()

	// Check the ticket fails.
	assertClaimLeader(c, tracker, false)

	// Stop the tracker before trying to look at its mocks.
	err := worker.Stop(tracker)
	c.Check(err, gc.ErrorMatches, "leadership failure: pow")
}

func (s *TrackerSuite) TestLoseLeadership(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.claimer = NewMockClaimer(ctrl)
	releases := make(chan struct{})
	s.maybeExpectBlockUntilLeadershipReleased(releases)

	s.claimLeaderErrors = []error{nil, coreleadership.ErrClaimDenied}
	tracker := s.newTracker()

	// Check the first ticket succeeds.
	assertClaimLeader(c, tracker, true)

	// Wait long enough for a single refresh, to trigger ErrClaimDenied; then
	// check the next ticket fails.
	s.refreshes(1)
	assertClaimLeader(c, tracker, false)

	workertest.CleanKill(c, tracker)
}

func (s *TrackerSuite) TestGainLeadership(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.claimer = NewMockClaimer(ctrl)
	releases := make(chan struct{})
	s.maybeExpectBlockUntilLeadershipReleased(releases)

	s.claimLeaderErrors = []error{coreleadership.ErrClaimDenied}

	tracker := s.newTracker()

	// Check initial ticket fails.
	assertClaimLeader(c, tracker, false)

	// Unblock the release goroutine...
	s.unblockRelease(c, releases)

	// advance the clock a small amount, but not enough to trigger a check
	s.refreshes(0)

	// ...then check the next ticket succeeds.
	assertClaimLeader(c, tracker, true)

	workertest.CleanKill(c, tracker)
}

func (s *TrackerSuite) TestFailGainLeadership(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.claimer = NewMockClaimer(ctrl)
	releases := make(chan struct{})
	s.maybeExpectBlockUntilLeadershipReleased(releases)

	s.claimLeaderErrors = []error{coreleadership.ErrClaimDenied, coreleadership.ErrClaimDenied}
	tracker := s.newTracker()

	// Check initial ticket fails.
	assertClaimLeader(c, tracker, false)

	// Unblock the release goroutine...
	s.unblockRelease(c, releases)

	// advance the clock a small amount, but not enough to trigger a check
	s.refreshes(0)

	// ...then check the next ticket fails again.
	assertClaimLeader(c, tracker, false)

	// This time, advance far enough that a refresh would trigger if it were
	// going to...
	s.refreshes(1)

	// ...but it won't, because we Stop the tracker...
	workertest.CleanKill(c, tracker)
}

func (s *TrackerSuite) TestWaitLeaderAlreadyLeader(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.claimer = NewMockClaimer(ctrl)
	releases := make(chan struct{})
	s.maybeExpectBlockUntilLeadershipReleased(releases)

	tracker := s.newTracker()

	// Check the ticket succeeds.
	assertWaitLeader(c, tracker, true)

	workertest.CleanKill(c, tracker)
}

func (s *TrackerSuite) TestWaitLeaderBecomeLeader(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.claimer = NewMockClaimer(ctrl)
	releases := make(chan struct{})
	s.maybeExpectBlockUntilLeadershipReleased(releases)

	s.claimLeaderErrors = []error{coreleadership.ErrClaimDenied}

	tracker := s.newTracker()

	// Check initial ticket fails.
	assertWaitLeader(c, tracker, false)

	// Unblock the release goroutine...
	s.unblockRelease(c, releases)

	// advance the clock a small amount, but not enough to trigger a check
	s.refreshes(0)

	// ...then check the next ticket succeeds.
	assertWaitLeader(c, tracker, true)

	workertest.CleanKill(c, tracker)
}

func (s *TrackerSuite) TestWaitLeaderNeverBecomeLeader(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.claimer = NewMockClaimer(ctrl)
	releases := make(chan struct{})
	s.maybeExpectBlockUntilLeadershipReleased(releases)

	s.claimLeaderErrors = []error{coreleadership.ErrClaimDenied}
	tracker := s.newTracker()

	// Check initial ticket fails.
	assertWaitLeader(c, tracker, false)

	// Get a new ticket and stop the tracker while it's pending.
	ticket := tracker.WaitLeader()
	workertest.CleanKill(c, tracker)

	// Check the ticket got closed without sending true.
	assertTicket(c, ticket, false)
	assertTicket(c, ticket, false)
}

func (s *TrackerSuite) TestWaitMinionAlreadyMinion(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.claimer = NewMockClaimer(ctrl)
	releases := make(chan struct{})
	s.maybeExpectBlockUntilLeadershipReleased(releases)

	s.claimLeaderErrors = []error{coreleadership.ErrClaimDenied}
	tracker := s.newTracker()

	// Check initial ticket is closed immediately.
	assertWaitLeader(c, tracker, false)

	workertest.CleanKill(c, tracker)
}

func (s *TrackerSuite) TestWaitMinionClaimerFails(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.claimer = NewMockClaimer(ctrl)
	releases := make(chan struct{})
	s.maybeExpectBlockUntilLeadershipReleased(releases)

	s.claimLeaderErrors = []error{coreleadership.ErrClaimDenied}
	s.blockUntilReleasedErrors = []error{errors.New("mein leben!")}

	tracker := s.newTrackerDirtyKill()
	s.unblockRelease(c, releases)

	err := workertest.CheckKilled(c, tracker)
	c.Assert(err, gc.ErrorMatches, "error while led-service/123 waiting for led-service leadership release: mein leben!")
}

func (s *TrackerSuite) TestWaitMinionBecomeMinion(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.claimer = NewMockClaimer(ctrl)
	releases := make(chan struct{})
	s.maybeExpectBlockUntilLeadershipReleased(releases)

	s.claimLeaderErrors = []error{nil, coreleadership.ErrClaimDenied}
	tracker := s.newTracker()

	// Check the first ticket stays open.
	assertWaitMinion(c, tracker, false)

	// Wait long enough for a single refresh, to trigger ErrClaimDenied; then
	// check the next ticket is closed.
	s.refreshes(1)
	assertWaitMinion(c, tracker, true)

	workertest.CleanKill(c, tracker)
}

func (s *TrackerSuite) TestWaitMinionNeverBecomeMinion(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.claimer = NewMockClaimer(ctrl)
	releases := make(chan struct{})
	s.maybeExpectBlockUntilLeadershipReleased(releases)

	tracker := s.newTracker()

	ticket := tracker.WaitMinion()
	s.refreshes(2)

	select {
	case <-ticket.Ready():
		c.Fatalf("got unexpected readiness: %v", ticket.Wait())
	default:
		// fallthrough
	}
}

func (s *TrackerSuite) finishLeadershipFunc(ctx context.Context, started, finish chan struct{}) error {
	select {
	case <-ctx.Done():
	case <-started:
	}

	select {
	case finish <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		return errors.New("trying to finish leadership func")
	}
	return nil
}

func (s *TrackerSuite) TestWithStableLeadership(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.claimer = NewMockClaimer(ctrl)
	releases := make(chan struct{})
	s.maybeExpectBlockUntilLeadershipReleased(releases)

	tracker := s.newTracker()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{})
	finishWithStableLeadership := make(chan struct{})
	go func(c *gc.C) {
		err := s.finishLeadershipFunc(ctx, started, finishWithStableLeadership)
		c.Assert(err, jc.ErrorIsNil)
	}(c)

	// Wait long enough for a single refresh.
	s.refreshes(1)

	called := false
	err := tracker.WithStableLeadership(ctx, func(ctx context.Context) error {
		close(started)
		called = true
		select {
		case <-finishWithStableLeadership:
		case <-ctx.Done():
		case <-time.After(coretesting.LongWait):
			return errors.New("trying to finish leadership func")
		}
		return ctx.Err()
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	workertest.CleanKill(c, tracker)
}

func (s *TrackerSuite) TestWithStableLeadershipLeadershipChanged(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.claimer = NewMockClaimer(ctrl)
	releases := make(chan struct{})
	s.maybeExpectBlockUntilLeadershipReleased(releases)

	s.claimLeaderErrors = []error{nil, coreleadership.ErrClaimDenied}
	tracker := s.newTracker()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	called := false
	waitErr := make(chan error, 1)
	go func() {
		err := tracker.WithStableLeadership(ctx, func(ctx context.Context) error {
			called = true
			select {
			case <-ctx.Done():
			case <-time.After(coretesting.LongWait):
				return errors.New("trying to finish leadership func")
			}
			return ctx.Err()
		})
		waitErr <- err
	}()

	// Wait long enough for a single refresh, to trigger ErrClaimDenied.
	s.refreshes(1)

	s.unblockRelease(c, releases)

	var err error
	select {
	case err = <-waitErr:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout waiting for leader func")
	}
	c.Assert(called, jc.IsTrue)
	c.Assert(err, jc.ErrorIs, coreleadership.ErrLeadershipChanged)
	workertest.CleanKill(c, tracker)
}

func (s *TrackerSuite) TestWithStableLeadershipFuncError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.claimer = NewMockClaimer(ctrl)
	releases := make(chan struct{})
	s.maybeExpectBlockUntilLeadershipReleased(releases)

	tracker := s.newTracker()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := tracker.WithStableLeadership(ctx, func(ctx context.Context) error {
		return errors.New("boom")
	})
	c.Assert(err, gc.ErrorMatches, "executing leadership func: boom")
	workertest.CleanKill(c, tracker)
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
