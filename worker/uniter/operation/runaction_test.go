// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/runner"
)

type RunActionSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RunActionSuite{})

func (s *RunActionSuite) TestPrepareErrorBadActionAndFailSucceeds(c *gc.C) {
	errBadAction := runner.NewBadActionError("some-action-id", "splat")
	runnerFactory := &MockRunnerFactory{
		MockNewActionRunner: &MockNewActionRunner{err: errBadAction},
	}
	callbacks := &RunActionCallbacks{
		MockFailAction: &MockFailAction{err: errors.New("squelch")},
	}
	factory := operation.NewFactory(nil, runnerFactory, callbacks, nil, nil)
	op, err := factory.NewAction(someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "squelch")
	c.Assert(*runnerFactory.MockNewActionRunner.gotActionId, gc.Equals, someActionId)
	c.Assert(*callbacks.MockFailAction.gotActionId, gc.Equals, someActionId)
	c.Assert(*callbacks.MockFailAction.gotMessage, gc.Equals, errBadAction.Error())
}

func (s *RunActionSuite) TestPrepareErrorBadActionAndFailErrors(c *gc.C) {
	errBadAction := runner.NewBadActionError("some-action-id", "foof")
	runnerFactory := &MockRunnerFactory{
		MockNewActionRunner: &MockNewActionRunner{err: errBadAction},
	}
	callbacks := &RunActionCallbacks{
		MockFailAction: &MockFailAction{},
	}
	factory := operation.NewFactory(nil, runnerFactory, callbacks, nil, nil)
	op, err := factory.NewAction(someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)
	c.Assert(*runnerFactory.MockNewActionRunner.gotActionId, gc.Equals, someActionId)
	c.Assert(*callbacks.MockFailAction.gotActionId, gc.Equals, someActionId)
	c.Assert(*callbacks.MockFailAction.gotMessage, gc.Equals, errBadAction.Error())
}

func (s *RunActionSuite) TestPrepareErrorActionNotAvailable(c *gc.C) {
	runnerFactory := &MockRunnerFactory{
		MockNewActionRunner: &MockNewActionRunner{err: runner.ErrActionNotAvailable},
	}
	factory := operation.NewFactory(nil, runnerFactory, nil, nil, nil)
	op, err := factory.NewAction(someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)
	c.Assert(*runnerFactory.MockNewActionRunner.gotActionId, gc.Equals, someActionId)
}

func (s *RunActionSuite) TestPrepareErrorOther(c *gc.C) {
	runnerFactory := &MockRunnerFactory{
		MockNewActionRunner: &MockNewActionRunner{err: errors.New("foop")},
	}
	factory := operation.NewFactory(nil, runnerFactory, nil, nil, nil)
	op, err := factory.NewAction(someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `cannot create runner for action ".*": foop`)
	c.Assert(*runnerFactory.MockNewActionRunner.gotActionId, gc.Equals, someActionId)
}

func (s *RunActionSuite) TestPrepareSuccessCleanState(c *gc.C) {
	runnerFactory := NewRunActionRunnerFactory(errors.New("should not call"))
	factory := operation.NewFactory(nil, runnerFactory, nil, nil, nil)
	op, err := factory.NewAction(someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, jc.DeepEquals, &operation.State{
		Kind:     operation.RunAction,
		Step:     operation.Pending,
		ActionId: &someActionId,
	})
	c.Assert(*runnerFactory.MockNewActionRunner.gotActionId, gc.Equals, someActionId)
}

func (s *RunActionSuite) TestPrepareSuccessDirtyState(c *gc.C) {
	runnerFactory := NewRunActionRunnerFactory(errors.New("should not call"))
	factory := operation.NewFactory(nil, runnerFactory, nil, nil, nil)
	op, err := factory.NewAction(someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(overwriteState)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, jc.DeepEquals, &operation.State{
		Kind:               operation.RunAction,
		Step:               operation.Pending,
		ActionId:           &someActionId,
		Started:            true,
		CollectMetricsTime: 1234567,
		UpdateStatusTime:   1234567,
		Hook:               &hook.Info{Kind: hooks.Install},
	})
	c.Assert(*runnerFactory.MockNewActionRunner.gotActionId, gc.Equals, someActionId)
}

