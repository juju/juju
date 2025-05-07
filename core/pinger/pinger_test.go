// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pinger

import (
	"time"

	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
)

type suite struct {
	testing.IsolationSuite

	clock *MockClock
}

var _ = tc.Suite(&suite{})

func (s *suite) TestPing(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.clock.EXPECT().After(time.Second).DoAndReturn(func(d time.Duration) <-chan time.Time {
		return make(chan time.Time)
	}).MinTimes(1)

	action := func() {
		c.Fatal("action should not be called")
	}

	p := NewPinger(action, s.clock, time.Second)
	defer workertest.CleanKill(c, p)

	for i := 0; i < 10; i++ {
		p.Ping()
		// Ensure that the ping timer has been at least called
		<-time.After(testing.ShortWait)
	}

	workertest.CheckKill(c, p)
}

func (s *suite) TestPingAfterKilled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.clock.EXPECT().After(time.Second).DoAndReturn(func(d time.Duration) <-chan time.Time {
		return make(chan time.Time)
	}).AnyTimes()

	action := func() {
		c.Fatal("action should not be called")
	}

	p := NewPinger(action, s.clock, time.Second)
	workertest.CheckKill(c, p)

	p.Ping()
}

func (s *suite) TestPingTimeout(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.clock.EXPECT().After(time.Second).DoAndReturn(func(d time.Duration) <-chan time.Time {
		ch := make(chan time.Time)
		go func() {
			ch <- time.Now()
		}()
		return ch
	}).MinTimes(1)

	sync := make(chan struct{})
	action := func() {
		close(sync)
	}

	p := NewPinger(action, s.clock, time.Second)
	defer workertest.DirtyKill(c, p)

	p.Ping()

	select {
	case <-sync:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for action")
	}

	err := p.Wait()
	c.Assert(err, tc.ErrorMatches, "ping timeout")
}

func (s *suite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)

	return ctrl
}
