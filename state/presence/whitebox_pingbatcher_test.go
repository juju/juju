// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

// WhiteboxPingBatcherSuite tests pieces of PingBatcher that need direct inspection but don't need database access.
type WhiteboxPingBatcherSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&WhiteboxPingBatcherSuite{})

func checkSleepRange(c *gc.C, interval, minTime, maxTime time.Duration) {
	pingBatcher := NewPingBatcher(nil, interval)
	defer pingBatcher.Stop()
	var lastTime time.Duration
	var measuredMinTime time.Duration
	var measuredMaxTime time.Duration

	for i := 0; i < 1000; i++ {
		next := pingBatcher.nextSleep()
		// We use Assert rather than Check because we don't want 100s of failures
		c.Assert(next, jc.GreaterThan, minTime)
		c.Assert(next, jc.LessThan, maxTime)
		if lastTime == 0 {
			measuredMinTime = next
			measuredMaxTime = next
		} else {
			// We are using a range in 100s of milliseconds at a
			// resolution of nanoseconds. The chance of getting the
			// same random value 2x in a row is sufficiently low that
			// we can just assert the value is changing.
			// (Chance of collision is roughly 1 in 100 Million)
			c.Assert(next, gc.Not(gc.Equals), lastTime)
			if next < measuredMinTime {
				measuredMinTime = next
			}
			if next > measuredMaxTime {
				measuredMaxTime = next
			}
		}
		lastTime = next
	}
	// Check that we're actually using the full range that was requested.
	// Assert that after 1000 tries we've used a good portion of the range
	// If we sampled perfectly, then we would have fully sampled the range,
	// spread very 1/1000 of the range.
	// If we set the required range to 1/100, then a given sample would fail 99%
	// of the time, 1000 samples would fail 0.99^1000=4e-5 or ~1-in-20,000 times.
	// (actual measurements showed 18 in 20,000, probably due to double ended range vs single ended)
	// However, at 1/10 its 0.9^1000=1.7e-46, or 10^41 times less likely to fail.
	// In 100,000 runs, a range of 1/10 never failed
	expectedCloseness := (maxTime - minTime) / 10
	c.Check(measuredMinTime, jc.LessThan, minTime+expectedCloseness)
	c.Check(measuredMaxTime, jc.GreaterThan, maxTime-expectedCloseness)
}

func (s *WhiteboxPingBatcherSuite) TestNextSleep(c *gc.C) {
	// nextSleep should select a random range of time to sleep before the
	// next flush will be called, however it should be within a valid range
	// of times
	// range is [800ms, 1200ms] inclusive, but we only can easily assert exclusive
	checkSleepRange(c, 1*time.Second, 799*time.Millisecond, 1201*time.Millisecond)
	checkSleepRange(c, 2*time.Second, 1599*time.Millisecond, 2401*time.Millisecond)
}
