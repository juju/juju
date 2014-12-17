// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"time"

	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter"
)

type CollectMetricsTimerSuite struct{}

var _ = gc.Suite(&CollectMetricsTimerSuite{})

func (*CollectMetricsTimerSuite) TestTimer(c *gc.C) {
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
		sig := (*uniter.ActiveMetricsTimer)(t.now, t.lastRun, t.interval)
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
