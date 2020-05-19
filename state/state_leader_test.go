// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"io/ioutil"
	"time" // Only used for time types.

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/globalclock"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	coretesting "github.com/juju/juju/testing"
)

type LeadershipSuite struct {
	ConnSuite
	checker     leadership.Checker
	claimer     leadership.Claimer
	globalClock globalclock.Updater
}

var _ = gc.Suite(&LeadershipSuite{})

func (s *LeadershipSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	err := s.State.SetClockForTesting(s.Clock)
	c.Assert(err, jc.ErrorIsNil)
	s.checker = s.State.LeadershipChecker()
	s.claimer = s.State.LeadershipClaimer()
	s.globalClock, err = s.State.GlobalClockUpdater()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LeadershipSuite) TestClaimValidatesApplicationname(c *gc.C) {
	err := s.claimer.ClaimLeadership("not/a/application", "u/0", time.Minute)
	c.Check(err, gc.ErrorMatches, `cannot claim lease "not/a/application": not an application name`)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *LeadershipSuite) TestClaimValidatesUnitName(c *gc.C) {
	err := s.claimer.ClaimLeadership("application", "not/a/unit", time.Minute)
	c.Check(err, gc.ErrorMatches, `cannot claim lease for holder "not/a/unit": not a unit name`)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *LeadershipSuite) TestClaimValidateDuration(c *gc.C) {
	err := s.claimer.ClaimLeadership("application", "u/0", 0)
	c.Check(err, gc.ErrorMatches, `cannot claim lease for 0s?: non-positive`)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *LeadershipSuite) TestCheckValidatesApplicationname(c *gc.C) {
	token := s.checker.LeadershipCheck("not/a/application", "u/0")
	err := token.Check(0, nil)
	c.Check(err, gc.ErrorMatches, `cannot check lease "not/a/application": not an application name`)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *LeadershipSuite) TestCheckValidatesUnitName(c *gc.C) {
	token := s.checker.LeadershipCheck("application", "not/a/unit")
	err := token.Check(0, nil)
	c.Check(err, gc.ErrorMatches, `cannot check holder "not/a/unit": not a unit name`)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *LeadershipSuite) TestBlockValidatesApplicationname(c *gc.C) {
	err := s.claimer.BlockUntilLeadershipReleased("not/an/application", nil)
	c.Check(err, gc.ErrorMatches, `cannot wait for lease "not/an/application" expiry: not an application name`)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *LeadershipSuite) TestClaimExpire(c *gc.C) {

	// Claim on behalf of one unit.
	err := s.claimer.ClaimLeadership("application", "application/0", time.Minute)
	c.Assert(err, jc.ErrorIsNil)

	// Claim on behalf of another.
	err = s.claimer.ClaimLeadership("application", "application/1", time.Minute)
	c.Check(err, gc.Equals, leadership.ErrClaimDenied)

	// Allow the first claim to expire.
	s.expire(c, "application")

	// Reclaim on behalf of another.
	err = s.claimer.ClaimLeadership("application", "application/1", time.Minute)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LeadershipSuite) TestCheck(c *gc.C) {

	// Create a single token for use by the whole test.
	token := s.checker.LeadershipCheck("application", "application/0")

	// Claim leadership.
	err := s.claimer.ClaimLeadership("application", "application/0", time.Minute)
	c.Assert(err, jc.ErrorIsNil)

	// Check token reports current leadership state.
	var ops []txn.Op
	err = token.Check(0, &ops)
	c.Check(err, jc.ErrorIsNil)
	c.Check(ops, gc.HasLen, 1)

	// Allow leadership to expire.
	s.expire(c, "application")

	// Check leadership still reported accurately.
	var ops2 []txn.Op
	err = token.Check(1, &ops2)
	c.Check(err, gc.ErrorMatches, `"application/0" is not leader of "application"`)
	c.Check(err, jc.Satisfies, leadership.IsNotLeaderError)
	c.Check(ops2, gc.IsNil)
}

func (s *LeadershipSuite) TestCloseStateUnblocksClaimer(c *gc.C) {
	err := s.claimer.ClaimLeadership("blah", "blah/0", time.Minute)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.Close()
	c.Assert(err, jc.ErrorIsNil)

	select {
	case err := <-s.expiryChan("blah", nil):
		c.Check(err, gc.ErrorMatches, "lease manager stopped")
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out while waiting for unblock")
	}
}

func (s *LeadershipSuite) TestLeadershipClaimerRestarts(c *gc.C) {
	// SetClockForTesting will restart the workers, and
	// will have replaced them by the time it returns.
	s.State.SetClockForTesting(testclock.NewClock(time.Time{}))

	err := s.claimer.ClaimLeadership("blah", "blah/0", time.Minute)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LeadershipSuite) TestLeadershipCheckerRestarts(c *gc.C) {
	err := s.claimer.ClaimLeadership("application", "application/0", time.Minute)
	c.Assert(err, jc.ErrorIsNil)

	// SetClockForTesting will restart the workers, and
	// will have replaced them by the time it returns.
	s.State.SetClockForTesting(testclock.NewClock(time.Time{}))

	token := s.checker.LeadershipCheck("application", "application/0")
	err = token.Check(0, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LeadershipSuite) TestBlockUntilLeadershipReleasedCancel(c *gc.C) {
	err := s.claimer.ClaimLeadership("blah", "blah/0", time.Minute)
	c.Assert(err, jc.ErrorIsNil)

	cancel := make(chan struct{})
	close(cancel)

	select {
	case err := <-s.expiryChan("blah", cancel):
		c.Check(err, gc.Equals, leadership.ErrBlockCancelled)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out while waiting for unblock")
	}
}

func (s *LeadershipSuite) TestApplicationLeaders(c *gc.C) {
	target := s.State.LeaseNotifyTarget(ioutil.Discard, loggo.GetLogger("leadership_test"))
	target.Claimed(lease.Key{"application-leadership", s.State.ModelUUID(), "blah"}, "blah/0")
	target.Claimed(lease.Key{"application-leadership", s.State.ModelUUID(), "application"}, "application/1")
	leaders, err := s.State.ApplicationLeaders()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leaders, jc.DeepEquals, map[string]string{
		"application": "application/1",
		"blah":        "blah/0",
	})
}

func (s *LeadershipSuite) expire(c *gc.C, applicationname string) {
	err := s.globalClock.Advance(time.Hour, nil)
	c.Assert(err, jc.ErrorIsNil)

	// The lease manager starts a new timer each time it
	// is waiting for something to do, so we can't know
	// how many clients are waiting on the clock without
	// being tying ourselves too closely to the implementation.
	// Instead, advance the clock by an hour and then unblock
	// all timers until the lease is expired.
	s.Clock.Advance(time.Hour)

	unblocked := s.expiryChan(applicationname, nil)
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("never unblocked")
		case err := <-unblocked:
			c.Assert(err, jc.ErrorIsNil)
			return
		case <-s.Clock.Alarms():
		}
	}
}

func (s *LeadershipSuite) expiryChan(applicationname string, cancel <-chan struct{}) <-chan error {
	expired := make(chan error, 1)
	go func() {
		expired <- s.claimer.BlockUntilLeadershipReleased(applicationname, cancel)
	}()
	return expired
}
