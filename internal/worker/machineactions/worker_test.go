// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/internal/worker/machineactions"
	"github.com/juju/juju/internal/worker/machineactions/mocks"
	"github.com/juju/juju/rpc/params"
)

type WorkerSuite struct {
	testing.IsolationSuite

	facade *mocks.MockFacade
	lock   *mocks.MockLock
}

var _ = tc.Suite(&WorkerSuite{})

func (*WorkerSuite) TestInvalidFacade(c *tc.C) {
	worker, err := machineactions.NewMachineActionsWorker(machineactions.WorkerConfig{
		Facade: nil,
	})
	c.Assert(err, tc.ErrorMatches, "nil Facade not valid")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(worker, tc.IsNil)
}

func (s *WorkerSuite) TestInvalidMachineTag(c *tc.C) {
	worker, err := machineactions.NewMachineActionsWorker(machineactions.WorkerConfig{
		Facade:     s.facade,
		MachineTag: names.MachineTag{},
	})
	c.Assert(err, tc.ErrorMatches, "unspecified MachineTag not valid")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(worker, tc.IsNil)
}

func (s *WorkerSuite) TestInvalidHandleAction(c *tc.C) {
	worker, err := machineactions.NewMachineActionsWorker(machineactions.WorkerConfig{
		Facade:       s.facade,
		MachineTag:   fakeTag,
		MachineLock:  s.lock,
		HandleAction: nil,
	})
	c.Assert(err, tc.ErrorMatches, "nil HandleAction not valid")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(worker, tc.IsNil)
}

func defaultConfig(stub *testing.Stub, facade machineactions.Facade, lock machinelock.Lock) machineactions.WorkerConfig {
	return machineactions.WorkerConfig{
		Facade:       facade,
		MachineTag:   fakeTag,
		HandleAction: mockHandleAction(stub),
		MachineLock:  lock,
	}
}

func (s *WorkerSuite) TestRunningActionsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.facade.EXPECT().RunningActions(gomock.Any(), fakeTag).Return(nil, errors.New("splash"))

	stub := &testing.Stub{}
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub, s.facade, s.lock))
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.ErrorMatches, "splash")
	stub.CheckNoCalls(c)
}

func (s *WorkerSuite) TestInvalidActionId(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 1)
	changes <- []string{"invalid-action-id"}

	s.facade.EXPECT().RunningActions(gomock.Any(), fakeTag).Return([]params.ActionResult{}, nil)
	s.facade.EXPECT().WatchActionNotifications(gomock.Any(), fakeTag).Return(&stubWatcher{
		Worker:  workertest.NewErrorWorker(nil),
		changes: changes}, nil)

	stub := &testing.Stub{}
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub, s.facade, s.lock))
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.ErrorMatches, "got invalid action id invalid-action-id")
	stub.CheckNoCalls(c)
}

func (s *WorkerSuite) TestWatchErrorNonEmptyRunningActions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.facade.EXPECT().RunningActions(gomock.Any(), fakeTag).Return(fakeRunningActions, nil)
	s.facade.EXPECT().ActionFinish(gomock.Any(), names.NewActionTag("3"), params.ActionFailed, nil, "action cancelled").Return(nil)
	s.facade.EXPECT().WatchActionNotifications(gomock.Any(), fakeTag).Return(nil, errors.New("kuso"))

	stub := &testing.Stub{}
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub, s.facade, s.lock))
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Check(errors.Cause(err), tc.ErrorMatches, "kuso")
	stub.CheckNoCalls(c)
}

func (s *WorkerSuite) TestCannotRetrieveAction(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.facade.EXPECT().RunningActions(gomock.Any(), fakeTag).Return([]params.ActionResult{}, nil)
	s.facade.EXPECT().WatchActionNotifications(gomock.Any(), fakeTag).Return(newStubWatcher(false), nil)
	s.facade.EXPECT().Action(gomock.Any(), names.NewActionTag("1")).Times(1).Return(firstAction, nil)
	s.facade.EXPECT().Action(gomock.Any(), names.NewActionTag("2")).Times(1).Return(nil, errors.New("zbosh"))
	s.facade.EXPECT().Action(gomock.Any(), names.NewActionTag("3")).Times(1).Return(thirdAction, nil)

	s.facade.EXPECT().ActionBegin(gomock.Any(), names.NewActionTag("1")).Return(nil)
	s.facade.EXPECT().ActionFinish(gomock.Any(), names.NewActionTag("1"), params.ActionCompleted, nil, "").Return(nil)

	s.facade.EXPECT().ActionBegin(gomock.Any(), names.NewActionTag("3")).Return(nil)
	s.facade.EXPECT().ActionFinish(gomock.Any(), names.NewActionTag("3"), params.ActionCompleted, nil, "").Return(nil)

	stub := &testing.Stub{}
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub, s.facade, s.lock))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)

	// Ensure we're still alive if the action can't be retrieved. Instead we'll
	// wait for another pass.
	workertest.CheckAlive(c, worker)

	// Ensure we can clean kill.
	workertest.CleanKill(c, worker)

	stub.CheckCallsUnordered(c, []testing.StubCall{{
		FuncName: "HandleAction",
		Args:     []interface{}{firstAction.Name()},
	}, {
		FuncName: "HandleAction",
		Args:     []interface{}{thirdAction.Name()},
	}})
}