func (s *RunActionSuite) TestExecuteLockError(c *gc.C) {
	runnerFactory := NewRunActionRunnerFactory(errors.New("should not call"))
	callbacks := &RunActionCallbacks{
		MockAcquireExecutionLock: &MockAcquireExecutionLock{err: errors.New("plonk")},
	}
	factory := operation.NewFactory(nil, runnerFactory, callbacks, nil, nil)
	op, err := factory.NewAction(someActionId)
	c.Assert(err, jc.ErrorIsNil)
	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)

	newState, err = op.Execute(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "plonk")
	c.Assert(*callbacks.MockAcquireExecutionLock.gotMessage, gc.Equals, "running action some-action-name")
}

func (s *RunActionSuite) TestExecuteRunError(c *gc.C) {
	runnerFactory := NewRunActionRunnerFactory(errors.New("snargle"))
	callbacks := &RunActionCallbacks{
		MockAcquireExecutionLock: &MockAcquireExecutionLock{},
	}
	factory := operation.NewFactory(nil, runnerFactory, callbacks, nil, nil)
	op, err := factory.NewAction(someActionId)
	c.Assert(err, jc.ErrorIsNil)
	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)

	newState, err = op.Execute(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `running action "some-action-name": snargle`)
	c.Assert(*callbacks.MockAcquireExecutionLock.gotMessage, gc.Equals, "running action some-action-name")
	c.Assert(callbacks.MockAcquireExecutionLock.didUnlock, jc.IsTrue)
	c.Assert(*runnerFactory.MockNewActionRunner.runner.MockRunAction.gotName, gc.Equals, "some-action-name")
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
			UpdateStatusTime:   1234567,
		},
	}}

	for i, test := range stateChangeTests {
		c.Logf("test %d: %s", i, test.description)
		runnerFactory := NewRunActionRunnerFactory(nil)
		callbacks := &RunActionCallbacks{
			MockAcquireExecutionLock: &MockAcquireExecutionLock{},
		}
		factory := operation.NewFactory(nil, runnerFactory, callbacks, nil, nil)
		op, err := factory.NewAction(someActionId)
		c.Assert(err, jc.ErrorIsNil)
		midState, err := op.Prepare(test.before)
		c.Assert(midState, gc.NotNil)
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Execute(*midState)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(newState, jc.DeepEquals, &test.after)
		c.Assert(callbacks.executingMessage, gc.Equals, "running action some-action-name")
		c.Assert(*callbacks.MockAcquireExecutionLock.gotMessage, gc.Equals, "running action some-action-name")
		c.Assert(callbacks.MockAcquireExecutionLock.didUnlock, jc.IsTrue)
		c.Assert(*runnerFactory.MockNewActionRunner.runner.MockRunAction.gotName, gc.Equals, "some-action-name")
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
		description: "preserves only appropriate fields, no hook",
		before: operation.State{
			Kind:               operation.Continue,
			Step:               operation.Pending,
			Started:            true,
			CollectMetricsTime: 1234567,
			UpdateStatusTime:   1234567,
			CharmURL:           curl("cs:quantal/wordpress-2"),
			ActionId:           &randomActionId,
		},
		after: operation.State{
			Kind:               operation.Continue,
			Step:               operation.Pending,
			Started:            true,
			CollectMetricsTime: 1234567,
			UpdateStatusTime:   1234567,
		},
	}, {
		description: "preserves only appropriate fields, with hook",
		before: operation.State{
			Kind:               operation.Continue,
			Step:               operation.Pending,
			Started:            true,
			CollectMetricsTime: 1234567,
			UpdateStatusTime:   1234567,
			CharmURL:           curl("cs:quantal/wordpress-2"),
			ActionId:           &randomActionId,
			Hook:               &hook.Info{Kind: hooks.Install},
		},
		after: operation.State{
			Kind:               operation.RunHook,
			Step:               operation.Pending,
			Hook:               &hook.Info{Kind: hooks.Install},
			Started:            true,
			CollectMetricsTime: 1234567,
			UpdateStatusTime:   1234567,
		},
	}}

	for i, test := range stateChangeTests {
		c.Logf("test %d: %s", i, test.description)
		factory := operation.NewFactory(nil, nil, nil, nil, nil)
		op, err := factory.NewAction(someActionId)
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Commit(test.before)
		c.Assert(newState, jc.DeepEquals, &test.after)
	}
}
