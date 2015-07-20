// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"time"

	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter"
)

type TimerSuite struct{}

var _ = gc.Suite(&TimerSuite{})

func (s *TimerSuite) TestCollectMetricsTimer(c *gc.C) {
	s.testTimer(c, uniter.ActiveCollectMetricsSignal)
}

func (s *TimerSuite) TestUpdateStatusTimer(c *gc.C) {
	s.testTimer(c, uniter.UpdateStatusSignal)
}

func (*TimerSuite) testTimer(c *gc.C, s uniter.TimedSignal) {
	now := time.Now()
	defaultInterval := coretesting.ShortWait / 5
	testCases := []struct {
		about        string
		now          time.Time
		lastRun      time.Time
		interval     time.Duration
		expectSignal bool
	}{{
		"Timer firing after delay.",
		now,
		now.Add(-defaultInterval / 2),
		defaultInterval,
		true,
	}, {
		"Timer firing the first time.",
		now,
		time.Unix(0, 0),
		defaultInterval,
		true,
	}, {
		"Timer not firing soon.",
		now,
		now,
		coretesting.ShortWait * 2,
		false,
	}}

	for i, t := range testCases {
		c.Logf("running test %d", i)
		sig := s(t.now, t.lastRun, t.interval)
		select {
		case <-sig:
			if !t.expectSignal {
				c.Errorf("not expecting a signal")
			}
		case <-time.After(coretesting.ShortWait):
			if t.expectSignal {
				c.Errorf("expected a signal")
			}
		}
	}
}
