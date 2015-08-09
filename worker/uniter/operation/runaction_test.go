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
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
		Callbacks:     callbacks,
	})
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
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
		Callbacks:     callbacks,
	})
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
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
	})
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
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
	})
	op, err := factory.NewAction(someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `cannot create runner for action ".*": foop`)
	c.Assert(*runnerFactory.MockNewActionRunner.gotActionId, gc.Equals, someActionId)
}

func (s *RunActionSuite) TestPrepareCtxCalled(c *gc.C) {
	ctx := &MockContext{actionData: &runner.ActionData{Name: "some-action-name"}}
	runnerFactory := &MockRunnerFactory{
		MockNewActionRunner: &MockNewActionRunner{
			runner: &MockRunner{
				context: ctx,
			},
		},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
	})
	op, err := factory.NewAction(someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.NotNil)
	ctx.CheckCall(c, 0, "Prepare")
}

func (s *RunActionSuite) TestPrepareCtxError(c *gc.C) {
	ctx := &MockContext{actionData: &runner.ActionData{Name: "some-action-name"}}
	ctx.SetErrors(errors.New("ctx prepare error"))
	runnerFactory := &MockRunnerFactory{
		MockNewActionRunner: &MockNewActionRunner{
			runner: &MockRunner{
				context: ctx,
			},
		},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
	})
	op, err := factory.NewAction(someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, gc.ErrorMatches, `ctx prepare error`)
	c.Assert(newState, gc.IsNil)
	ctx.CheckCall(c, 0, "Prepare")
}

func (s *RunActionSuite) TestPrepareSuccessCleanState(c *gc.C) {
	runnerFactory := NewRunActionRunnerFactory(errors.New("should not call"))
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
	})
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
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
	})
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
		callbacks := &RunActionCallbacks{}
		factory := operation.NewFactory(operation.FactoryParams{
			RunnerFactory: runnerFactory,
			Callbacks:     callbacks,
		})
		op, err := factory.NewAction(someActionId)
		c.Assert(err, jc.ErrorIsNil)
		midState, err := op.Prepare(test.before)
		c.Assert(midState, gc.NotNil)
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Execute(*midState)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(newState, jc.DeepEquals, &test.after)
		c.Assert(callbacks.executingMessage, gc.Equals, "running action some-action-name")
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
		factory := operation.NewFactory(operation.FactoryParams{})
		op, err := factory.NewAction(someActionId)
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Commit(test.before)
		c.Assert(newState, jc.DeepEquals, &test.after)
	}
}

func (s *RunActionSuite) TestNeedsGlobalMachineLock(c *gc.C) {
	factory := operation.NewFactory(operation.FactoryParams{})
	op, err := factory.NewAction(someActionId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.NeedsGlobalMachineLock(), jc.IsTrue)
}
