// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pinger

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
)

type suite struct {
	testhelpers.IsolationSuite

	clock *MockClock
}

func TestSuite(t *testing.T) {
	tc.Run(t, &suite{})
}

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
		<-time.After(testhelpers.ShortWait)
	}

	workertest.CleanKill(c, p)
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
	workertest.CleanKill(c, p)

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
	case <-time.After(testhelpers.ShortWait):
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
