// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"context"
	"errors"
	stdtesting "testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	jtesting "github.com/juju/juju/internal/testing"
)

func TestMonitorSuite(t *stdtesting.T) {
	tc.Run(t, &MonitorSuite{})
}

type MonitorSuite struct {
	testhelpers.IsolationSuite
	clock   *testclock.Clock
	closed  chan struct{}
	dead    chan struct{}
	broken  chan struct{}
	monitor *monitor
}

const testPingPeriod = 30 * time.Second
const testPingTimeout = time.Second

func (s *MonitorSuite) SetUpTest(c *tc.C) {
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

func (s *MonitorSuite) TestClose(c *tc.C) {
	go s.monitor.run()
	s.waitForClock(c)
	close(s.closed)
	assertEvent(c, s.broken)
}

func (s *MonitorSuite) TestDead(c *tc.C) {
	go s.monitor.run()
	s.waitForClock(c)
	close(s.dead)
	assertEvent(c, s.broken)
}

func (s *MonitorSuite) TestFirstPingFails(c *tc.C) {
	s.monitor.ping = func(context.Context) error { return errors.New("boom") }
	go s.monitor.run()

	s.waitThenAdvance(c, testPingPeriod)
	assertEvent(c, s.broken)
}

func (s *MonitorSuite) TestLaterPingFails(c *tc.C) {
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

func (s *MonitorSuite) TestPingsTimesOut(c *tc.C) {
	s.monitor.ping = func(context.Context) error {
		// Advance the clock only once this ping call is being waited on.
		s.waitThenAdvance(c, testPingTimeout)
		return nil
	}
	go s.monitor.run()

	s.waitThenAdvance(c, testPingPeriod)
	assertEvent(c, s.broken)
}

func (s *MonitorSuite) waitForClock(c *tc.C) {
	assertEvent(c, s.clock.Alarms())
}

func (s *MonitorSuite) waitThenAdvance(c *tc.C, d time.Duration) {
	s.waitForClock(c)
	s.clock.Advance(d)
}

func assertEvent(c *tc.C, ch <-chan struct{}) {
	select {
	case <-ch:
	case <-time.After(jtesting.LongWait):
		c.Fatal("timed out waiting for channel event")
	}
}
