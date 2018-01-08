// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6/hooks"

	"github.com/juju/juju/worker/caasoperator/hook"
	"github.com/juju/juju/worker/caasoperator/operation"
	"github.com/juju/juju/worker/common/charmrunner"
)

type RunHookSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RunHookSuite{})

type newHook func(operation.Factory, hook.Info) (operation.Operation, error)

func (s *RunHookSuite) testPrepareHookError(
	c *gc.C, newHook newHook, expectClearResolvedFlag, expectSkip bool,
) {
	callbacks := &ExecuteHookCallbacks{
		MockPrepareHook: &MockPrepareHook{err: errors.New("pow")},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
	})
	op, err := newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Check(newState, gc.IsNil)
	if expectSkip {
		c.Check(err, gc.Equals, operation.ErrSkipExecute)
		c.Check(callbacks.MockPrepareHook.gotHook, gc.IsNil)
		return
	}
	c.Check(err, gc.ErrorMatches, "pow")
	c.Check(callbacks.MockPrepareHook.gotHook, gc.DeepEquals, &hook.Info{
		Kind: hooks.ConfigChanged,
	})
}

func (s *RunHookSuite) TestPrepareHookCtxCalled(c *gc.C) {
	ctx := &MockContext{}
	callbacks := &ExecuteHookCallbacks{
		MockPrepareHook: &MockPrepareHook{},
	}
	runnerFactory := &MockRunnerFactory{
		MockNewHookRunner: &MockNewHookRunner{
			runner: &MockRunner{
				context: ctx,
			},
		},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
		Callbacks:     callbacks,
	})

	op, err := factory.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Check(newState, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)

	ctx.CheckCall(c, 0, "Prepare")
}

func (s *RunHookSuite) TestPrepareHookCtxError(c *gc.C) {
	ctx := &MockContext{}
	ctx.SetErrors(errors.New("ctx prepare error"))
	callbacks := newExecuteHookCallbacks()
	callbacks.MockPrepareHook = &MockPrepareHook{}
	runnerFactory := &MockRunnerFactory{
		MockNewHookRunner: &MockNewHookRunner{
			runner: &MockRunner{
				context: ctx,
			},
		},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
		Callbacks:     callbacks,
	})

	op, err := factory.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Check(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `ctx prepare error`)

	ctx.CheckCall(c, 0, "Prepare")
}

func (s *RunHookSuite) TestPrepareHookError_Run(c *gc.C) {
	s.testPrepareHookError(c, (operation.Factory).NewRunHook, false, false)
}

func (s *RunHookSuite) TestPrepareHookError_Skip(c *gc.C) {
	s.testPrepareHookError(c, (operation.Factory).NewSkipHook, true, true)
}

func (s *RunHookSuite) testPrepareRunnerError(c *gc.C, newHook newHook) {
	callbacks := newExecuteHookCallbacks()
	runnerFactory := &MockRunnerFactory{
		MockNewHookRunner: &MockNewHookRunner{err: errors.New("splat")},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
		Callbacks:     callbacks,
	})
	op, err := newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Check(newState, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "splat")
	c.Check(runnerFactory.MockNewHookRunner.gotHook, gc.DeepEquals, &hook.Info{
		Kind: hooks.ConfigChanged,
	})
}

func (s *RunHookSuite) TestPrepareRunnerError_Run(c *gc.C) {
	s.testPrepareRunnerError(c, (operation.Factory).NewRunHook)
}

func (s *RunHookSuite) testPrepareSuccess(
	c *gc.C, newHook newHook, before, after operation.State,
) {
	runnerFactory := NewRunHookRunnerFactory(errors.New("should not call"))
	callbacks := newExecuteHookCallbacks()
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
		Callbacks:     callbacks,
	})
	op, err := newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(before)
	c.Check(err, jc.ErrorIsNil)
	c.Check(newState, gc.DeepEquals, &after)
}

func (s *RunHookSuite) TestPrepareSuccess_BlankSlate(c *gc.C) {
	s.testPrepareSuccess(c,
		(operation.Factory).NewRunHook,
		operation.State{},
		operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		},
	)
}

func (s *RunHookSuite) TestPrepareSuccess_Preserve(c *gc.C) {
	s.testPrepareSuccess(c,
		(operation.Factory).NewRunHook,
		overwriteState,
		operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		},
	)
}

