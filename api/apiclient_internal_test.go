// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"time"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	jjtesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&MonitorSuite{})

type MonitorSuite struct {
	testing.IsolationSuite
	clock   *testing.Clock
	closed  chan (struct{})
	broken  chan (struct{})
	monitor *monitor
}

func (s *MonitorSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.clock = testing.NewClock(time.Time{})
	s.closed = make(chan struct{})
	s.broken = make(chan struct{})
	s.monitor = &monitor{
		clock:  s.clock,
		ping:   func() error { return nil },
		closed: s.closed,
		broken: s.broken,
	}
}

func (s *MonitorSuite) TestFirstPingFails(c *gc.C) {
	s.monitor.ping = func() error { return errors.New("boom") }
	go s.monitor.run()

	assertEvent(c, s.clock.Alarms())
	s.clock.Advance(PingPeriod)
	assertEvent(c, s.broken)
}

func (s *MonitorSuite) TestLaterPingFails(c *gc.C) {
	pings := 0
	s.monitor.ping = func() error {
		if pings > 0 {
			return errors.New("boom")
		}
		pings++
		return nil
	}
	go s.monitor.run()

	assertEvent(c, s.clock.Alarms())
	s.clock.Advance(PingPeriod)
	assertEvent(c, s.clock.Alarms())
	s.clock.Advance(PingPeriod)
	assertEvent(c, s.broken)
}

func (s *MonitorSuite) TestPingsTimesOut(c *gc.C) {
	s.monitor.ping = func() error {
		// Advance the clock only once this ping call is being waited on.
		s.clock.WaitAdvance(PingTimeout, jjtesting.LongWait, 1)
		return nil
	}
	go s.monitor.run()

	assertEvent(c, s.clock.Alarms())
	s.clock.Advance(PingPeriod)
	assertEvent(c, s.broken)
}

func (s *MonitorSuite) TestClose(c *gc.C) {
	go s.monitor.run()
	assertEvent(c, s.clock.Alarms())
	close(s.closed)
	assertEvent(c, s.broken)
}

func assertEvent(c *gc.C, ch <-chan struct{}) {
	select {
	case <-ch:
	case <-time.After(jjtesting.LongWait):
		c.Fatal("timed out waiting for channel event")
	}
}
