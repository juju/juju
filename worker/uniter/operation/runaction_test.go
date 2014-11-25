// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/context"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

type RunActionSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RunActionSuite{})

func (s *RunActionSuite) TestPrepareErrorBadActionAndFailSucceeds(c *gc.C) {
	errBadAction := context.NewBadActionError("some-action-id", "splat")
	contextFactory := &MockContextFactory{
		MockNewActionContext: &MockNewActionContext{err: errBadAction},
	}
	callbacks := &RunActionCallbacks{
		MockFailAction: &MockFailAction{err: errors.New("squelch")},
	}
	factory := operation.NewFactory(nil, contextFactory, callbacks, nil)
	op, err := factory.NewAction(someActionId)
	c.Assert(err, gc.IsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "squelch")
	c.Assert(*contextFactory.MockNewActionContext.gotActionId, gc.Equals, someActionId)
	c.Assert(*callbacks.MockFailAction.gotActionId, gc.Equals, someActionId)
	c.Assert(*callbacks.MockFailAction.gotMessage, gc.Equals, errBadAction.Error())
}

func (s *RunActionSuite) TestPrepareErrorBadActionAndFailErrors(c *gc.C) {
	errBadAction := context.NewBadActionError("some-action-id", "foof")
	contextFactory := &MockContextFactory{
		MockNewActionContext: &MockNewActionContext{err: errBadAction},
	}
	callbacks := &RunActionCallbacks{
		MockFailAction: &MockFailAction{},
	}
	factory := operation.NewFactory(nil, contextFactory, callbacks, nil)
	op, err := factory.NewAction(someActionId)
	c.Assert(err, gc.IsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)
	c.Assert(*contextFactory.MockNewActionContext.gotActionId, gc.Equals, someActionId)
	c.Assert(*callbacks.MockFailAction.gotActionId, gc.Equals, someActionId)
	c.Assert(*callbacks.MockFailAction.gotMessage, gc.Equals, errBadAction.Error())
}

func (s *RunActionSuite) TestPrepareErrorActionNotAvailable(c *gc.C) {
	contextFactory := &MockContextFactory{
		MockNewActionContext: &MockNewActionContext{err: context.ErrActionNotAvailable},
	}
	factory := operation.NewFactory(nil, contextFactory, nil, nil)
	op, err := factory.NewAction(someActionId)
	c.Assert(err, gc.IsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)
	c.Assert(*contextFactory.MockNewActionContext.gotActionId, gc.Equals, someActionId)
}

func (s *RunActionSuite) TestPrepareErrorOther(c *gc.C) {
	contextFactory := &MockContextFactory{
		MockNewActionContext: &MockNewActionContext{err: errors.New("foop")},
	}
	factory := operation.NewFactory(nil, contextFactory, nil, nil)
	op, err := factory.NewAction(someActionId)
	c.Assert(err, gc.IsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `cannot create context for action "foo_a_1": foop`)
	c.Assert(*contextFactory.MockNewActionContext.gotActionId, gc.Equals, someActionId)
}

func (s *RunActionSuite) TestPrepareSuccessCleanState(c *gc.C) {
	contextFactory := NewRunActionSuccessContextFactory()
	factory := operation.NewFactory(nil, contextFactory, nil, nil)
	op, err := factory.NewAction(someActionId)
	c.Assert(err, gc.IsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, gc.IsNil)
	c.Assert(newState, gc.NotNil)
	c.Assert(*newState, gc.DeepEquals, operation.State{
		Kind:     operation.RunAction,
		Step:     operation.Pending,
		ActionId: &someActionId,
	})
	c.Assert(*contextFactory.MockNewActionContext.gotActionId, gc.Equals, someActionId)
}

func (s *RunActionSuite) TestPrepareSuccessDirtyState(c *gc.C) {
	contextFactory := NewRunActionSuccessContextFactory()
	factory := operation.NewFactory(nil, contextFactory, nil, nil)
	op, err := factory.NewAction(someActionId)
	c.Assert(err, gc.IsNil)

	newState, err := op.Prepare(overwriteState)
	c.Assert(err, gc.IsNil)
	c.Assert(newState, gc.NotNil)
	c.Assert(*newState, gc.DeepEquals, operation.State{
		Kind:               operation.RunAction,
		Step:               operation.Pending,
		ActionId:           &someActionId,
		Started:            true,
		CollectMetricsTime: 1234567,
		Hook:               &hook.Info{Kind: hooks.Install},
	})
	c.Assert(*contextFactory.MockNewActionContext.gotActionId, gc.Equals, someActionId)
}

