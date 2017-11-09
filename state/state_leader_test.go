// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type StateLeadershipSuite struct {
	ConnSuite
}

var _ = gc.Suite(&StateLeadershipSuite{})

func (s *StateLeadershipSuite) TestHackLeadershipUnblocksClaimer(c *gc.C) {
	claimer := s.State.LeadershipClaimer()
	err := claimer.ClaimLeadership("blah", "blah/0", time.Minute)
	c.Assert(err, jc.ErrorIsNil)

	unblocked := make(chan struct{})
	go func() {
		err := claimer.BlockUntilLeadershipReleased("blah")
		close(unblocked)
		c.Check(err, gc.ErrorMatches, "leadership manager stopped")
	}()

	s.State.HackLeadership()
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out while waiting for unblock")
	case <-unblocked:
	}
}

func (s *StateLeadershipSuite) TestLeadershipClaimerRestarts(c *gc.C) {
	claimer := s.State.LeadershipClaimer()
	err := claimer.ClaimLeadership("blah", "blah/0", time.Minute)
	c.Assert(err, jc.ErrorIsNil)

	state.RestartLeadershipManager(s.State)

	err = claimer.ClaimLeadership("blah", "blah/0", time.Minute)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StateLeadershipSuite) TestLeadershipCheckerRestarts(c *gc.C) {
	claimer := s.State.LeadershipClaimer()
	checker := s.State.LeadershipChecker()
	err := claimer.ClaimLeadership("application", "application/0", time.Minute)
	c.Assert(err, jc.ErrorIsNil)

	state.RestartLeadershipManager(s.State)

	token := checker.LeadershipCheck("application", "application/0")
	err = token.Check(nil)
	c.Assert(err, jc.ErrorIsNil)
}
