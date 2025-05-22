// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/worker/uniter"
)

type timerSuite struct{}

func TestTimerSuite(t *testing.T) {
	tc.Run(t, &timerSuite{})
}

func (s *timerSuite) TestTimer(c *tc.C) {
	nominal := 100 * time.Second
	minTime := 80*time.Second - time.Millisecond
	maxTime := 120*time.Second + time.Millisecond

	timer := uniter.NewUpdateStatusTimer()
	var lastTime time.Duration
	var measuredMinTime time.Duration
	var measuredMaxTime time.Duration

	for i := 0; i < 1000; i++ {
		wait := timer(nominal)
		waitDuration := time.Duration(reflect.ValueOf(wait).Int())
		// We use Assert rather than Check because we don't want 100s of failures
		c.Assert(wait, tc.GreaterThan, minTime)
		c.Assert(wait, tc.LessThan, maxTime)
		if lastTime == 0 {
			measuredMinTime = waitDuration
			measuredMaxTime = waitDuration
		} else {
			// We are using a range in 100s of milliseconds at a
			// resolution of nanoseconds. The chance of getting the
			// same random value 2x in a row is sufficiently low that
			// we can just assert the value is changing.
			// (Chance of collision is roughly 1 in 100 Million)
			c.Assert(wait, tc.Not(tc.Equals), lastTime)
			if waitDuration < measuredMinTime {
				measuredMinTime = waitDuration
			}
			if waitDuration > measuredMaxTime {
				measuredMaxTime = waitDuration
			}
		}
		lastTime = waitDuration
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
	c.Check(measuredMinTime, tc.LessThan, minTime+expectedCloseness)
	c.Check(measuredMaxTime, tc.GreaterThan, maxTime-expectedCloseness)
}
