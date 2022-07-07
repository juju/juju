// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/worker/machineactions"
	"github.com/juju/juju/worker/machineactions/mocks"
)

type WorkerSuite struct {
	testing.IsolationSuite

	facade *mocks.MockFacade
	lock   *mocks.MockLock
}

var _ = gc.Suite(&WorkerSuite{})

func (*WorkerSuite) TestInvalidFacade(c *gc.C) {
	worker, err := machineactions.NewMachineActionsWorker(machineactions.WorkerConfig{
		Facade: nil,
	})
	c.Assert(err, gc.ErrorMatches, "nil Facade not valid")
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(worker, gc.IsNil)
}

func (s *WorkerSuite) TestInvalidMachineTag(c *gc.C) {
	worker, err := machineactions.NewMachineActionsWorker(machineactions.WorkerConfig{
		Facade:     s.facade,
		MachineTag: names.MachineTag{},
	})
	c.Assert(err, gc.ErrorMatches, "unspecified MachineTag not valid")
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(worker, gc.IsNil)
}

func (s *WorkerSuite) TestInvalidHandleAction(c *gc.C) {
	worker, err := machineactions.NewMachineActionsWorker(machineactions.WorkerConfig{
		Facade:       s.facade,
		MachineTag:   fakeTag,
		MachineLock:  s.lock,
		HandleAction: nil,
	})
	c.Assert(err, gc.ErrorMatches, "nil HandleAction not valid")
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(worker, gc.IsNil)
}

func defaultConfig(stub *testing.Stub, facade machineactions.Facade, lock machinelock.Lock) machineactions.WorkerConfig {
	return machineactions.WorkerConfig{
		Facade:       facade,
		MachineTag:   fakeTag,
		HandleAction: mockHandleAction(stub),
		MachineLock:  lock,
	}
}

func (s *WorkerSuite) TestRunningActionsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.facade.EXPECT().RunningActions(fakeTag).Return(nil, errors.New("splash"))

	stub := &testing.Stub{}
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub, s.facade, s.lock))
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "splash")
	stub.CheckNoCalls(c)
}

func (s *WorkerSuite) TestInvalidActionId(c *gc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 1)
	changes <- []string{"invalid-action-id"}

	s.facade.EXPECT().RunningActions(fakeTag).Return([]params.ActionResult{}, nil)
	s.facade.EXPECT().WatchActionNotifications(fakeTag).Return(&stubWatcher{
		Worker:  workertest.NewErrorWorker(nil),
		changes: changes}, nil)

	stub := &testing.Stub{}
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub, s.facade, s.lock))
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "got invalid action id invalid-action-id")
	stub.CheckNoCalls(c)
}

func (s *WorkerSuite) TestWatchErrorNonEmptyRunningActions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.facade.EXPECT().RunningActions(fakeTag).Return(fakeRunningActions, nil)
	s.facade.EXPECT().ActionFinish(names.NewActionTag("3"), params.ActionFailed, nil, "action cancelled").Return(nil)
	s.facade.EXPECT().WatchActionNotifications(fakeTag).Return(nil, errors.New("kuso"))

	stub := &testing.Stub{}
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub, s.facade, s.lock))
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Check(errors.Cause(err), gc.ErrorMatches, "kuso")
	stub.CheckNoCalls(c)
}

func (s *WorkerSuite) TestCannotRetrieveAction(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.facade.EXPECT().RunningActions(fakeTag).Return([]params.ActionResult{}, nil)
	s.facade.EXPECT().WatchActionNotifications(fakeTag).Return(newStubWatcher(false), nil)
	s.facade.EXPECT().Action(names.NewActionTag("1")).Times(1).Return(firstAction, nil)
	s.facade.EXPECT().Action(names.NewActionTag("2")).Times(1).Return(nil, errors.New("zbosh"))
	s.facade.EXPECT().ActionBegin(names.NewActionTag("1")).Return(nil)
	s.facade.EXPECT().ActionFinish(names.NewActionTag("1"), params.ActionCompleted, nil, "").Return(nil)

	stub := &testing.Stub{}
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub, s.facade, s.lock))
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Check(errors.Cause(err), gc.ErrorMatches, "zbosh")
	stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "HandleAction",
		Args:     []interface{}{firstAction.Name()},
	}})
}