func (s *RunHookSuite) getExecuteRunnerTest(c *gc.C, newHook newHook, kind hooks.Kind, runErr error) (operation.Operation, *ExecuteHookCallbacks, *MockRunnerFactory) {
	runnerFactory := NewRunHookRunnerFactory(runErr)
	callbacks := newExecuteHookCallbacks()
	factory := operation.NewFactory(operation.FactoryParams{
		RunnerFactory: runnerFactory,
		Callbacks:     callbacks,
	})
	op, err := newHook(factory, hook.Info{Kind: kind})
	c.Assert(err, jc.ErrorIsNil)
	return op, callbacks, runnerFactory
}

func (s *RunHookSuite) TestExecuteMissingHookError(c *gc.C) {
	runErr := charmrunner.NewMissingHookError("blah-blah")
	for _, kind := range unitHooks() {
		c.Logf("hook %v", kind)
		op, _, runnerFactory := s.getExecuteRunnerTest(c, (operation.Factory).NewRunHook, kind, runErr)
		_, err := op.Prepare(operation.State{})
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Execute(operation.State{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(newState, gc.DeepEquals, &operation.State{
			Kind: operation.RunHook,
			Step: operation.Done,
			Hook: &hook.Info{Kind: kind},
		})
		c.Assert(*runnerFactory.MockNewHookRunner.runner.MockRunHook.gotName, gc.Equals, "some-hook-name")
	}
}

func (s *RunHookSuite) TestExecuteOtherError(c *gc.C) {
	runErr := errors.New("graaargh")
	op, _, runnerFactory := s.getExecuteRunnerTest(c, (operation.Factory).NewRunHook, hooks.ConfigChanged, runErr)
	_, err := op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(operation.State{})
	c.Assert(err, gc.Equals, operation.ErrHookFailed)
	c.Assert(newState, gc.IsNil)
	c.Assert(*runnerFactory.MockNewHookRunner.runner.MockRunHook.gotName, gc.Equals, "some-hook-name")
}

func (s *RunHookSuite) testExecuteSuccess(c *gc.C, before, after operation.State) {
	op, callbacks, _ := s.getExecuteRunnerTest(c, (operation.Factory).NewRunHook, hooks.ConfigChanged, nil)
	midState, err := op.Prepare(before)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(midState, gc.NotNil)

	newState, err := op.Execute(*midState)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.DeepEquals, &after)
	c.Check(callbacks.executingMessage, gc.Equals, "running some-hook-name hook")
}

func (s *RunHookSuite) TestExecuteSuccess_BlankSlate(c *gc.C) {
	s.testExecuteSuccess(c,
		operation.State{},
		operation.State{
			Kind: operation.RunHook,
			Step: operation.Done,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		},
	)
}

func (s *RunHookSuite) TestExecuteSuccess_Preserve(c *gc.C) {
	s.testExecuteSuccess(c,
		overwriteState,
		operation.State{
			Kind: operation.RunHook,
			Step: operation.Done,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		},
	)
}

func (s *RunHookSuite) testBeforeHookExecute(c *gc.C, newHook newHook, kind hooks.Kind) {
	// To check what happens in the beforeHook() call, we run a hook with an error
	// so that it does not complete successfully and thus afterHook() does not run,
	// overwriting the values.
	runErr := errors.New("graaargh")
	op, _, _ := s.getExecuteRunnerTest(c, newHook, kind, runErr)
	_, err := op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(operation.State{})
	c.Assert(err, gc.Equals, operation.ErrHookFailed)
	c.Assert(newState, gc.IsNil)
}

func (s *RunHookSuite) TestBeforeHookStatus(c *gc.C) {
	for _, kind := range unitHooks() {
		c.Logf("hook %v", kind)
		s.testBeforeHookExecute(c, (operation.Factory).NewRunHook, kind)
	}
}

func (s *RunHookSuite) testCommitError(c *gc.C, newHook newHook) {
	callbacks := &CommitHookCallbacks{
		MockCommitHook: &MockCommitHook{nil, errors.New("pow")},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
	})
	op, err := newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Commit(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "pow")
}

func (s *RunHookSuite) TestCommitError_Run(c *gc.C) {
	s.testCommitError(c, (operation.Factory).NewRunHook)
}

