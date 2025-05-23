// Copyright 2014 Cloudbase Solutions SRL
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot_test

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

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
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

// testMachine is an helper struct to keep machine information during tests
type testMachine struct {
	uuid      coremachine.UUID
	tag       names.Tag
	rebootAPI *reboot.RebootAPI
	args      params.Entities

	w  watcher.NotifyWatcher
	wc watchertest.NotifyWatcherC
}

var (
	// parentTag is used to create a parent machine
	parentTag = names.NewMachineTag("0/parent/0")
	// parentTag is used to create a child machine
	childTag = names.NewMachineTag("0/child/0")
)

type rebootSuite struct {
	testhelpers.CleanupSuite
	jujutesting.ApiServerSuite
	changestreamtesting.ModelSuite

	watcherRegistry facade.WatcherRegistry
	machineService  *service.WatchableService
}

func TestRebootSuite(t *testing.T) {
	tc.Run(t, &rebootSuite{})
}

func (s *rebootSuite) createMachine(c *tc.C, tag names.MachineTag) *testMachine {
	uuid, err := s.machineService.CreateMachine(c.Context(), coremachine.Name(tag.Id()))
	c.Assert(err, tc.ErrorIsNil)

	return s.setupMachine(c, tag, err, uuid)
}

func (s *rebootSuite) createMachineWithParent(c *tc.C, tag names.MachineTag, parent *testMachine) *testMachine {
	uuid, err := s.machineService.CreateMachineWithParent(c.Context(), coremachine.Name(tag.Id()), coremachine.Name(parent.tag.Id()))
	c.Assert(err, tc.ErrorIsNil)

	return s.setupMachine(c, tag, err, uuid)
}

func (s *rebootSuite) setupMachine(c *tc.C, tag names.MachineTag, err error, uuid coremachine.UUID) *testMachine {
	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming we logged in as a machine agent.
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: tag,
	}

	rebootAPI, err := reboot.NewRebootAPI(authorizer, s.watcherRegistry, s.machineService)
	c.Assert(err, tc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: tag.String()},
	}}

	watcherResult, err := rebootAPI.WatchForRebootEvent(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(watcherResult.NotifyWatcherId, tc.Not(tc.Equals), "")
	c.Check(watcherResult.Error, tc.IsNil)

	rebootWatcher, err := s.watcherRegistry.Get(watcherResult.NotifyWatcherId)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(rebootWatcher, tc.NotNil)

	w := rebootWatcher.(watcher.NotifyWatcher)
	wc := watchertest.NewNotifyWatcherC(c, w)
	wc.AssertNoChange()

	s.AddCleanup(func(c *tc.C) {
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

func (s *rebootSuite) SetUpSuite(c *tc.C) {
	s.CleanupSuite.SetUpSuite(c)
	s.ModelSuite.SetUpSuite(c)
	s.ApiServerSuite.SetUpSuite(c)
}

func (s *rebootSuite) SetUpTest(c *tc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.ModelSuite.SetUpTest(c)
	s.ApiServerSuite.SetUpTest(c)

	var err error
	s.watcherRegistry, err = registry.NewRegistry(clock.WallClock)
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(c *tc.C) {
		workertest.DirtyKill(c, s.watcherRegistry)
	})

	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "machine")
	s.machineService = service.NewWatchableService(
		state.NewState(
			func() (database.TxnRunner, error) { return factory() },
			clock.WallClock,
			loggertesting.WrapCheckLog(c),
		),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *rebootSuite) TearDownTest(c *tc.C) {

	s.CleanupSuite.TearDownTest(c)
	s.ApiServerSuite.TearDownTest(c)
	s.ModelSuite.TearDownTest(c)
}

func (s *rebootSuite) TearDownSuite(c *tc.C) {
	s.CleanupSuite.TearDownSuite(c)
	s.ApiServerSuite.TearDownSuite(c)
	s.ModelSuite.TearDownSuite(c)
}

// TestWatchForRebootEventFromChild tests the functionality of watching for a reboot event from a child machine.
// It creates a parent machine and a child machine, sends a reboot request to the child machine,
// and asserts that the child machine receives the reboot event while the parent machine does not.
func (s *rebootSuite) TestWatchForRebootEventFromChild(c *tc.C) {
	parent := s.createMachine(c, parentTag)
	child := s.createMachineWithParent(c, childTag, parent)

	_, err := child.rebootAPI.RequestReboot(c.Context(), child.args)
	c.Assert(err, tc.ErrorIsNil)

	child.wc.AssertOneChange()
	parent.wc.AssertNoChange()
}

// TestWatchForRebootEventFromParent tests the functionality of watching for a reboot event from a parent machine.
// It creates a parent machine and a child machine, sends a reboot request to the parent machine,
// and asserts that both the parent machine and child receives a reboot event.
func (s *rebootSuite) TestWatchForRebootEventFromParent(c *tc.C) {
	parent := s.createMachine(c, parentTag)
	child := s.createMachineWithParent(c, childTag, parent)

	_, err := parent.rebootAPI.RequestReboot(c.Context(), parent.args)
	c.Assert(err, tc.ErrorIsNil)

	parent.wc.AssertOneChange()
	child.wc.AssertOneChange()
}

// TestRequestReboot tests the functionality of requesting a reboot for a machine.
// It creates a machine, sends a reboot request, and asserts that the request is successful and the appropriate changes are made.
// Additionally, it verifies the reboot action after the request.
func (s *rebootSuite) TestRequestReboot(c *tc.C) {
	machine := s.createMachine(c, parentTag)

	errResult, err := machine.rebootAPI.RequestReboot(c.Context(), machine.args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errResult, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		}})

	machine.wc.AssertOneChange()

	res, err := machine.rebootAPI.GetRebootAction(c.Context(), machine.args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldReboot},
		}})
}

