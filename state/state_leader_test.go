// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
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