func (s *RunHookSuite) TestCommitError_Skip(c *gc.C) {
	s.testCommitError(c, (operation.Factory).NewSkipHook)
}

func (s *RunHookSuite) testCommitSuccess(c *gc.C, newHook newHook, hookInfo hook.Info, before, after operation.State) {
	callbacks := &CommitHookCallbacks{
		MockCommitHook: &MockCommitHook{},
	}
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
	})
	op, err := newHook(factory, hookInfo)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Commit(before)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.DeepEquals, &after)
}

func (s *RunHookSuite) TestCommitSuccess_ConfigChanged_QueueStartHook(c *gc.C) {
	for i, newHook := range []newHook{
		(operation.Factory).NewRunHook,
		(operation.Factory).NewSkipHook,
	} {
		c.Logf("variant %d", i)
		s.testCommitSuccess(c,
			newHook,
			hook.Info{Kind: hooks.ConfigChanged},
			operation.State{},
			operation.State{
				Kind: operation.Continue,
				Step: operation.Pending,
			},
		)
	}
}

func (s *RunHookSuite) TestCommitSuccess_ConfigChanged_Preserve(c *gc.C) {
	for i, newHook := range []newHook{
		(operation.Factory).NewRunHook,
		(operation.Factory).NewSkipHook,
	} {
		c.Logf("variant %d", i)
		s.testCommitSuccess(c,
			newHook,
			hook.Info{Kind: hooks.ConfigChanged},
			overwriteState,
			operation.State{
				Kind: operation.Continue,
				Step: operation.Pending,
			},
		)
	}
}

func (s *RunHookSuite) testQueueHook_BlankSlate(c *gc.C, cause hooks.Kind) {
	for i, newHook := range []newHook{
		(operation.Factory).NewRunHook,
		(operation.Factory).NewSkipHook,
	} {
		c.Logf("variant %d", i)
		var hi *hook.Info
		switch cause {
		case hooks.UpgradeCharm:
			hi = &hook.Info{Kind: hooks.ConfigChanged}
		default:
			hi = nil
		}
		s.testCommitSuccess(c,
			newHook,
			hook.Info{Kind: cause},
			operation.State{},
			operation.State{
				Kind: operation.RunHook,
				Step: operation.Queued,
				Hook: hi,
			},
		)
	}
}

func (s *RunHookSuite) testQueueHook_Preserve(c *gc.C, cause hooks.Kind) {
	for i, newHook := range []newHook{
		(operation.Factory).NewRunHook,
		(operation.Factory).NewSkipHook,
	} {
		c.Logf("variant %d", i)
		s.testCommitSuccess(c,
			newHook,
			hook.Info{Kind: cause},
			overwriteState,
			operation.State{
				Kind: operation.RunHook,
				Step: operation.Queued,
			},
		)
	}
}

func (s *RunHookSuite) testQueueNothing_BlankSlate(c *gc.C, hookInfo hook.Info) {
	for i, newHook := range []newHook{
		(operation.Factory).NewRunHook,
		(operation.Factory).NewSkipHook,
	} {
		c.Logf("variant %d", i)
		s.testCommitSuccess(c,
			newHook,
			hookInfo,
			operation.State{},
			operation.State{
				Kind: operation.Continue,
				Step: operation.Pending,
			},
		)
	}
}

func (s *RunHookSuite) testQueueNothing_Preserve(c *gc.C, hookInfo hook.Info) {
	for i, newHook := range []newHook{
		(operation.Factory).NewRunHook,
		(operation.Factory).NewSkipHook,
	} {
		c.Logf("variant %d", i)
		s.testCommitSuccess(c,
			newHook,
			hookInfo,
			overwriteState,
			operation.State{
				Kind: operation.Continue,
				Step: operation.Pending,
			},
		)
	}
}

func (s *RunHookSuite) TestQueueNothing_RelationChanged_BlankSlate(c *gc.C) {
	s.testQueueNothing_BlankSlate(c, hook.Info{
		Kind:       hooks.RelationChanged,
		RemoteUnit: "u/0",
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationChanged_Preserve(c *gc.C) {
	s.testQueueNothing_Preserve(c, hook.Info{
		Kind:       hooks.RelationChanged,
		RemoteUnit: "u/0",
	})
}