func (s *RunActionSuite) TestExecuteLockError(c *gc.C) {
	contextFactory := NewRunActionSuccessContextFactory()
	callbacks := &RunActionCallbacks{
		MockAcquireExecutionLock: &MockAcquireExecutionLock{err: errors.New("plonk")},
	}
	factory := operation.NewFactory(nil, contextFactory, callbacks, nil)
	op, err := factory.NewAction(someActionId)
	c.Assert(err, gc.IsNil)
	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.NotNil)
	c.Assert(err, gc.IsNil)

	newState, err = op.Execute(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "plonk")
	c.Assert(*callbacks.MockAcquireExecutionLock.gotMessage, gc.Equals, "running action some-action-name")
}

func (s *RunActionSuite) TestExecuteRunError(c *gc.C) {
	contextFactory := NewRunActionSuccessContextFactory()
	callbacks := &RunActionCallbacks{
		MockAcquireExecutionLock: &MockAcquireExecutionLock{},
		MockGetRunner:            NewActionRunnerGetter(errors.New("snargle")),
	}
	factory := operation.NewFactory(nil, contextFactory, callbacks, nil)
	op, err := factory.NewAction(someActionId)
	c.Assert(err, gc.IsNil)
	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.NotNil)
	c.Assert(err, gc.IsNil)

	newState, err = op.Execute(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `running action "some-action-name": snargle`)
	c.Assert(*callbacks.MockAcquireExecutionLock.gotMessage, gc.Equals, "running action some-action-name")
	c.Assert(callbacks.MockAcquireExecutionLock.didUnlock, jc.IsTrue)
	c.Assert(*callbacks.MockGetRunner.gotContext, gc.Equals, contextFactory.MockNewActionContext.context)
	c.Assert(*callbacks.MockGetRunner.runner.MockRunAction.gotName, gc.Equals, "some-action-name")
}

func (s *RunActionSuite) TestExecuteSuccess(c *gc.C) {
	var stateChangeTests = []struct {
		description string
		before      operation.State
		after       operation.State
	}{{
		description: "empty state",
		after: operation.State{
			Kind:     operation.RunAction,
			Step:     operation.Done,
			ActionId: &someActionId,
		},
	}, {
		description: "preserves appropriate fields",
		before:      overwriteState,
		after: operation.State{
			Kind:               operation.RunAction,
			Step:               operation.Done,
			ActionId:           &someActionId,
			Hook:               &hook.Info{Kind: hooks.Install},
			Started:            true,
			CollectMetricsTime: 1234567,
		},
	}}

	for i, test := range stateChangeTests {
		c.Logf("test %d: %s", i, test.description)
		contextFactory := NewRunActionSuccessContextFactory()
		callbacks := &RunActionCallbacks{
			MockAcquireExecutionLock: &MockAcquireExecutionLock{},
			MockGetRunner:            NewActionRunnerGetter(nil),
		}
		factory := operation.NewFactory(nil, contextFactory, callbacks, nil)
		op, err := factory.NewAction(someActionId)
		c.Assert(err, gc.IsNil)
		midState, err := op.Prepare(test.before)
		c.Assert(midState, gc.NotNil)
		c.Assert(err, gc.IsNil)

		newState, err := op.Execute(*midState)
		c.Assert(err, gc.IsNil)
		c.Assert(newState, gc.NotNil)
		c.Assert(*newState, gc.DeepEquals, test.after)
		c.Assert(*callbacks.MockAcquireExecutionLock.gotMessage, gc.Equals, "running action some-action-name")
		c.Assert(callbacks.MockAcquireExecutionLock.didUnlock, jc.IsTrue)
		c.Assert(*callbacks.MockGetRunner.gotContext, gc.Equals, contextFactory.MockNewActionContext.context)
		c.Assert(*callbacks.MockGetRunner.runner.MockRunAction.gotName, gc.Equals, "some-action-name")
	}
}

func (s *RunActionSuite) TestCommit(c *gc.C) {
	var stateChangeTests = []struct {
		description string
		before      operation.State
		after       operation.State
	}{{
		description: "empty state",
		after: operation.State{
			Kind: operation.Continue,
			Step: operation.Pending,
		},
	}, {
		description: "preserves appropriate fields",
		before:      overwriteState,
		after: operation.State{
			Kind:               operation.Continue,
			Step:               operation.Pending,
			Hook:               &hook.Info{Kind: hooks.Install},
			Started:            true,
			CollectMetricsTime: 1234567,
		},
	}}

	for i, test := range stateChangeTests {
		c.Logf("test %d: %s", i, test.description)
		factory := operation.NewFactory(nil, nil, nil, nil)
		op, err := factory.NewAction(someActionId)
		c.Assert(err, gc.IsNil)

		newState, err := op.Commit(test.before)
		c.Assert(err, gc.IsNil)
		c.Assert(*newState, gc.DeepEquals, test.after)
	}
}