// TestClearReboot tests the functionality of clearing a reboot request for a machine.
// It creates a machine, sends a reboot request, and then clears the reboot request.
// It asserts that the request and clearing are successful and the appropriate changes are made.
// Additionally, it verifies the reboot action after the clearing.
func (s *rebootSuite) TestClearReboot(c *tc.C) {
	machine := s.createMachine(c, parentTag)

	errResult, err := machine.rebootAPI.RequestReboot(c.Context(), machine.args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errResult, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	machine.wc.AssertOneChange()

	res, err := machine.rebootAPI.GetRebootAction(c.Context(), machine.args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldReboot},
		}})

	errResult, err = machine.rebootAPI.ClearReboot(c.Context(), machine.args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errResult, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	res, err = machine.rebootAPI.GetRebootAction(c.Context(), machine.args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldDoNothing},
		}})
}

// TestRebootRequestFromParent tests the functionality of requesting a reboot on a parent machine.
// It creates a parent machine and a child machine, and sends a reboot request to the parent machine.
// It asserts that the reboot action for parent machine is reboot and the reboot action for child is
// to shut down.
// It also verifies that the proper rebootEvent are sent.
func (s *rebootSuite) TestRebootRequestFromParent(c *tc.C) {
	parent := s.createMachine(c, parentTag)
	child := s.createMachineWithParent(c, childTag, parent)
	// Request reboot on the root machine: all machines should see it
	// parent should reboot
	// child should shutdown
	errResult, err := parent.rebootAPI.RequestReboot(c.Context(), parent.args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errResult, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		}})

	parent.wc.AssertOneChange()
	child.wc.AssertOneChange()

	res, err := parent.rebootAPI.GetRebootAction(c.Context(), parent.args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldReboot},
		}})

	res, err = child.rebootAPI.GetRebootAction(c.Context(), child.args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldShutdown},
		}})

	errResult, err = parent.rebootAPI.ClearReboot(c.Context(), parent.args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errResult, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	parent.wc.AssertOneChange()
	child.wc.AssertOneChange()
}

// TestRebootRequestFromChild tests the functionality of requesting a reboot on a child machine.
// It creates a parent machine and a child machine, and sends a reboot request to the child machine.
// The parent machine should do nothing, while the child machine should reboot.
// It asserts that the appropriate changes are made and the expected reboot action is received.
// It also check that the correct events are sent.
func (s *rebootSuite) TestRebootRequestFromChild(c *tc.C) {
	parent := s.createMachine(c, parentTag)
	child := s.createMachineWithParent(c, childTag, parent)

	// Request reboot on the container: container and nested container should see it
	// parent should do nothing
	// child should reboot
	errResult, err := child.rebootAPI.RequestReboot(c.Context(), child.args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errResult, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		}})

	child.wc.AssertOneChange()
	parent.wc.AssertNoChange()

	res, err := parent.rebootAPI.GetRebootAction(c.Context(), parent.args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldDoNothing},
		}})

	res, err = child.rebootAPI.GetRebootAction(c.Context(), child.args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Result: params.ShouldReboot},
		}})

	errResult, err = child.rebootAPI.ClearReboot(c.Context(), child.args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errResult, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	child.wc.AssertOneChange()
	parent.wc.AssertNoChange()
}
