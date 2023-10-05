// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"errors"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	jtesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&MonitorSuite{})

type MonitorSuite struct {
	testing.IsolationSuite
	clock   *testclock.Clock
	closed  chan struct{}
	dead    chan struct{}
	broken  chan struct{}
	monitor *monitor
}

const testPingPeriod = 30 * time.Second
const testPingTimeout = time.Second

func (s *MonitorSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.clock = testclock.NewClock(time.Time{})
	s.closed = make(chan struct{})
	s.dead = make(chan struct{})
	s.broken = make(chan struct{})
	s.monitor = &monitor{
		clock:       s.clock,
		ping:        func(context.Context) error { return nil },
		pingPeriod:  testPingPeriod,
		pingTimeout: testPingTimeout,
		closed:      s.closed,
		dead:        s.dead,
		broken:      s.broken,
	}
}

func (s *MonitorSuite) TestClose(c *gc.C) {
	go s.monitor.run()
	s.waitForClock(c)
	close(s.closed)
	assertEvent(c, s.broken)
}

func (s *MonitorSuite) TestDead(c *gc.C) {
	go s.monitor.run()
	s.waitForClock(c)
	close(s.dead)
	assertEvent(c, s.broken)
}

func (s *MonitorSuite) TestFirstPingFails(c *gc.C) {
	s.monitor.ping = func(context.Context) error { return errors.New("boom") }
	go s.monitor.run()

	s.waitThenAdvance(c, testPingPeriod)
	assertEvent(c, s.broken)
}

func (s *MonitorSuite) TestLaterPingFails(c *gc.C) {
	pings := 0
	s.monitor.ping = func(context.Context) error {
		if pings > 0 {
			return errors.New("boom")
		}
		pings++
		return nil
	}
	go s.monitor.run()

	s.waitThenAdvance(c, testPingPeriod) // in run
	s.waitForClock(c)                    // in pingWithTimeout
	s.waitThenAdvance(c, testPingPeriod) // in run
	s.waitForClock(c)                    // in pingWithTimeout
	assertEvent(c, s.broken)
}

func (s *MonitorSuite) TestPingsTimesOut(c *gc.C) {
	s.monitor.ping = func(context.Context) error {
		// Advance the clock only once this ping call is being waited on.
		s.waitThenAdvance(c, testPingTimeout)
		return nil
	}
	go s.monitor.run()

	s.waitThenAdvance(c, testPingPeriod)
	assertEvent(c, s.broken)
}

func (s *MonitorSuite) waitForClock(c *gc.C) {
	assertEvent(c, s.clock.Alarms())
}

func (s *MonitorSuite) waitThenAdvance(c *gc.C, d time.Duration) {
	s.waitForClock(c)
	s.clock.Advance(d)
}

func assertEvent(c *gc.C, ch <-chan struct{}) {
	select {
	case <-ch:
	case <-time.After(jtesting.LongWait):
		c.Fatal("timed out waiting for channel event")
	}
}
