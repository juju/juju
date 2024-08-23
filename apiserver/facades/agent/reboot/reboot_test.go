// Copyright 2014 Cloudbase Solutions SRL
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot_test

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/agent/reboot"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/registry"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/machine/service"
	"github.com/juju/juju/domain/machine/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type testMachine struct {
	uuid      string
	tag       names.Tag
	rebootAPI *reboot.RebootAPI
	args      params.Entities

	w  watcher.NotifyWatcher
	wc watchertest.NotifyWatcherC
}

type rebootSuite struct {
	jujutesting.ApiServerSuite
	changestreamtesting.ModelSuite

	watcherRegistry facade.WatcherRegistry
	machineService  *service.WatchableService
}

var (
	parentTag = names.NewMachineTag("0/parent/0")
	childTag  = names.NewMachineTag("0/child/0")
)

var _ = gc.Suite(&rebootSuite{})

// TODO: it is very strange here because we assume that there is no such thing as grandparent for parent in dqlite
// But this test is actually behaving like there is a grandparent... I removed nested container but it make me wonder.

func (s *rebootSuite) createMachine(c *gc.C, tag names.MachineTag) *testMachine {
	uuid, err := s.machineService.CreateMachine(context.Background(), coremachine.Name(tag.Id()))
	c.Assert(err, jc.ErrorIsNil)

	return s.setupMachine(c, tag, err, uuid)
}

func (s *rebootSuite) createMachineWithParent(c *gc.C, tag names.MachineTag, parent *testMachine) *testMachine {
	uuid, err := s.machineService.CreateMachineWithParent(context.Background(), coremachine.Name(tag.Id()), coremachine.Name(parent.tag.Id()))
	c.Assert(err, jc.ErrorIsNil)

	return s.setupMachine(c, tag, err, uuid)
}

func (s *rebootSuite) setupMachine(c *gc.C, tag names.MachineTag, err error, uuid string) *testMachine {
	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming we logged in as a machine agent.
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: tag,
	}

	rebootAPI, err := reboot.NewRebootAPI(authorizer, s.watcherRegistry, s.machineService)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: tag.String()},
	}}

	watcherResult, err := rebootAPI.WatchForRebootEvent(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(watcherResult.NotifyWatcherId, gc.Not(gc.Equals), "")
	c.Check(watcherResult.Error, gc.IsNil)

	rebootWatcher, err := s.watcherRegistry.Get(watcherResult.NotifyWatcherId)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(rebootWatcher, gc.NotNil)

	w := rebootWatcher.(watcher.NotifyWatcher)
	wc := watchertest.NewNotifyWatcherC(c, w)
	wc.AssertNoChange()

	s.AddCleanup(func(c *gc.C) {
		watchertest.DirtyKill(c, w)
		wc.AssertKilled()
	})

	return &testMachine{
		uuid:      uuid,
		tag:       tag,
		rebootAPI: rebootAPI,
		args:      args,
		w:         w,
		wc:        wc,
	}
}

func (s *rebootSuite) SetUpSuite(c *gc.C) {
	s.ModelSuite.SetUpSuite(c)
	s.ApiServerSuite.SetUpSuite(c)

	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "machine")
	s.machineService = service.NewWatchableService(
		state.NewState(
			func() (database.TxnRunner, error) { return factory() },
			loggertesting.WrapCheckLog(c),
		),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
	)
}

func (s *rebootSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)
	s.ApiServerSuite.SetUpTest(c)

	var err error
	s.watcherRegistry, err = registry.NewRegistry(clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.watcherRegistry) })
}

func (s *rebootSuite) TearDownTest(c *gc.C) {
	s.ApiServerSuite.TearDownTest(c)
	s.ModelSuite.TearDownTest(c)
}

func (s *rebootSuite) TearDownSuite(c *gc.C) {
	s.ApiServerSuite.TearDownSuite(c)
	s.ModelSuite.TearDownSuite(c)
}

func (s *rebootSuite) TestWatchForRebootEventFromChild(c *gc.C) {
	parent := s.createMachine(c, parentTag)
	child := s.createMachineWithParent(c, childTag, parent)

	_, err := child.rebootAPI.RequestReboot(context.Background(), child.args)
	c.Assert(err, jc.ErrorIsNil)

	child.wc.AssertOneChange()
	parent.wc.AssertNoChange()
}

func (s *rebootSuite) TestWatchForRebootEventFromParent(c *gc.C) {
	parent := s.createMachine(c, parentTag)
	child := s.createMachineWithParent(c, childTag, parent)

	_, err := parent.rebootAPI.RequestReboot(context.Background(), parent.args)
	c.Assert(err, jc.ErrorIsNil)

	parent.wc.AssertOneChange()
	child.wc.AssertOneChange()
}

func (s *rebootSuite) TestRequestReboot(c *gc.C) {
	machine := s.createMachine(c, parentTag)

	errResult, err := machine.rebootAPI.RequestReboot(context.Background(), machine.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		}})

	machine.wc.AssertOneChange()

	res, err := machine.rebootAPI.GetRebootAction(context.Background(), machine.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldReboot},
		}})
}

func (s *rebootSuite) TestClearReboot(c *gc.C) {
	machine := s.createMachine(c, parentTag)

	errResult, err := machine.rebootAPI.RequestReboot(context.Background(), machine.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	machine.wc.AssertOneChange()

	res, err := machine.rebootAPI.GetRebootAction(context.Background(), machine.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldReboot},
		}})

	errResult, err = machine.rebootAPI.ClearReboot(context.Background(), machine.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	res, err = machine.rebootAPI.GetRebootAction(context.Background(), machine.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldDoNothing},
		}})
}

func (s *rebootSuite) TestRebootRequestFromParent(c *gc.C) {
	parent := s.createMachine(c, parentTag)
	child := s.createMachineWithParent(c, childTag, parent)
	// Request reboot on the root machine: all machines should see it
	// parent should reboot
	// child should shutdown
	errResult, err := parent.rebootAPI.RequestReboot(context.Background(), parent.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		}})

	parent.wc.AssertOneChange()
	child.wc.AssertOneChange()

	res, err := parent.rebootAPI.GetRebootAction(context.Background(), parent.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldReboot},
		}})

	res, err = child.rebootAPI.GetRebootAction(context.Background(), child.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldShutdown},
		}})

	errResult, err = parent.rebootAPI.ClearReboot(context.Background(), parent.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	parent.wc.AssertOneChange()
	child.wc.AssertOneChange()
}

func (s *rebootSuite) TestRebootRequestFromContainer(c *gc.C) {
	parent := s.createMachine(c, parentTag)
	child := s.createMachineWithParent(c, childTag, parent)

	// Request reboot on the container: container and nested container should see it
	// parent should do nothing
	// child should reboot
	errResult, err := child.rebootAPI.RequestReboot(context.Background(), child.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		}})

	child.wc.AssertOneChange()
	parent.wc.AssertNoChange()

	res, err := parent.rebootAPI.GetRebootAction(context.Background(), parent.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldDoNothing},
		}})

	res, err = child.rebootAPI.GetRebootAction(context.Background(), child.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldReboot},
		}})

	errResult, err = child.rebootAPI.ClearReboot(context.Background(), child.args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	child.wc.AssertOneChange()
	parent.wc.AssertNoChange()
}