func (s *WorkerSuite) TestRunActions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	released := false
	s.facade.EXPECT().RunningActions(gomock.Any(), fakeTag).Return([]params.ActionResult{}, nil)
	s.facade.EXPECT().WatchActionNotifications(gomock.Any(), fakeTag).Return(newStubWatcher(true), nil)
	s.facade.EXPECT().Action(gomock.Any(), names.NewActionTag("1")).Times(1).Return(firstAction, nil)
	s.facade.EXPECT().Action(gomock.Any(), names.NewActionTag("2")).Times(1).Return(secondAction, nil)
	s.facade.EXPECT().Action(gomock.Any(), names.NewActionTag("3")).Times(1).Return(thirdAction, nil)

	// Action 1 cannot start.
	begin1 := s.facade.EXPECT().ActionBegin(gomock.Any(), names.NewActionTag("1")).Times(1).Return(errors.New("kermack"))
	s.facade.EXPECT().ActionFinish(gomock.Any(), names.NewActionTag("1"), params.ActionFailed, nil, "could not begin action foo: kermack").After(begin1).Return(nil)

	// Action 2 does not run in parallel, so needs the lock.
	acquire2 := s.lock.EXPECT().Acquire(gomock.Any()).Times(1).Return(func() {
		released = true
	}, nil)
	begin2 := s.facade.EXPECT().ActionBegin(gomock.Any(), names.NewActionTag("2")).After(acquire2).Return(nil)
	s.facade.EXPECT().ActionFinish(gomock.Any(), names.NewActionTag("2"), params.ActionCompleted, nil, "").After(begin2).Return(nil)
	begin3 := s.facade.EXPECT().ActionBegin(gomock.Any(), names.NewActionTag("3")).Times(1).Return(nil)
	s.facade.EXPECT().ActionFinish(gomock.Any(), names.NewActionTag("3"), params.ActionCompleted, nil, "").After(begin3).Return(nil)

	stub := &testing.Stub{}
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub, s.facade, s.lock))
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Check(errors.Cause(err), tc.ErrorMatches, "got invalid action id invalid-action-id")
	stub.CheckCallsUnordered(c, []testing.StubCall{{
		FuncName: "HandleAction",
		Args:     []interface{}{secondAction.Name()},
	}, {
		FuncName: "HandleAction",
		Args:     []interface{}{thirdAction.Name()},
	}})
	c.Assert(released, jc.IsTrue)
}

func (s *WorkerSuite) TestActionHandleError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.facade.EXPECT().RunningActions(gomock.Any(), fakeTag).Return([]params.ActionResult{}, nil)
	s.facade.EXPECT().WatchActionNotifications(gomock.Any(), fakeTag).Return(newStubWatcher(false), nil)
	s.facade.EXPECT().Action(gomock.Any(), names.NewActionTag("1")).Times(1).Return(firstAction, nil)
	s.facade.EXPECT().Action(gomock.Any(), names.NewActionTag("2")).Times(1).Return(nil, errors.New("boom"))
	s.facade.EXPECT().Action(gomock.Any(), names.NewActionTag("3")).Times(1).Return(thirdAction, nil)

	s.facade.EXPECT().ActionBegin(gomock.Any(), names.NewActionTag("1")).Times(1).Return(nil)
	s.facade.EXPECT().ActionBegin(gomock.Any(), names.NewActionTag("3")).Times(1).Return(nil)

	// To deal with the race because of the goroutines, we will use assertions based on the message.
	assertFinish := func(_ context.Context, tag names.ActionTag, status string, results map[string]interface{}, message string) error {
		if message == "slob" {
			c.Assert(status, tc.Equals, params.ActionFailed)
			return nil
		}
		c.Assert(status, tc.Equals, params.ActionCompleted)
		return nil
	}
	s.facade.EXPECT().ActionFinish(gomock.Any(), names.NewActionTag("1"), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(assertFinish)
	s.facade.EXPECT().ActionFinish(gomock.Any(), names.NewActionTag("3"), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(assertFinish)

	stub := &testing.Stub{}
	stub.SetErrors(errors.New("slob"))
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub, s.facade, s.lock))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)

	// Ensure we're still alive if the action can't be retrieved. Instead we'll
	// wait for another pass.
	workertest.CheckAlive(c, worker)

	// Ensure we can clean kill.
	workertest.CleanKill(c, worker)

	stub.CheckCallsUnordered(c, []testing.StubCall{{
		FuncName: "HandleAction",
		Args:     []interface{}{firstAction.Name()},
	}, {
		FuncName: "HandleAction",
		Args:     []interface{}{thirdAction.Name()},
	}})
}

func (s *WorkerSuite) TestWorkerNoError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.facade.EXPECT().RunningActions(gomock.Any(), fakeTag).Return([]params.ActionResult{}, nil)
	s.facade.EXPECT().WatchActionNotifications(gomock.Any(), fakeTag).Return(&stubWatcher{
		Worker: workertest.NewErrorWorker(nil),
	}, nil)

	stub := &testing.Stub{}
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub, s.facade, s.lock))
	c.Assert(err, jc.ErrorIsNil)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
	stub.CheckNoCalls(c)
}

func (s *WorkerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.facade = mocks.NewMockFacade(ctrl)
	s.lock = mocks.NewMockLock(ctrl)
	return ctrl
}
