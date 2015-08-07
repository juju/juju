// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	utilexec "github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/runner"
)

type RunCommandsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RunCommandsSuite{})

func (s *RunCommandsSuite) TestPrepareError(c *gc.C) {
	runnerFactory := &MockRunnerFactory{
		MockNewCommandRunner: &MockNewCommandRunner{err: errors.New("blooey")},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
	})
	sendResponse := func(*utilexec.ExecResponse, error) { panic("not expected") }
	op, err := factory.NewCommands(someCommandArgs, sendResponse)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, gc.ErrorMatches, "blooey")
	c.Assert(newState, gc.IsNil)
	c.Assert(*runnerFactory.MockNewCommandRunner.gotInfo, gc.Equals, runner.CommandInfo{
		RelationId:      123,
		RemoteUnitName:  "foo/456",
		ForceRemoteUnit: true,
	})
}

func (s *RunCommandsSuite) TestPrepareSuccess(c *gc.C) {
	ctx := &MockContext{}
	runnerFactory := &MockRunnerFactory{
		MockNewCommandRunner: &MockNewCommandRunner{
			runner: &MockRunner{
				context: ctx,
			},
		},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
	})
	sendResponse := func(*utilexec.ExecResponse, error) { panic("not expected") }
	op, err := factory.NewCommands(someCommandArgs, sendResponse)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.IsNil)
	c.Assert(*runnerFactory.MockNewCommandRunner.gotInfo, gc.Equals, runner.CommandInfo{
		RelationId:      123,
		RemoteUnitName:  "foo/456",
		ForceRemoteUnit: true,
	})
	ctx.CheckCall(c, 0, "Prepare")
}

func (s *RunCommandsSuite) TestPrepareCtxError(c *gc.C) {
	ctx := &MockContext{}
	ctx.SetErrors(errors.New("ctx prepare error"))
	runnerFactory := &MockRunnerFactory{
		MockNewCommandRunner: &MockNewCommandRunner{
			runner: &MockRunner{
				context: ctx,
			},
		},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
	})
	sendResponse := func(*utilexec.ExecResponse, error) { panic("not expected") }
	op, err := factory.NewCommands(someCommandArgs, sendResponse)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, gc.ErrorMatches, "ctx prepare error")
	c.Assert(newState, gc.IsNil)
	ctx.CheckCall(c, 0, "Prepare")
}

func (s *RunCommandsSuite) TestExecuteRebootErrors(c *gc.C) {
	for _, sendErr := range []error{runner.ErrRequeueAndReboot, runner.ErrReboot} {
		runnerFactory := NewRunCommandsRunnerFactory(
			&utilexec.ExecResponse{Code: 101}, sendErr,
		)
		callbacks := &RunCommandsCallbacks{}
		factory := operation.NewFactory(operation.FactoryParams{
			RunnerFactory: runnerFactory,
			Callbacks:     callbacks,
		})
		sendResponse := &MockSendResponse{}
		op, err := factory.NewCommands(someCommandArgs, sendResponse.Call)
		c.Assert(err, jc.ErrorIsNil)
		_, err = op.Prepare(operation.State{})
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Execute(operation.State{})
		c.Assert(newState, gc.IsNil)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(*runnerFactory.MockNewCommandRunner.runner.MockRunCommands.gotCommands, gc.Equals, "do something")
		c.Assert(*sendResponse.gotResponse, gc.DeepEquals, &utilexec.ExecResponse{Code: 101})
		c.Assert(*sendResponse.gotErr, gc.Equals, operation.ErrNeedsReboot)
	}
}

func (s *RunCommandsSuite) TestExecuteOtherError(c *gc.C) {
	runnerFactory := NewRunCommandsRunnerFactory(
		nil, errors.New("sneh"),
	)
	callbacks := &RunCommandsCallbacks{}
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
		Callbacks:     callbacks,
	})
	sendResponse := &MockSendResponse{}
	op, err := factory.NewCommands(someCommandArgs, sendResponse.Call)
	c.Assert(err, jc.ErrorIsNil)
	_, err = op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*runnerFactory.MockNewCommandRunner.runner.MockRunCommands.gotCommands, gc.Equals, "do something")
	c.Assert(*sendResponse.gotResponse, gc.IsNil)
	c.Assert(*sendResponse.gotErr, gc.ErrorMatches, "sneh")
}

func (s *RunCommandsSuite) TestExecuteSuccess(c *gc.C) {
	runnerFactory := NewRunCommandsRunnerFactory(
		&utilexec.ExecResponse{Code: 222}, nil,
	)
	callbacks := &RunCommandsCallbacks{}
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
		Callbacks:     callbacks,
	})
	sendResponse := &MockSendResponse{}
	op, err := factory.NewCommands(someCommandArgs, sendResponse.Call)
	c.Assert(err, jc.ErrorIsNil)
	_, err = op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*runnerFactory.MockNewCommandRunner.runner.MockRunCommands.gotCommands, gc.Equals, "do something")
	c.Assert(*sendResponse.gotResponse, gc.DeepEquals, &utilexec.ExecResponse{Code: 222})
	c.Assert(*sendResponse.gotErr, jc.ErrorIsNil)
}

func (s *RunCommandsSuite) TestCommit(c *gc.C) {
	factory := operation.NewFactory(operation.FactoryParams{})
	sendResponse := func(*utilexec.ExecResponse, error) { panic("not expected") }
	op, err := factory.NewCommands(someCommandArgs, sendResponse)
	c.Assert(err, jc.ErrorIsNil)
	newState, err := op.Commit(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RunCommandsSuite) TestNeedsGlobalMachineLock(c *gc.C) {
	factory := operation.NewFactory(operation.FactoryParams{})
	sendResponse := &MockSendResponse{}
	op, err := factory.NewCommands(someCommandArgs, sendResponse.Call)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.NeedsGlobalMachineLock(), jc.IsTrue)
}
