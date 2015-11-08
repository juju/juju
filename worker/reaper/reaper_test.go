// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reaper_test

import (
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/systemmanager"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker/reaper"
)

type reaperSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&reaperSuite{})

func (s *reaperSuite) TestAPICalls(c *gc.C) {
	client := &mockClient{
		calls: make(chan string, 1),
		mockEnviron: clientEnviron{
			Life: state.Dying,
			UUID: utils.MustNewUUID().String(),
			HasMachinesAndServices: true,
		},
	}
	startTime := time.Date(2015, time.September, 1, 17, 2, 1, 0, time.UTC)
	mClock := testing.NewClock(startTime)

	worker := reaper.NewReaper(client, mClock)
	defer worker.Kill()

	c.Assert(client.mockEnviron.TimeOfDeath, gc.IsNil)
	for _, test := range []struct {
		call     string
		callback func()
	}{{
		call: "EnvironInfo",
		callback: func() {
			mClock.Advance(reaper.ReaperPeriod)
		},
	}, {
		call: "ProcessDyingEnviron",
		callback: func() {
			c.Assert(client.mockEnviron.Life, gc.Equals, state.Dying)
			c.Assert(client.mockEnviron.TimeOfDeath, gc.IsNil)
			client.mockEnviron.HasMachinesAndServices = false

			mClock.Advance(reaper.ReaperPeriod)
		}}, {
		call: "ProcessDyingEnviron",
		callback: func() {
			nekMinute := startTime.Add(reaper.ReaperPeriod)
			c.Assert(mClock.Now().After(nekMinute), jc.IsTrue)

			c.Assert(client.mockEnviron.Life, gc.Equals, state.Dead)
			c.Assert(client.mockEnviron.TimeOfDeath, gc.NotNil)

			mClock.Advance(reaper.RIPTime)
		}}, {
		call: "RemoveEnviron",
		callback: func() {
			oneDayLater := startTime.Add(reaper.RIPTime)
			c.Assert(mClock.Now().After(oneDayLater), jc.IsTrue)
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

func (s *reaperSuite) TestRemoveEnvironDocsNotCalledForStateServer(c *gc.C) {
	client := &mockClient{
		calls: make(chan string, 1),
		mockEnviron: clientEnviron{
			Life:     state.Dying,
			UUID:     utils.MustNewUUID().String(),
			IsSystem: true,
		},
	}
	startTime := time.Date(2015, time.September, 1, 17, 2, 1, 0, time.UTC)
	mClock := testing.NewClock(startTime)
	worker := reaper.NewReaper(client, mClock)
	defer worker.Kill()

	c.Assert(client.mockEnviron.TimeOfDeath, gc.IsNil)
	for _, test := range []struct {
		call     string
		callback func()
	}{{
		call: "EnvironInfo",
		callback: func() {
			mClock.Advance(reaper.ReaperPeriod)
		},
	}, {
		call: "ProcessDyingEnviron",
		callback: func() {
			c.Assert(client.mockEnviron.Life, gc.Equals, state.Dead)
			c.Assert(client.mockEnviron.TimeOfDeath, gc.NotNil)

			mClock.Advance(reaper.RIPTime)
		}}} {
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

func (s *reaperSuite) TestRemoveEnvironOnRebootCalled(c *gc.C) {
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

	worker := reaper.NewReaper(client, mClock)
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
			// reaper picks up where it left off and RemoveEnviron
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

// ----

type destroySystemSuite struct {
	jujutesting.JujuConnSuite
	commontesting.BlockHelper

	systemManager *systemmanager.SystemManagerAPI

	otherState    *state.State
	otherEnvOwner names.UserTag
	otherEnvUUID  string
}

var _ = gc.Suite(&destroySystemSuite{})

func (s *destroySystemSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })

	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })

	authoriser := apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	systemManager, err := systemmanager.NewSystemManagerAPI(s.State, resources, authoriser)
	c.Assert(err, jc.ErrorIsNil)
	s.systemManager = systemManager

	s.otherEnvOwner = names.NewUserTag("jess@dummy")
	s.otherState = factory.NewFactory(s.State).MakeEnvironment(c, &factory.EnvParams{
		Name:    "dummytoo",
		Owner:   s.otherEnvOwner,
		Prepare: true,
		ConfigAttrs: testing.Attrs{
			"state-server": false,
		},
	})
	s.AddCleanup(func(c *gc.C) { s.otherState.Close() })
	s.otherEnvUUID = s.otherState.EnvironUUID()
}

func (s *destroySystemSuite) TestDestroySystemKillsHostedEnvsWithBlocks(c *gc.C) {
	s.startMockReaper(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroyEnvironment")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	s.otherState.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	s.otherState.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")
	err := s.systemManager.DestroySystem(params.DestroySystemArgs{
		DestroyEnvironments: true,
		IgnoreBlocks:        true,
	})
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.otherState.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dead)

	env, err = s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroySystemSuite) TestDestroySystemReturnsBlockedEnvironmentsErr(c *gc.C) {
	s.startMockReaper(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroyEnvironment")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	s.otherState.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	s.otherState.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	err := s.systemManager.DestroySystem(params.DestroySystemArgs{
		DestroyEnvironments: true,
	})
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)

	numBlocks, err := s.State.AllBlocksForSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 4)

	_, err = s.otherState.Environment()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *destroySystemSuite) TestDestroySystemKillsHostedEnvs(c *gc.C) {
	s.startMockReaper(c)
	err := s.systemManager.DestroySystem(params.DestroySystemArgs{
		DestroyEnvironments: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.otherState.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dead)
	env, err = s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroySystemSuite) TestDestroySystemLeavesBlocksIfIgnoreBlocks(c *gc.C) {
	s.startMockReaper(c)
	s.BlockDestroyEnvironment(c, "TestBlockDestroyEnvironment")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")
	s.otherState.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyEnvironment")
	s.otherState.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	err := s.systemManager.DestroySystem(params.DestroySystemArgs{})
	c.Assert(err, gc.ErrorMatches, "found blocks in system environments")

	numBlocks, err := s.State.AllBlocksForSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 4)
}

