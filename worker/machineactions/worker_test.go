// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/machineactions"
)

type WorkerSuite struct {
	testing.IsolationSuite
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

func (*WorkerSuite) TestInvalidMachineTag(c *gc.C) {
	worker, err := machineactions.NewMachineActionsWorker(machineactions.WorkerConfig{
		Facade:     &mockFacade{},
		MachineTag: names.MachineTag{},
	})
	c.Assert(err, gc.ErrorMatches, "unspecified MachineTag not valid")
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(worker, gc.IsNil)
}

func (*WorkerSuite) TestInvalidHandleAction(c *gc.C) {
	worker, err := machineactions.NewMachineActionsWorker(machineactions.WorkerConfig{
		Facade:       &mockFacade{},
		MachineTag:   fakeTag,
		HandleAction: nil,
	})
	c.Assert(err, gc.ErrorMatches, "nil HandleAction not valid")
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(worker, gc.IsNil)
}

func defaultConfig(stub *testing.Stub) machineactions.WorkerConfig {
	return machineactions.WorkerConfig{
		Facade:       &mockFacade{stub: stub},
		MachineTag:   fakeTag,
		HandleAction: mockHandleAction(stub),
	}
}

func (*WorkerSuite) TestRunningActionsError(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(errors.New("splash"))
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub))
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "splash")

	stub.CheckCalls(c, getSuccessfulCalls(1))
}

func (*WorkerSuite) TestInvalidActionId(c *gc.C) {
	stub := &testing.Stub{}
	facade := &mockFacade{
		stub:                     stub,
		watcherSendInvalidValues: true,
	}
	config := machineactions.WorkerConfig{
		Facade:       facade,
		MachineTag:   fakeTag,
		HandleAction: mockHandleAction(stub),
	}
	worker, err := machineactions.NewMachineActionsWorker(config)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "got invalid action id invalid-action-id")

	stub.CheckCalls(c, getSuccessfulCalls(allCalls))
}

func (*WorkerSuite) TestWatchErrorNonEmptyRunningActions(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(nil, errors.New("ignored"), errors.New("kuso"))
	facade := &mockFacade{
		stub:           stub,
		runningActions: fakeRunningActions,
	}
	config := machineactions.WorkerConfig{
		Facade:       facade,
		MachineTag:   fakeTag,
		HandleAction: mockHandleAction(stub),
	}
	worker, err := machineactions.NewMachineActionsWorker(config)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "kuso")

	stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "RunningActions",
		Args:     []interface{}{fakeTag},
	}, {
		FuncName: "ActionFinish",
		Args:     []interface{}{thirdActionTag, params.ActionFailed, "action cancelled"},
	}, {
		FuncName: "WatchActionNotifications",
		Args:     []interface{}{fakeTag},
	}})
}

func (*WorkerSuite) TestCannotRetrieveFirstAction(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(nil, nil, errors.New("zbosh"))
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub))
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Check(errors.Cause(err), gc.ErrorMatches, "zbosh")

	stub.CheckCalls(c, getSuccessfulCalls(3))
}

func (*WorkerSuite) TestCannotBeginAction(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(nil, nil, nil, errors.New("kermack"))
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub))
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Check(errors.Cause(err), gc.ErrorMatches, "kermack")

	stub.CheckCalls(c, getSuccessfulCalls(4))
}

func (*WorkerSuite) TestFirstActionHandleErrAndFinishErr(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(nil, nil, nil, nil, errors.New("sentToActionFinish"), errors.New("slob"))
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub))
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Check(errors.Cause(err), gc.ErrorMatches, "slob")

	successfulCalls := getSuccessfulCalls(6)
	successfulCalls[5].Args = []interface{}{firstActionTag, params.ActionFailed, "sentToActionFinish"}
	stub.CheckCalls(c, successfulCalls)
}

func (*WorkerSuite) TestFirstActionHandleErrButFinishErrCannotRetrieveSecond(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(nil, nil, nil, nil, errors.New("sentToActionFinish"), nil, errors.New("gotcha"))
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub))
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, worker)
	c.Check(errors.Cause(err), gc.ErrorMatches, "gotcha")

	successfulCalls := getSuccessfulCalls(7)
	successfulCalls[5].Args = []interface{}{firstActionTag, params.ActionFailed, "sentToActionFinish"}
	stub.CheckCalls(c, successfulCalls)
}

func (*WorkerSuite) TestFailHandlingSecondActionSendAllResults(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(nil, nil, nil, nil, nil, nil, nil, nil, errors.New("kryptonite"))
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub))
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)

	successfulCalls := getSuccessfulCalls(allCalls)
	successfulCalls[9].Args = []interface{}{secondActionTag, params.ActionFailed, "kryptonite"}
	stub.CheckCalls(c, successfulCalls)
}

func (*WorkerSuite) TestWorkerNoErr(c *gc.C) {
	stub := &testing.Stub{}
	worker, err := machineactions.NewMachineActionsWorker(defaultConfig(stub))
	c.Assert(err, jc.ErrorIsNil)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
	stub.CheckCalls(c, getSuccessfulCalls(allCalls))
}

const allCalls = 14

func getSuccessfulCalls(index int) []testing.StubCall {
	successfulCalls := []testing.StubCall{{
		FuncName: "RunningActions",
		Args:     []interface{}{fakeTag},
	}, {
		FuncName: "WatchActionNotifications",
		Args:     []interface{}{fakeTag},
	}, {
		FuncName: "Action",
		Args:     []interface{}{firstActionTag},
	}, {
		FuncName: "ActionBegin",
		Args:     []interface{}{firstActionTag},
	}, {
		FuncName: "HandleAction",
		Args:     []interface{}{firstAction.Name()},
	}, {
		FuncName: "ActionFinish",
		Args:     []interface{}{firstActionTag, params.ActionCompleted, ""},
	}, {
		FuncName: "Action",
		Args:     []interface{}{secondActionTag},
	}, {
		FuncName: "ActionBegin",
		Args:     []interface{}{secondActionTag},
	}, {
		FuncName: "HandleAction",
		Args:     []interface{}{secondAction.Name()},
	}, {
		FuncName: "ActionFinish",
		Args:     []interface{}{secondActionTag, params.ActionCompleted, ""},
	}, {
		FuncName: "Action",
		Args:     []interface{}{thirdActionTag},
	}, {
		FuncName: "ActionBegin",
		Args:     []interface{}{thirdActionTag},
	}, {
		FuncName: "HandleAction",
		Args:     []interface{}{thirdAction.Name()},
	}, {
		FuncName: "ActionFinish",
		Args:     []interface{}{thirdActionTag, params.ActionCompleted, ""},
	}}
	return successfulCalls[:index]
}
