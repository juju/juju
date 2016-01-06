// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"sync"
	"sync/atomic"
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

type clock struct {
	// advanceDurationAfterNow is the duration to advance the clock after the
	// next call to Now().
	advanceDurationAfterNow int64

	*testing.Clock
}

func (c *clock) Now() time.Time {
	now := c.Clock.Now()
	d := atomic.LoadInt64(&c.advanceDurationAfterNow)
	if d != 0 {
		c.Clock.Advance(time.Duration(d))
		atomic.StoreInt64(&c.advanceDurationAfterNow, 0)
	}

	return now
}

func (c *clock) advanceAfterNextNow(d time.Duration) {
	atomic.StoreInt64(&c.advanceDurationAfterNow, int64(d))
}

func (s *undertakerSuite) TestAPICalls(c *gc.C) {
	client := &mockClient{
		calls: make(chan string),
		mockEnviron: clientEnviron{
			Life: state.Dying,
			UUID: utils.MustNewUUID().String(),
			HasMachinesAndServices: true,
		},
		watcher: &mockEnvironResourceWatcher{
			events: make(chan struct{}),
		},
	}

	startTime := time.Date(2015, time.September, 1, 17, 2, 1, 0, time.UTC)
	mClock := &clock{
		Clock: testing.NewClock(startTime),
	}

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		for _, test := range []struct {
			call     string
			callback func()
		}{{
			call: "EnvironInfo",
		}, {
			call: "ProcessDyingEnviron",
			callback: func() {
				c.Check(client.mockEnviron.Life, gc.Equals, state.Dying)
				c.Check(client.mockEnviron.TimeOfDeath, gc.IsNil)
				client.mockEnviron.HasMachinesAndServices = false
				client.watcher.(*mockEnvironResourceWatcher).events <- struct{}{}
				mClock.advanceAfterNextNow(undertaker.RIPTime)
			}}, {
			call: "ProcessDyingEnviron",
			callback: func() {
				c.Check(client.mockEnviron.Life, gc.Equals, state.Dead)
				c.Check(client.mockEnviron.TimeOfDeath, gc.NotNil)
			}}, {
			call: "RemoveEnviron",
			callback: func() {
				oneDayLater := startTime.Add(undertaker.RIPTime)
				c.Check(mClock.Now().Equal(oneDayLater), jc.IsTrue)
				c.Check(client.mockEnviron.Removed, gc.Equals, true)
			}},
		} {
			select {
			case call := <-client.calls:
				c.Check(call, gc.Equals, test.call)
				if test.callback != nil {
					test.callback()
				}
			case <-time.After(testing.LongWait):
				c.Fatalf("timed out waiting for API call: %q", test.call)
			}
		}
	}()

	worker := undertaker.NewUndertaker(client, mClock)
	defer worker.Kill()

	wg.Wait()

	assertNoMoreCalls(c, client)
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
	mClock := &clock{
		Clock: testing.NewClock(startTime),
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()

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

				mClock.advanceAfterNextNow(undertaker.RIPTime)
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
	}()

	worker := undertaker.NewUndertaker(client, mClock)
	defer worker.Kill()

	wg.Wait()

	assertNoMoreCalls(c, client)
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

	wg := sync.WaitGroup{}
	wg.Add(1)

	// We expect RemoveEnviron not to be called, as we have to wait another
	// 12hrs.
	go func() {
		defer wg.Done()
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
	}()

	worker := undertaker.NewUndertaker(client, mClock)
	defer worker.Kill()

	wg.Wait()

	assertNoMoreCalls(c, client)
}

func assertNoMoreCalls(c *gc.C, client *mockClient) {
	select {
	case call := <-client.calls:
		c.Fatalf("unexpected API call: %q", call)
	case <-time.After(testing.ShortWait):
	}
}