func (s *destroySystemSuite) TestDestroySystemNoHostedEnvs(c *gc.C) {
	s.startMockReaper(c)
	err := common.DestroyEnvironment(s.State, s.otherState.EnvironTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	err = s.systemManager.DestroySystem(params.DestroySystemArgs{})
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroySystemSuite) TestDestroySystemNoHostedEnvsWithBlock(c *gc.C) {
	s.startMockReaper(c)
	err := common.DestroyEnvironment(s.State, s.otherState.EnvironTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	s.BlockDestroyEnvironment(c, "TestBlockDestroyEnvironment")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	err = s.systemManager.DestroySystem(params.DestroySystemArgs{
		IgnoreBlocks: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
}

func (s *destroySystemSuite) TestDestroySystemNoHostedEnvsWithBlockFail(c *gc.C) {
	s.startMockReaper(c)
	err := common.DestroyEnvironment(s.State, s.otherState.EnvironTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	s.BlockDestroyEnvironment(c, "TestBlockDestroyEnvironment")
	s.BlockRemoveObject(c, "TestBlockRemoveObject")

	err = s.systemManager.DestroySystem(params.DestroySystemArgs{})
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)

	numBlocks, err := s.State.AllBlocksForSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(numBlocks), gc.Equals, 2)
}

// startMockReaper processes a dying environment and sets it to dead.
// Normally the reaper worker, running on the machine agent, would
// process all dying environments. But for the unit tests, we run a mock
// reaper in the background.
func (s *destroySystemSuite) startMockReaper(c *gc.C) {
	watcher := watchEnvironResources(c, s.otherState)
	s.AddCleanup(func(c *gc.C) { c.Assert(watcher.Stop(), jc.ErrorIsNil) })
	go func() {
		for {
			select {
			case _, ok := <-watcher.Changes():
				if !ok {
					return
				}
				if err := s.otherState.ProcessDyingEnviron(); err == nil {
					watcher.Stop()
					return
				}
			}
		}
	}()
}

func watchEnvironResources(c *gc.C, st *state.State) state.NotifyWatcher {
	env, err := st.Environment()
	c.Assert(err, jc.ErrorIsNil)
	machines, err := st.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	services, err := st.AllServices()
	c.Assert(err, jc.ErrorIsNil)

	watchers := []state.NotifyWatcher{env.Watch()}
	for _, machine := range machines {
		watchers = append(watchers, machine.Watch())
	}
	for _, service := range services {
		watchers = append(watchers, service.Watch())
	}
	return common.NewMultiNotifyWatcher(watchers...)
}
