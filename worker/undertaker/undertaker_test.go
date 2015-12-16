// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/undertaker"
)

type undertakerSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&undertakerSuite{})

func (s *undertakerSuite) TestAPICalls(c *gc.C) {
	client := &mockClient{
		calls: make(chan string, 1),
		mockEnviron: clientEnviron{
			Life: state.Dying,
			UUID: utils.MustNewUUID().String(),
			HasMachinesAndServices: true,
		},
		watcher: &mockEnvironResourceWatcher{
			events: make(chan struct{}, 1),
		},
	}
	startTime := time.Date(2015, time.September, 1, 17, 2, 1, 0, time.UTC)
	mClock := testing.NewClock(startTime)
	worker := undertaker.NewUndertaker(client, mClock)
	defer worker.Kill()

	c.Assert(client.mockEnviron.TimeOfDeath, gc.IsNil)
	for _, test := range []struct {
		call     string
		callback func()
	}{{
		call: "EnvironInfo",
		callback: func() {
			client.watcher.(*mockEnvironResourceWatcher).events <- struct{}{}
		},
	}, {
		call: "ProcessDyingEnviron",
		callback: func() {

			c.Assert(client.mockEnviron.Life, gc.Equals, state.Dying)
			c.Assert(client.mockEnviron.TimeOfDeath, gc.IsNil)
			client.mockEnviron.HasMachinesAndServices = false
			client.watcher.(*mockEnvironResourceWatcher).events <- struct{}{}

		}}, {
		call: "ProcessDyingEnviron",
		callback: func() {
			c.Assert(client.mockEnviron.Life, gc.Equals, state.Dead)
			c.Assert(client.mockEnviron.TimeOfDeath, gc.NotNil)

			mClock.Advance(undertaker.RIPTime)
		}}, {
		call: "RemoveEnviron",
		callback: func() {
			oneDayLater := startTime.Add(undertaker.RIPTime)
			c.Assert(mClock.Now().Equal(oneDayLater), jc.IsTrue)
			c.Assert(client.mockEnviron.Removed, gc.Equals, true)
		}},
	} {
		select {
		case call := <-client.calls:
			c.Assert(call, gc.Equals, test.call)
			if test.callback != nil {
				test.callback()
			}
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for API call: %q", test.call)
		}
	}

	select {
	case call := <-client.calls:
		c.Fatalf("unexpected API call: %q", call)
	case <-time.After(testing.ShortWait):
	}
}

func (s *undertakerSuite) TestRemoveEnvironDocsNotCalledForStateServer(c *gc.C) {
	mockWatcher := &mockEnvironResourceWatcher{
		events: make(chan struct{}, 1),
	}
	client := &mockClient{
		calls: make(chan string, 1),
		mockEnviron: clientEnviron{
			Life:     state.Dying,
			UUID:     utils.MustNewUUID().String(),
			IsSystem: true,
		},
		watcher: mockWatcher,
	}
	startTime := time.Date(2015, time.September, 1, 17, 2, 1, 0, time.UTC)
	mClock := testing.NewClock(startTime)
	worker := undertaker.NewUndertaker(client, mClock)
	defer worker.Kill()

	c.Assert(client.mockEnviron.TimeOfDeath, gc.IsNil)
	for _, test := range []struct {
		call     string
		callback func()
	}{{
		call: "EnvironInfo",
		callback: func() {
			mockWatcher.events <- struct{}{}
		},
	}, {
		call: "ProcessDyingEnviron",
		callback: func() {
			c.Assert(client.mockEnviron.Life, gc.Equals, state.Dead)
			c.Assert(client.mockEnviron.TimeOfDeath, gc.NotNil)

			mClock.Advance(undertaker.RIPTime)
		},
	},
	} {
		select {
		case call := <-client.calls:
			c.Assert(call, gc.Equals, test.call)
			if test.callback != nil {
				test.callback()
			}
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for API call: %q", test.call)
		}
	}

	select {
	case call := <-client.calls:
		c.Fatalf("unexpected API call: %q", call)
	case <-time.After(testing.ShortWait):
	}
}

func (s *undertakerSuite) TestRemoveEnvironOnRebootCalled(c *gc.C) {
	startTime := time.Date(2015, time.September, 1, 17, 2, 1, 0, time.UTC)
	mClock := testing.NewClock(startTime)
	halfDayEarlier := mClock.Now().Add(-12 * time.Hour)

	client := &mockClient{
		calls: make(chan string, 1),
		// Mimic the situation where the worker is started after the
		// environment has been set to dead 12hrs ago.
		mockEnviron: clientEnviron{
			Life:        state.Dead,
			UUID:        utils.MustNewUUID().String(),
			TimeOfDeath: &halfDayEarlier,
		},
	}

	worker := undertaker.NewUndertaker(client, mClock)
	defer worker.Kill()

	// We expect RemoveEnviron not to be called, as we have to wait another
	// 12hrs.
	for _, test := range []struct {
		call     string
		callback func()
	}{{
		call: "EnvironInfo",
		callback: func() {
			// As environ was set to dead 12hrs earlier, assert that the
			// undertaker picks up where it left off and RemoveEnviron
			// is called 12hrs later.
			mClock.Advance(12 * time.Hour)
		},
	}, {
		call: "RemoveEnviron",
		callback: func() {
			c.Assert(client.mockEnviron.Removed, gc.Equals, true)
		}},
	} {
		select {
		case call := <-client.calls:
			c.Assert(call, gc.Equals, test.call)
			if test.callback != nil {
				test.callback()
			}
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for API call: %q", test.call)
		}
	}

	select {
	case call := <-client.calls:
		c.Fatalf("unexpected API call: %q", call)
	case <-time.After(testing.ShortWait):
	}
}