func (s *WorkerSuite) TestRunActions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	released := false
	s.facade.EXPECT().RunningActions(fakeTag).Return([]params.ActionResult{}, nil)
	s.facade.EXPECT().WatchActionNotifications(fakeTag).Return(newStubWatcher(true), nil)
	s.facade.EXPECT().Action(names.NewActionTag("1")).Times(1).Return(firstAction, nil)
	s.facade.EXPECT().Action(names.NewActionTag("2")).Times(1).Return(secondAction, nil)
	s.facade.EXPECT().Action(names.NewActionTag("3")).Times(1).Return(thirdAction, nil)

	// Action 1 cannot start.
	begin1 := s.facade.EXPECT().ActionBegin(names.NewActionTag("1")).Times(1).Return(errors.New("kermack"))
	s.facade.EXPECT().ActionFinish(names.NewActionTag("1"), params.ActionFailed, nil, "could not begin action foo: kermack").After(begin1).Return(nil)

	// Action 2 does not run in parallel, so needs the lock.
	acquire2 := s.lock.EXPECT().Acquire(gomock.Any()).Times(1).Return(func() {
		released = true
	}, nil)
	begin2 := s.facade.EXPECT().ActionBegin(names.NewActionTag("2")).After(acquire2).Return(nil)
	s.facade.EXPECT().ActionFinish(names.NewActionTag("2"), params.ActionCompleted, nil, "").After(begin2).Return(nil)
	begin3 := s.facade.EXPECT().ActionBegin(names.NewActionTag("3")).Times(1).Return(nil)
	s.facade.EXPECT().ActionFinish(names.NewActionTag("3"), params.ActionCompleted, nil, "").After(begin3).Return(nil)

	stub := &testing.Stub{}
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub, s.facade, s.lock))
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Check(errors.Cause(err), gc.ErrorMatches, "got invalid action id invalid-action-id")
	stub.CheckCallsUnordered(c, []testing.StubCall{{
		FuncName: "HandleAction",
		Args:     []interface{}{secondAction.Name()},
	}, {
		FuncName: "HandleAction",
		Args:     []interface{}{thirdAction.Name()},
	}})
	c.Assert(released, jc.IsTrue)
}

func (s *WorkerSuite) TestActionHandleErr(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.facade.EXPECT().RunningActions(fakeTag).Return([]params.ActionResult{}, nil)
	s.facade.EXPECT().WatchActionNotifications(fakeTag).Return(newStubWatcher(false), nil)
	s.facade.EXPECT().Action(names.NewActionTag("1")).Times(1).Return(firstAction, nil)
	s.facade.EXPECT().Action(names.NewActionTag("2")).Times(1).Return(nil, errors.New("boom"))
	s.facade.EXPECT().ActionBegin(names.NewActionTag("1")).Times(1).Return(nil)
	s.facade.EXPECT().ActionFinish(names.NewActionTag("1"), params.ActionFailed, nil, "slob").Return(nil)

	stub := &testing.Stub{}
	stub.SetErrors(errors.New("slob"))
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub, s.facade, s.lock))
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Check(errors.Cause(err), gc.ErrorMatches, "boom")
	stub.CheckCallsUnordered(c, []testing.StubCall{{
		FuncName: "HandleAction",
		Args:     []interface{}{firstAction.Name()},
	}})
}

func (s *WorkerSuite) TestWorkerNoErr(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.facade.EXPECT().RunningActions(fakeTag).Return([]params.ActionResult{}, nil)
	s.facade.EXPECT().WatchActionNotifications(fakeTag).Return(&stubWatcher{
		Worker: workertest.NewErrorWorker(nil),
	}, nil)

	stub := &testing.Stub{}
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub, s.facade, s.lock))
	c.Assert(err, jc.ErrorIsNil)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
	stub.CheckNoCalls(c)
}

func (s *WorkerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.facade = mocks.NewMockFacade(ctrl)
	s.lock = mocks.NewMockLock(ctrl)
	return ctrl
}
