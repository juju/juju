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

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/provider/dummy"
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
	cfg, uuid := dummyCfgAndUUID(c)
	client := &mockClient{
		calls: make(chan string),
		mockModel: clientModel{
			Life: state.Dying,
			UUID: uuid,
			HasMachinesAndServices: true,
		},
		cfg: cfg,
		watcher: &mockModelResourceWatcher{
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
			call: "ModelInfo",
		}, {
			call: "ProcessDyingModel",
			callback: func() {
				c.Check(client.mockModel.Life, gc.Equals, state.Dying)
				c.Check(client.mockModel.TimeOfDeath, gc.IsNil)
				client.mockModel.HasMachinesAndServices = false
				client.watcher.(*mockModelResourceWatcher).events <- struct{}{}
				mClock.advanceAfterNextNow(undertaker.RIPTime)
			}}, {
			call: "ProcessDyingModel",
			callback: func() {
				c.Check(client.mockModel.Life, gc.Equals, state.Dead)
				c.Check(client.mockModel.TimeOfDeath, gc.NotNil)
			}}, {
			call: "RemoveModel",
			callback: func() {
				oneDayLater := startTime.Add(undertaker.RIPTime)
				c.Check(mClock.Now().Equal(oneDayLater), jc.IsTrue)
				c.Check(client.mockModel.Removed, gc.Equals, true)
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

	worker, err := undertaker.NewUndertaker(client, mClock)
	c.Assert(err, jc.ErrorIsNil)
	defer worker.Kill()

	wg.Wait()

	assertNoMoreCalls(c, client)
}

func (s *undertakerSuite) TestRemoveModelDocsNotCalledForController(c *gc.C) {
	mockWatcher := &mockModelResourceWatcher{
		events: make(chan struct{}, 1),
	}
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	client := &mockClient{
		calls: make(chan string, 1),
		mockModel: clientModel{
			Life:     state.Dying,
			UUID:     uuid.String(),
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
			call: "ModelInfo",
			callback: func() {
				mockWatcher.events <- struct{}{}
			},
		}, {
			call: "ProcessDyingModel",
			callback: func() {
				c.Assert(client.mockModel.Life, gc.Equals, state.Dead)
				c.Assert(client.mockModel.TimeOfDeath, gc.NotNil)

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

	worker, err := undertaker.NewUndertaker(client, mClock)
	c.Assert(err, jc.ErrorIsNil)
	defer worker.Kill()

	wg.Wait()

	assertNoMoreCalls(c, client)
}

func (s *undertakerSuite) TestRemoveModelOnRebootCalled(c *gc.C) {
	startTime := time.Date(2015, time.September, 1, 17, 2, 1, 0, time.UTC)
	mClock := testing.NewClock(startTime)
	halfDayEarlier := mClock.Now().Add(-12 * time.Hour)

	cfg, uuid := dummyCfgAndUUID(c)
	client := &mockClient{
		calls: make(chan string, 1),
		// Mimic the situation where the worker is started after the
		// model has been set to dead 12hrs ago.
		mockModel: clientModel{
			Life:        state.Dead,
			UUID:        uuid,
			TimeOfDeath: &halfDayEarlier,
		},
		cfg: cfg,
	}

	wg := sync.WaitGroup{}
	wg.Add(1)

	// We expect RemoveModel not to be called, as we have to wait another
	// 12hrs.
	go func() {
		defer wg.Done()
		for _, test := range []struct {
			call     string
			callback func()
		}{{
			call: "ModelInfo",
			callback: func() {
				// As model was set to dead 12hrs earlier, assert that the
				// undertaker picks up where it left off and RemoveModel
				// is called 12hrs later.
				mClock.Advance(12 * time.Hour)
			},
		}, {
			call: "RemoveModel",
			callback: func() {
				c.Assert(client.mockModel.Removed, gc.Equals, true)
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

	worker, err := undertaker.NewUndertaker(client, mClock)
	c.Assert(err, jc.ErrorIsNil)
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

func dummyCfgAndUUID(c *gc.C) (*config.Config, string) {
	cfg := testingEnvConfig(c)
	uuid, ok := cfg.UUID()
	c.Assert(ok, jc.IsTrue)
	return cfg, uuid
}

// testingEnvConfig prepares an environment configuration using
// the dummy provider.
func testingEnvConfig(c *gc.C) *config.Config {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.Prepare(
		modelcmd.BootstrapContext(testing.Context(c)), configstore.NewMem(),
		jujuclienttesting.NewMemStore(),
		"dummycontroller", environs.PrepareForBootstrapParams{Config: cfg},
	)
	c.Assert(err, jc.ErrorIsNil)
	return env.Config()
}
