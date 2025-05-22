// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	utilexec "github.com/juju/utils/v4/exec"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
)

type RunCommandsSuite struct {
	testhelpers.IsolationSuite
}

func TestRunCommandsSuite(t *testing.T) {
	tc.Run(t, &RunCommandsSuite{})
}

func (s *RunCommandsSuite) TestPrepareError(c *tc.C) {
	runnerFactory := &MockRunnerFactory{
		MockNewCommandRunner: &MockNewCommandRunner{err: errors.New("blooey")},
	}
	factory := newOpFactory(c, runnerFactory, nil)
	sendResponse := func(*utilexec.ExecResponse, error) bool { panic("not expected") }
	op, err := factory.NewCommands(someCommandArgs, sendResponse)
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorMatches, "blooey")
	c.Assert(newState, tc.IsNil)
	c.Assert(*runnerFactory.MockNewCommandRunner.gotInfo, tc.Equals, context.CommandInfo{
		RelationId:      123,
		RemoteUnitName:  "foo/456",
		ForceRemoteUnit: true,
	})
}

func (s *RunCommandsSuite) TestPrepareSuccess(c *tc.C) {
	ctx := &MockContext{}
	runnerFactory := &MockRunnerFactory{
		MockNewCommandRunner: &MockNewCommandRunner{
			runner: &MockRunner{
				context: ctx,
			},
		},
	}
	factory := newOpFactory(c, runnerFactory, nil)
	sendResponse := func(*utilexec.ExecResponse, error) bool { panic("not expected") }
	op, err := factory.NewCommands(someCommandArgs, sendResponse)
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newState, tc.IsNil)
	c.Assert(*runnerFactory.MockNewCommandRunner.gotInfo, tc.Equals, context.CommandInfo{
		RelationId:      123,
		RemoteUnitName:  "foo/456",
		ForceRemoteUnit: true,
	})
	ctx.CheckCall(c, 0, "Prepare")
}

func (s *RunCommandsSuite) TestPrepareCtxError(c *tc.C) {
	ctx := &MockContext{}
	ctx.SetErrors(errors.New("ctx prepare error"))
	runnerFactory := &MockRunnerFactory{
		MockNewCommandRunner: &MockNewCommandRunner{
			runner: &MockRunner{
				context: ctx,
			},
		},
	}
	factory := newOpFactory(c, runnerFactory, nil)
	sendResponse := func(*utilexec.ExecResponse, error) bool { panic("not expected") }
	op, err := factory.NewCommands(someCommandArgs, sendResponse)
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorMatches, "ctx prepare error")
	c.Assert(newState, tc.IsNil)
	ctx.CheckCall(c, 0, "Prepare")
}

func (s *RunCommandsSuite) TestExecuteRebootErrors(c *tc.C) {
	for _, sendErr := range []error{context.ErrRequeueAndReboot, context.ErrReboot} {
		runnerFactory := NewRunCommandsRunnerFactory(
			&utilexec.ExecResponse{Code: 101}, sendErr,
		)
		callbacks := &RunCommandsCallbacks{}
		factory := newOpFactory(c, runnerFactory, callbacks)
		sendResponse := &MockSendResponse{}
		op, err := factory.NewCommands(someCommandArgs, sendResponse.Call)
		c.Assert(err, tc.ErrorIsNil)
		_, err = op.Prepare(c.Context(), operation.State{})
		c.Assert(err, tc.ErrorIsNil)

		newState, err := op.Execute(c.Context(), operation.State{})
		c.Assert(newState, tc.IsNil)
		c.Assert(err, tc.Equals, operation.ErrNeedsReboot)
		c.Assert(*runnerFactory.MockNewCommandRunner.runner.MockRunCommands.gotCommands, tc.Equals, "do something")
		c.Assert(*sendResponse.gotResponse, tc.DeepEquals, &utilexec.ExecResponse{Code: 101})
		c.Assert(*sendResponse.gotErr, tc.ErrorIsNil)
	}
}

func (s *RunCommandsSuite) TestExecuteOtherError(c *tc.C) {
	runnerFactory := NewRunCommandsRunnerFactory(
		nil, errors.New("sneh"),
	)
	callbacks := &RunCommandsCallbacks{}
	factory := newOpFactory(c, runnerFactory, callbacks)
	sendResponse := &MockSendResponse{}
	op, err := factory.NewCommands(someCommandArgs, sendResponse.Call)
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Prepare(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Execute(c.Context(), operation.State{})
	c.Assert(newState, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "sneh")
	c.Assert(*runnerFactory.MockNewCommandRunner.runner.MockRunCommands.gotCommands, tc.Equals, "do something")
	c.Assert(*sendResponse.gotResponse, tc.IsNil)
	c.Assert(*sendResponse.gotErr, tc.ErrorMatches, "sneh")
}

func (s *RunCommandsSuite) TestExecuteConsumeOtherError(c *tc.C) {
	runnerFactory := NewRunCommandsRunnerFactory(
		nil, errors.New("sneh"),
	)
	callbacks := &RunCommandsCallbacks{}
	factory := newOpFactory(c, runnerFactory, callbacks)
	sendResponse := &MockSendResponse{
		eatError: true,
	}
	op, err := factory.NewCommands(someCommandArgs, sendResponse.Call)
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Prepare(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Execute(c.Context(), operation.State{})
	c.Assert(newState, tc.IsNil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*runnerFactory.MockNewCommandRunner.runner.MockRunCommands.gotCommands, tc.Equals, "do something")
	c.Assert(*sendResponse.gotResponse, tc.IsNil)
	c.Assert(*sendResponse.gotErr, tc.ErrorMatches, "sneh")
}

func (s *RunCommandsSuite) TestExecuteSuccess(c *tc.C) {
	runnerFactory := NewRunCommandsRunnerFactory(
		&utilexec.ExecResponse{Code: 222}, nil,
	)
	callbacks := &RunCommandsCallbacks{}
	factory := newOpFactory(c, runnerFactory, callbacks)
	sendResponse := &MockSendResponse{}
	op, err := factory.NewCommands(someCommandArgs, sendResponse.Call)
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Prepare(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Execute(c.Context(), operation.State{})
	c.Assert(newState, tc.IsNil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*runnerFactory.MockNewCommandRunner.runner.MockRunCommands.gotCommands, tc.Equals, "do something")
	c.Assert(*sendResponse.gotResponse, tc.DeepEquals, &utilexec.ExecResponse{Code: 222})
	c.Assert(*sendResponse.gotErr, tc.ErrorIsNil)
}

func (s *RunCommandsSuite) TestCommit(c *tc.C) {
	factory := newOpFactory(c, nil, nil)
	sendResponse := func(*utilexec.ExecResponse, error) bool { panic("not expected") }
	op, err := factory.NewCommands(someCommandArgs, sendResponse)
	c.Assert(err, tc.ErrorIsNil)
	newState, err := op.Commit(c.Context(), operation.State{})
	c.Assert(newState, tc.IsNil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *RunCommandsSuite) TestNeedsGlobalMachineLock(c *tc.C) {
	factory := newOpFactory(c, nil, nil)
	sendResponse := &MockSendResponse{}
	op, err := factory.NewCommands(someCommandArgs, sendResponse.Call)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.NeedsGlobalMachineLock(), tc.IsTrue)
}
