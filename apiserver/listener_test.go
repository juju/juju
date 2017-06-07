// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type listenerSuite struct {
	testing.IsolationSuite

	listener *mockListener

	minPause time.Duration
	maxPause time.Duration
	clock    *testing.Clock
}

var _ = gc.Suite(&listenerSuite{})

func (s *listenerSuite) SetUpTest(c *gc.C) {
	s.minPause = 10 * time.Millisecond
	s.maxPause = 5 * time.Second
	s.clock = testing.NewClock(time.Now())
	s.listener = &mockListener{}
}

func (s *listenerSuite) testListener() *throttlingListener {
	return newThrottlingListener(s.listener, s.minPause, s.maxPause, s.clock).(*throttlingListener)
}

func (s *listenerSuite) TestConnRateNoConnections(c *gc.C) {
	l := s.testListener()
	c.Assert(l.connRateMetric(), gc.Equals, 0)
}

func (s *listenerSuite) TestConnRateOneConnection(c *gc.C) {
	l := s.testListener()
	s.listener.Accept()
	c.Assert(l.connRateMetric(), gc.Equals, 0)
}

func (s *listenerSuite) TestConnRateBufferUnder(c *gc.C) {
	l := s.testListener()
	l.maxPause = 0
	for i := 0; i < 100; i++ {
		l.Accept()
		s.clock.Advance(time.Millisecond)
	}
	c.Assert(s.listener.count, gc.Equals, 100)
	c.Assert(l.connRateMetric(), gc.Equals, 10)
}

func (s *listenerSuite) TestConnRateBufferOver(c *gc.C) {
	l := s.testListener()
	l.maxPause = 0
	for i := 0; i < 550; i++ {
		l.Accept()
		s.clock.Advance(2 * time.Millisecond)
	}
	c.Assert(s.listener.count, gc.Equals, 550)
	c.Assert(l.connRateMetric(), gc.Equals, 5)
}

func (s *listenerSuite) TestConnRateNonContiguous(c *gc.C) {
	l := s.testListener()
	l.maxPause = 0
	for i := 0; i < 550; i++ {
		l.Accept()
		if i == 200 {
			s.clock.Advance(100 * time.Millisecond)
		}
		s.clock.Advance(2 * time.Millisecond)
	}
	c.Assert(s.listener.count, gc.Equals, 550)
	c.Assert(l.connRateMetric(), gc.Equals, 4)
}

func (s *listenerSuite) setupConnRate(l *throttlingListener, rate int) {
	max := l.maxPause
	l.maxPause = 0
	for i := 0; i < rate; i++ {
		l.Accept()
		s.clock.Advance(10 * time.Millisecond / time.Duration(rate-1))
	}
	l.maxPause = max
}

func (s *listenerSuite) TestPauseTimeMin(c *gc.C) {
	l := s.testListener()
	for i := 0; i < 100; i++ {
		c.Assert(l.pauseTime(), gc.Equals, s.minPause)
	}
}

func (s *listenerSuite) TestPauseTimeMax(c *gc.C) {
	l := s.testListener()
	s.setupConnRate(l, 1+int(l.maxPause-l.minPause)/int(5*time.Millisecond))
	for i := 0; i < 100; i++ {
		c.Assert(l.pauseTime(), gc.Equals, s.maxPause)
	}
}

func (s *listenerSuite) TestPauseTimeInBetween(c *gc.C) {
	l := s.testListener()
	s.setupConnRate(l, 1+int(l.maxPause-l.minPause)/int(20*time.Millisecond))
	for i := 0; i < 100; i++ {
		pauseTime := l.pauseTime()
		c.Assert(pauseTime, jc.LessThan, s.minPause+(s.maxPause-s.minPause+1)/2)
		c.Assert(pauseTime, jc.GreaterThan, l.minPause)
	}
}

func (s *listenerSuite) TestPause(c *gc.C) {
	l := s.testListener()
	start := make(chan bool, 0)
	done := make(chan bool, 1)
	go func() {
		<-start
		l.Accept()
		done <- true
	}()

	start <- true
	s.clock.Advance((l.minPause/time.Millisecond - 1) * time.Millisecond)
	select {
	case <-done:
		c.Fatal("pause returned too soon")
	case <-time.After(50 * time.Millisecond):
	}
	c.Assert(s.listener.count, gc.Equals, 0)
	s.clock.Advance(time.Millisecond)
	select {
	case <-done:
	case <-time.After(10 * time.Millisecond):
		c.Fatal("pause failed")
	}
	c.Assert(s.listener.count, gc.Equals, 1)
}

type mockListener struct {
	net.Listener
	count int
}

func (m *mockListener) Accept() (net.Conn, error) {
	m.count++
	return nil, nil
}
