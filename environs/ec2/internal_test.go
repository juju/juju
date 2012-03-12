package ec2

import (
	. "launchpad.net/gocheck"
	"time"
)

type internalSuite struct{}

var _ = Suite(internalSuite{})

func (internalSuite) TestAttemptTiming(c *C) {
	const delta = 0.01e9
	testAttempt := attemptStrategy{
		total: 0.25e9,
		delay: 0.1e9,
	}
	want := []time.Duration{0, 0.1e9, 0.2e9, 0.2e9}
	got := make([]time.Duration, 0, len(want)) // avoid allocation when testing timing
	t0 := time.Now()
	for a := testAttempt.start(); a.next(); {
		got = append(got, time.Now().Sub(t0))
	}
	got = append(got, time.Now().Sub(t0))
	c.Assert(got, HasLen, len(want))
	for i, got := range want {
		lo := want[i] - delta
		hi := want[i] + delta
		if got < lo || got > hi {
			c.Errorf("attempt %d want %g got %g", i, want[i].Seconds(), got.Seconds())
		}
	}
}
