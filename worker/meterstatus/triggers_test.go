// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	"time"

	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/meterstatus"
)

type TriggersSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&TriggersSuite{})

var fudge = time.Second

const (
	testAmberGracePeriod = time.Minute * 10
	testRedGracePeriod   = time.Minute * 30
)

func (*TriggersSuite) TestTriggerCreation(c *gc.C) {
	now := time.Now()
	tests := []struct {
		description  string
		worker       meterstatus.WorkerState
		status       string
		disconnected time.Time
		now          clock.Clock
		check        func(*gc.C, <-chan time.Time, <-chan time.Time)
	}{{
		"normal start, unit status is green",
		meterstatus.Uninitialized,
		"GREEN",
		now,
		coretesting.NewClock(now),
		func(c *gc.C, amber, red <-chan time.Time) {
			c.Check(amber, gc.NotNil)
			c.Check(red, gc.NotNil)
		}}, {
		"normal start, unit status is amber",
		meterstatus.Uninitialized,
		"AMBER",
		now,
		coretesting.NewClock(now),
		func(c *gc.C, amber, red <-chan time.Time) {
			c.Check(amber, gc.NotNil)
			c.Check(red, gc.NotNil)
		}}, {
		"normal start, unit status is RED",
		meterstatus.Uninitialized,
		"RED",
		now,
		coretesting.NewClock(now),
		func(c *gc.C, amber, red <-chan time.Time) {
			c.Check(amber, gc.IsNil)
			c.Check(red, gc.NotNil)
		}}, {
		"restart, unit status is green, amber status not yet triggered",
		meterstatus.WaitingAmber,
		"GREEN",
		now,
		coretesting.NewClock(now),
		func(c *gc.C, amber, red <-chan time.Time) {
			c.Check(amber, gc.NotNil)
			c.Check(red, gc.NotNil)
		}}, {
		"restart, unit status is amber, amber status not yet triggered",
		meterstatus.WaitingAmber,
		"AMBER",
		now,
		coretesting.NewClock(now),
		func(c *gc.C, amber, red <-chan time.Time) {
			c.Check(amber, gc.NotNil)
			c.Check(red, gc.NotNil)
		}}, {
		"restart, unit status is red, amber status not yet triggered",
		meterstatus.WaitingAmber,
		"RED",
		now,
		coretesting.NewClock(now),
		func(c *gc.C, amber, red <-chan time.Time) {
			c.Check(amber, gc.IsNil)
			c.Check(red, gc.NotNil)
		}}, {
		"restart, unit status is green, amber status trigger time passed",
		meterstatus.WaitingAmber,
		"GREEN",
		now.Add(-(testAmberGracePeriod + fudge)),
		coretesting.NewClock(now),
		func(c *gc.C, amber, red <-chan time.Time) {
			c.Check(amber, gc.NotNil)
			c.Check(red, gc.NotNil)
		}}, {
		"restart, unit status is amber, amber status trigger time passed",
		meterstatus.WaitingAmber,
		"AMBER",
		now.Add(-(testAmberGracePeriod + fudge)),
		coretesting.NewClock(now),
		func(c *gc.C, amber, red <-chan time.Time) {
			c.Check(amber, gc.NotNil)
			c.Check(red, gc.NotNil)
		}}, {
		"restart, unit status is red, amber status trigger time passed",
		meterstatus.WaitingAmber,
		"RED",
		now.Add(-(testAmberGracePeriod + fudge)),
		coretesting.NewClock(now),
		func(c *gc.C, amber, red <-chan time.Time) {
			c.Check(amber, gc.IsNil)
			c.Check(red, gc.NotNil)
		}}, {
		"restart, unit status is amber, amber status has been triggered",
		meterstatus.WaitingRed,
		"AMBER",
		now.Add(-(testAmberGracePeriod + fudge)),
		coretesting.NewClock(now),
		func(c *gc.C, amber, red <-chan time.Time) {
			c.Check(amber, gc.IsNil)
			c.Check(red, gc.NotNil)
		}}, {
		"restart, unit status is amber, red status trigger time has passed",
		meterstatus.WaitingRed,
		"AMBER",
		now.Add(-(testRedGracePeriod + fudge)),
		coretesting.NewClock(now),
		func(c *gc.C, amber, red <-chan time.Time) {
			c.Check(amber, gc.IsNil)
			c.Check(red, gc.NotNil)
		}}, {
		"restart, unit status is red, red status trigger time has passed",
		meterstatus.WaitingRed,
		"RED",
		now.Add(-(testRedGracePeriod + fudge)),
		coretesting.NewClock(now),
		func(c *gc.C, amber, red <-chan time.Time) {
			c.Check(amber, gc.IsNil)
			c.Check(red, gc.NotNil)
		}}, {
		"restart, unit status is red, red status has been triggered",
		meterstatus.Done,
		"RED",
		now.Add(-(testRedGracePeriod + fudge)),
		coretesting.NewClock(now),
		func(c *gc.C, amber, red <-chan time.Time) {
			c.Check(amber, gc.IsNil)
			c.Check(red, gc.IsNil)
		}}}

	for i, test := range tests {
		c.Logf("%d: %s", i, test.description)
		signalAmber, signalRed := meterstatus.GetTriggers(test.worker, test.status, test.disconnected, test.now, testAmberGracePeriod, testRedGracePeriod)
		test.check(c, signalAmber, signalRed)
	}
}
