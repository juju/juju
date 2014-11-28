// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	utilexec "github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/context"
	"github.com/juju/juju/worker/uniter/operation"
)

type RunCommandsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RunCommandsSuite{})

func (s *RunCommandsSuite) TestPrepareError(c *gc.C) {
	contextFactory := &MockContextFactory{
		MockNewRunContext: &MockNewRunContext{err: errors.New("blooey")},
	}
	factory := operation.NewFactory(nil, contextFactory, nil, nil)
	sendResponse := func(*utilexec.ExecResponse, error) { panic("not expected") }
	op, err := factory.NewCommands("do something", 123, "foo/456", sendResponse)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, gc.ErrorMatches, "blooey")
	c.Assert(newState, gc.IsNil)
	c.Assert(*contextFactory.MockNewRunContext.gotRelationId, gc.Equals, 123)
	c.Assert(*contextFactory.MockNewRunContext.gotRemoteUnitName, gc.Equals, "foo/456")
}

func (s *RunCommandsSuite) TestPrepareSuccess(c *gc.C) {
	contextFactory := &MockContextFactory{
		MockNewRunContext: &MockNewRunContext{},
	}
	factory := operation.NewFactory(nil, contextFactory, nil, nil)
	sendResponse := func(*utilexec.ExecResponse, error) { panic("not expected") }
	op, err := factory.NewCommands("do something", 123, "foo/456", sendResponse)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.IsNil)
	c.Assert(*contextFactory.MockNewRunContext.gotRelationId, gc.Equals, 123)
	c.Assert(*contextFactory.MockNewRunContext.gotRemoteUnitName, gc.Equals, "foo/456")
}

func (s *RunCommandsSuite) TestExecuteLockError(c *gc.C) {
	contextFactory := &MockContextFactory{
		MockNewRunContext: &MockNewRunContext{},
	}
	callbacks := &RunCommandsCallbacks{
		MockAcquireExecutionLock: &MockAcquireExecutionLock{err: errors.New("sneh")},
	}
	factory := operation.NewFactory(nil, contextFactory, callbacks, nil)
	sendResponse := func(*utilexec.ExecResponse, error) { panic("not expected") }
	op, err := factory.NewCommands("do something", 123, "foo/456", sendResponse)
	c.Assert(err, jc.ErrorIsNil)
	_, err = op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "sneh")
	c.Assert(*callbacks.MockAcquireExecutionLock.gotMessage, gc.Equals, "run commands")
}

func (s *RunCommandsSuite) TestExecuteRebootErrors(c *gc.C) {
	for _, sendErr := range []error{context.ErrRequeueAndReboot, context.ErrReboot} {
		contextFactory := &MockContextFactory{
			MockNewRunContext: &MockNewRunContext{context: &MockContext{}},
		}
		callbacks := &RunCommandsCallbacks{
			MockAcquireExecutionLock: &MockAcquireExecutionLock{},
			MockGetRunner: NewCommandsRunnerGetter(
				&utilexec.ExecResponse{Code: 101}, sendErr,
			),
		}
		factory := operation.NewFactory(nil, contextFactory, callbacks, nil)
		sendResponse := &MockSendResponse{}
		op, err := factory.NewCommands("do something", 123, "foo/456", sendResponse.Call)
		c.Assert(err, jc.ErrorIsNil)
		_, err = op.Prepare(operation.State{})
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Execute(operation.State{})
		c.Assert(newState, gc.IsNil)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(*callbacks.MockAcquireExecutionLock.gotMessage, gc.Equals, "run commands")
		c.Assert(callbacks.MockAcquireExecutionLock.didUnlock, jc.IsTrue)
		c.Assert(*callbacks.MockGetRunner.gotContext, gc.Equals, contextFactory.MockNewRunContext.context)
		c.Assert(*sendResponse.gotResponse, gc.DeepEquals, &utilexec.ExecResponse{Code: 101})
		c.Assert(*sendResponse.gotErr, gc.Equals, operation.ErrNeedsReboot)
	}
}

func (s *RunCommandsSuite) TestExecuteOtherError(c *gc.C) {
	contextFactory := &MockContextFactory{
		MockNewRunContext: &MockNewRunContext{context: &MockContext{}},
	}
	callbacks := &RunCommandsCallbacks{
		MockAcquireExecutionLock: &MockAcquireExecutionLock{},
		MockGetRunner: NewCommandsRunnerGetter(
			nil, errors.New("sneh"),
		),
	}
	factory := operation.NewFactory(nil, contextFactory, callbacks, nil)
	sendResponse := &MockSendResponse{}
	op, err := factory.NewCommands("do something", 123, "foo/456", sendResponse.Call)
	c.Assert(err, jc.ErrorIsNil)
	_, err = op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*callbacks.MockAcquireExecutionLock.gotMessage, gc.Equals, "run commands")
	c.Assert(callbacks.MockAcquireExecutionLock.didUnlock, jc.IsTrue)
	c.Assert(*callbacks.MockGetRunner.gotContext, gc.Equals, contextFactory.MockNewRunContext.context)
	c.Assert(*sendResponse.gotResponse, gc.IsNil)
	c.Assert(*sendResponse.gotErr, gc.ErrorMatches, "sneh")
}

func (s *RunCommandsSuite) TestExecuteSuccess(c *gc.C) {
	contextFactory := &MockContextFactory{
		MockNewRunContext: &MockNewRunContext{context: &MockContext{}},
	}
	callbacks := &RunCommandsCallbacks{
		MockAcquireExecutionLock: &MockAcquireExecutionLock{},
		MockGetRunner: NewCommandsRunnerGetter(
			&utilexec.ExecResponse{Code: 222}, nil,
		),
	}
	factory := operation.NewFactory(nil, contextFactory, callbacks, nil)
	sendResponse := &MockSendResponse{}
	op, err := factory.NewCommands("do something", 123, "foo/456", sendResponse.Call)
	c.Assert(err, jc.ErrorIsNil)
	_, err = op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*callbacks.MockAcquireExecutionLock.gotMessage, gc.Equals, "run commands")
	c.Assert(callbacks.MockAcquireExecutionLock.didUnlock, jc.IsTrue)
	c.Assert(*callbacks.MockGetRunner.gotContext, gc.Equals, contextFactory.MockNewRunContext.context)
	c.Assert(*sendResponse.gotResponse, gc.DeepEquals, &utilexec.ExecResponse{Code: 222})
	c.Assert(*sendResponse.gotErr, jc.ErrorIsNil)
}

func (s *RunCommandsSuite) TestCommit(c *gc.C) {
	factory := operation.NewFactory(nil, nil, nil, nil)
	sendResponse := func(*utilexec.ExecResponse, error) { panic("not expected") }
	op, err := factory.NewCommands("do something", 123, "foo/456", sendResponse)
	c.Assert(err, jc.ErrorIsNil)
	newState, err := op.Commit(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)
}
