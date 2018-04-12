// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"math"
	"net"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

type listenerSuite struct {
	testing.IsolationSuite

	listener *mockListener

	minPause       time.Duration
	maxPause       time.Duration
	lowerThreshold int
	upperThreshold int
	clock          *testing.Clock
}

var _ = gc.Suite(&listenerSuite{})

func (s *listenerSuite) SetUpTest(c *gc.C) {
	s.minPause = 10 * time.Millisecond
	s.maxPause = 5 * time.Second
	s.lowerThreshold = 1000
	s.upperThreshold = 10000

	s.clock = testing.NewClock(time.Now())
	s.listener = &mockListener{}
}

func (s *listenerSuite) testListener() *throttlingListener {
	cfg := DefaultRateLimitConfig()
	cfg.ConnMinPause = s.minPause
	cfg.ConnMaxPause = s.maxPause
	cfg.ConnLowerThreshold = s.lowerThreshold
	cfg.ConnUpperThreshold = s.upperThreshold
	return newThrottlingListener(s.listener, cfg, s.clock).(*throttlingListener)
}

func (s *listenerSuite) roundRate(rate int) int {
	return (rate / 100) * 100
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
	c.Assert(s.roundRate(l.connRateMetric()), gc.Equals, 1000)
}

func (s *listenerSuite) TestConnRateBufferOver(c *gc.C) {
	l := s.testListener()
	l.maxPause = 0
	for i := 0; i < 250; i++ {
		l.Accept()
		s.clock.Advance(2 * time.Millisecond)
	}
	c.Assert(s.listener.count, gc.Equals, 250)
	c.Assert(s.roundRate(l.connRateMetric()), gc.Equals, 500)
}

func (s *listenerSuite) TestConnRateNonContiguous(c *gc.C) {
	l := s.testListener()
	l.maxPause = 0
	for i := 0; i < 90; i++ {
		l.Accept()
		if i == 80 {
			s.clock.Advance(50 * time.Millisecond)
		}
		s.clock.Advance(time.Millisecond)
	}
	c.Assert(s.listener.count, gc.Equals, 90)
	c.Assert(s.roundRate(l.connRateMetric()), gc.Equals, 600)
}

func (s *listenerSuite) TestConnRateIgnoresOld(c *gc.C) {
	l := s.testListener()
	l.maxPause = 0
	for i := 0; i < 140; i++ {
		l.Accept()
		if i < 100 {
			s.clock.Advance(10 * time.Microsecond)
		} else if i == 100 {
			s.clock.Advance(time.Second)
		} else {
			s.clock.Advance(10 * time.Microsecond)
		}
	}
	c.Assert(s.listener.count, gc.Equals, 140)
	c.Assert(l.connRateMetric(), gc.Equals, 39)
}

func (s *listenerSuite) setupConnRate(l *throttlingListener, rate int) {
	max := l.maxPause
	l.maxPause = 0
	for i := 0; i <= rate; i++ {
		l.Accept()
		s.clock.Advance(time.Second / time.Duration(rate))
	}
	l.maxPause = max
}

func (s *listenerSuite) TestPauseTimeMin(c *gc.C) {
	l := s.testListener()
	c.Assert(l.pauseTime(), gc.Equals, s.minPause)
	s.setupConnRate(l, s.lowerThreshold-100)
	c.Assert(l.pauseTime(), gc.Equals, s.minPause)
}

func (s *listenerSuite) TestPauseTimeMax(c *gc.C) {
	l := s.testListener()
	s.setupConnRate(l, s.upperThreshold+1)
	c.Assert(l.pauseTime(), gc.Equals, s.maxPause)
}

func (s *listenerSuite) TestPauseTimeInBetween(c *gc.C) {
	l := s.testListener()
	s.setupConnRate(l, (s.lowerThreshold+s.upperThreshold)/2)
	pauseTime := l.pauseTime()
	round := func(t time.Duration) float64 {
		return 1.0 * math.Floor(10.0*float64(t)/float64(time.Second)) / 10.0
	}
	c.Assert(round(pauseTime), gc.Equals, round(s.minPause+(s.maxPause-s.minPause+1)/2))
}

func (s *listenerSuite) TestPause(c *gc.C) {
	l := s.testListener()
	done := make(chan bool, 1)
	go func() {
		l.Accept()
		done <- true
	}()

	// Min pause is 10ms.
	err := s.clock.WaitAdvance(9*time.Millisecond, coretesting.ShortWait, 1)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-done:
		c.Fatal("pause returned too soon")
	case <-time.After(coretesting.ShortWait):
	}
	c.Assert(s.listener.count, gc.Equals, 0)
	err = s.clock.WaitAdvance(2*time.Millisecond, coretesting.ShortWait, 1)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
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
