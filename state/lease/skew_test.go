// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/lease"
)

type SkewSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&SkewSuite{})

func (s *SkewSuite) TestZero(c *gc.C) {
	now := time.Now()

	// The zero Skew should act as unskewed.
	skew := lease.Skew{}

	c.Check(skew.Earliest(now), gc.Equals, now)
	c.Check(skew.Latest(now), gc.Equals, now)
}

func (s *SkewSuite) TestApparentPastWrite(c *gc.C) {
	now := time.Now()
	c.Logf("now: %s", now)
	oneSecondAgo := now.Add(-time.Second)
	threeSecondsAgo := now.Add(-3 * time.Second)
	nineSecondsAgo := now.Add(-9 * time.Second)
	sixSecondsLater := now.Add(6 * time.Second)
	eightSecondsLater := now.Add(8 * time.Second)

	// Where T is the current local time:
	// between T-3 and T-1, we read T-9 from the remote clock.
	skew := lease.Skew{
		LastWrite:  nineSecondsAgo,
		ReadAfter:  threeSecondsAgo,
		ReadBefore: oneSecondAgo,
	}

	// If the remote wrote a long time ago -- say, 20 minutes ago it thought it
	// was 9 seconds ago -- its clock could be arbitrarily far ahead of ours.
	// But we know that when we started reading, 3 seconds ago, it might not
	// have seen a time later than 9 seconds ago; so right now, three seconds
	// after that, it might not have seen a time later than 6 seconds ago.
	c.Check(skew.Earliest(now), gc.DeepEquals, sixSecondsLater)

	// If the remote wrote at the very last moment -- exactly one second ago,
	// it thought it was nine seconds ago -- it could have a clock a full 8
	// seconds behind ours. If so, the *latest* time at which it *might* still
	// think it's before now is 8 seconds in the future.
	c.Check(skew.Latest(now), gc.DeepEquals, eightSecondsLater)
}

func (s *SkewSuite) TestApparentFutureWrite(c *gc.C) {
	now := time.Now()
	c.Logf("now: %s", now)
	oneSecondAgo := now.Add(-time.Second)
	threeSecondsAgo := now.Add(-3 * time.Second)
	tenSecondsAgo := now.Add(-10 * time.Second)
	twelveSecondsAgo := now.Add(-12 * time.Second)
	nineSecondsLater := now.Add(9 * time.Second)

	// Where T is the current local time:
	// between T-3 and T-1, we read T+9 from the remote clock.
	skew := lease.Skew{
		LastWrite:  nineSecondsLater,
		ReadAfter:  threeSecondsAgo,
		ReadBefore: oneSecondAgo,
	}

	// If the remote wrote a long time ago -- say, 20 minutes ago it thought
	// it was nine seconds after now -- its clock could be arbitrarily far
	// ahead of ours. But we know that when we started reading, 3 seconds ago,
	// it might not have seen a time later than 9 seconds in the future; so
	// right now, three seconds after that, it might not have seen a time later
	// than twelve seconds in the future.
	c.Check(skew.Earliest(now), gc.DeepEquals, twelveSecondsAgo)

	// If the remote wrote at the very last moment -- exactly one second ago,
	// it thought it was 9 seconds in the future -- it could have a clock a
	// full 10 seconds ahead of ours. If so, the *latest* time at which it
	// might still have thought it was before now is ten seconds in the past.
	c.Check(skew.Latest(now), gc.DeepEquals, tenSecondsAgo)
}

func (s *SkewSuite) TestBracketedWrite(c *gc.C) {
	now := time.Now()
	c.Logf("now: %s", now)
	oneSecondAgo := now.Add(-time.Second)
	twoSecondsAgo := now.Add(-2 * time.Second)
	threeSecondsAgo := now.Add(-3 * time.Second)
	fiveSecondsAgo := now.Add(-5 * time.Second)
	oneSecondLater := now.Add(time.Second)

	// Where T is the current local time:
	// between T-5 and T-1, we read T-2 from the remote clock.
	skew := lease.Skew{
		LastWrite:  twoSecondsAgo,
		ReadAfter:  fiveSecondsAgo,
		ReadBefore: oneSecondAgo,
	}

	// If the remote wrote a long time ago -- say, 20 minutes ago it thought
	// it was two seconds before now -- its clock could be arbitrarily far
	// ahead of ours. But we know that when we started reading, 5 seconds ago,
	// it might not have seen a time later than 2 seconds in the past; so
	// right now, five seconds after that, it might not have seen a time later
	// than three seconds in the future.
	c.Check(skew.Earliest(now), gc.DeepEquals, threeSecondsAgo)

	// If the remote wrote at the very last moment -- exactly one second ago,
	// it thought it was 2 seconds in the past -- it could have a clock one
	// second behind ours. If so, the *latest* time at which it might still
	// have thought it was before now is one second in the future.
	c.Check(skew.Latest(now), gc.DeepEquals, oneSecondLater)
}

func (s *SkewSuite) TestMixedTimezones(c *gc.C) {
	here := time.FixedZone("here", -3600)
	there := time.FixedZone("there", -7200)
	elsewhere := time.FixedZone("elsewhere", -10800)

	// This is a straight copy of TestBracketedWrite, with strange timezones
	// inserted to check that they don't affect the results at all.
	now := time.Now()
	c.Logf("now: %s", now)
	oneSecondAgo := now.Add(-time.Second)
	twoSecondsAgo := now.Add(-2 * time.Second)
	threeSecondsAgo := now.Add(-3 * time.Second)
	fiveSecondsAgo := now.Add(-5 * time.Second)
	oneSecondLater := now.Add(time.Second)

	// Where T is the current local time:
	// between T-5 and T-1, we read T-2 from the remote clock.
	skew := lease.Skew{
		LastWrite:  twoSecondsAgo.In(here),
		ReadAfter:  fiveSecondsAgo.In(there),
		ReadBefore: oneSecondAgo.In(elsewhere),
	}

	// If the remote wrote a long time ago -- say, 20 minutes ago it thought
	// it was two seconds before now -- its clock could be arbitrarily far
	// ahead of ours. But we know that when we started reading, 5 seconds ago,
	// it might not have seen a time later than 2 seconds in the past; so
	// right now, five seconds after that, it might not have seen a time later
	// than three seconds in the future.
	c.Check(skew.Earliest(now), gc.DeepEquals, threeSecondsAgo.In(there))

	// If the remote wrote at the very last moment -- exactly one second ago,
	// it thought it was 2 seconds in the past -- it could have a clock one
	// second behind ours. If so, the *latest* time at which it might still
	// have thought it was before now is one second in the future.
	c.Check(skew.Latest(now), gc.DeepEquals, oneSecondLater.In(elsewhere))
}
