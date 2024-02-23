// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	stdcontext "context"

	"github.com/juju/errors"
	"github.com/juju/juju/charm/hooks"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/runner"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type RunHookSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RunHookSuite{})

type newHook func(operation.Factory, hook.Info) (operation.Operation, error)

func (s *RunHookSuite) testPrepareHookError(
	c *gc.C, newHook newHook, expectSkip bool,
) {
	callbacks := &PrepareHookCallbacks{
		MockPrepareHook: &MockPrepareHook{err: errors.New("pow")},
	}
	factory := newOpFactory(nil, callbacks)
	op, err := newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{})
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
	callbacks := &PrepareHookCallbacks{
		MockPrepareHook: &MockPrepareHook{},
	}
	runnerFactory := &MockRunnerFactory{
		MockNewHookRunner: &MockNewHookRunner{
			runner: &MockRunner{
				context: ctx,
			},
		},
	}
	factory := newOpFactory(runnerFactory, callbacks)

	op, err := factory.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Check(newState, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)

	ctx.CheckCall(c, 0, "Prepare")
}

func (s *RunHookSuite) TestPrepareHookCtxError(c *gc.C) {
	ctx := &MockContext{}
	ctx.SetErrors(errors.New("ctx prepare error"))
	callbacks := &PrepareHookCallbacks{
		MockPrepareHook: &MockPrepareHook{},
	}
	runnerFactory := &MockRunnerFactory{
		MockNewHookRunner: &MockNewHookRunner{
			runner: &MockRunner{
				context: ctx,
			},
		},
	}
	factory := newOpFactory(runnerFactory, callbacks)

	op, err := factory.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Check(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `ctx prepare error`)

	ctx.CheckCall(c, 0, "Prepare")
}

func (s *RunHookSuite) TestPrepareHookError_Run(c *gc.C) {
	s.testPrepareHookError(c, operation.Factory.NewRunHook, false)
}

func (s *RunHookSuite) TestPrepareHookError_Skip(c *gc.C) {
	s.testPrepareHookError(c, operation.Factory.NewSkipHook, true)
}

func (s *RunHookSuite) TestPrepareHookError_LeaderElectedNotLeader(c *gc.C) {
	callbacks := &PrepareHookCallbacks{
		MockPrepareHook: &MockPrepareHook{nil, string(hooks.LeaderElected), nil},
	}
	runnerFactory := &MockRunnerFactory{
		MockNewHookRunner: &MockNewHookRunner{
			runner: &MockRunner{
				context: &MockContext{isLeader: false},
			},
		},
	}
	factory := newOpFactory(runnerFactory, callbacks)

	op, err := operation.Factory.NewRunHook(factory, hook.Info{Kind: hooks.LeaderElected})
	c.Assert(err, jc.ErrorIsNil)

	_, err = op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)
}

func (s *RunHookSuite) testPrepareRunnerError(c *gc.C, newHook newHook) {
	callbacks := NewPrepareHookCallbacks(hooks.ConfigChanged)
	runnerFactory := &MockRunnerFactory{
		MockNewHookRunner: &MockNewHookRunner{err: errors.New("splat")},
	}
	factory := newOpFactory(runnerFactory, callbacks)
	op, err := newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Check(newState, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "splat")
	c.Check(runnerFactory.MockNewHookRunner.gotHook, gc.DeepEquals, &hook.Info{
		Kind: hooks.ConfigChanged,
	})
}

func (s *RunHookSuite) TestPrepareRunnerError_Run(c *gc.C) {
	s.testPrepareRunnerError(c, operation.Factory.NewRunHook)
}

func (s *RunHookSuite) testPrepareSuccess(
	c *gc.C, newHook newHook, before, after operation.State,
) {
	runnerFactory := NewRunHookRunnerFactory(errors.New("should not call"))
	callbacks := NewPrepareHookCallbacks(hooks.ConfigChanged)
	factory := newOpFactory(runnerFactory, callbacks)
	op, err := newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), before)
	c.Check(err, jc.ErrorIsNil)
	c.Check(newState, gc.DeepEquals, &after)
}

func (s *RunHookSuite) TestPrepareSuccess_BlankSlate(c *gc.C) {
	s.testPrepareSuccess(c,
		operation.Factory.NewRunHook,
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
		operation.Factory.NewRunHook,
		overwriteState,
		operation.State{
			Started: true,
			Kind:    operation.RunHook,
			Step:    operation.Pending,
			Hook:    &hook.Info{Kind: hooks.ConfigChanged},
		},
	)
}

func (s *RunHookSuite) getExecuteRunnerTest(
	c *gc.C, newHook newHook, kind hooks.Kind, runErr error, contextOps ...func(*MockContext),
) (operation.Operation, *ExecuteHookCallbacks, *MockRunnerFactory) {
	runnerFactory := NewRunHookRunnerFactory(runErr, contextOps...)
	callbacks := &ExecuteHookCallbacks{
		PrepareHookCallbacks:    NewPrepareHookCallbacks(kind),
		MockNotifyHookCompleted: &MockNotify{},
		MockNotifyHookFailed:    &MockNotify{},
	}
	factory := newOpFactory(runnerFactory, callbacks)

	// Target is supplied for the special-cased pre-series-upgrade hook.
	// This is the only one of the designated unit hooks with validation.
	op, err := newHook(factory, hook.Info{Kind: kind, MachineUpgradeTarget: "ubuntu@20.04"})

	c.Assert(err, jc.ErrorIsNil)
	return op, callbacks, runnerFactory
}

func (s *RunHookSuite) TestExecuteMissingHookError(c *gc.C) {
	runErr := charmrunner.NewMissingHookError("blah-blah")
	for _, kind := range hooks.UnitHooks() {
		c.Logf("hook %v", kind)
		op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, kind, runErr)
		_, err := op.Prepare(stdcontext.Background(), operation.State{})
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Execute(stdcontext.Background(), operation.State{})
		c.Assert(err, jc.ErrorIsNil)

		s.assertStateMatches(c, newState, operation.RunHook, operation.Done, kind)

		c.Assert(*runnerFactory.MockNewHookRunner.runner.MockRunHook.gotName, gc.Equals, string(kind))
		c.Assert(callbacks.MockNotifyHookCompleted.gotName, gc.IsNil)
		c.Assert(callbacks.MockNotifyHookFailed.gotName, gc.IsNil)

		status, err := runnerFactory.MockNewHookRunner.runner.Context().UnitStatus(stdcontext.Background())
		c.Assert(err, jc.ErrorIsNil)
		testAfterHookStatus(c, kind, status, false)
	}
}

func (s *RunHookSuite) assertStateMatches(
	c *gc.C, st *operation.State, opKind operation.Kind, step operation.Step, hookKind hooks.Kind,
) {
	c.Assert(st.Kind, gc.Equals, opKind)
	c.Assert(st.Step, gc.Equals, step)
	c.Assert(st.Hook, gc.NotNil)
	c.Assert(st.Hook.Kind, gc.Equals, hookKind)
	if step == operation.Queued {
		c.Assert(st.HookStep, gc.NotNil)
		c.Assert(*st.HookStep, gc.Equals, step)
	}
}

func (s *RunHookSuite) TestExecuteRequeueRebootError(c *gc.C) {
	runErr := context.ErrRequeueAndReboot
	op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, hooks.ConfigChanged, runErr)
	_, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(stdcontext.Background(), operation.State{})
	c.Assert(err, gc.Equals, operation.ErrNeedsReboot)

	s.assertStateMatches(c, newState, operation.RunHook, operation.Queued, hooks.ConfigChanged)

	c.Assert(*runnerFactory.MockNewHookRunner.runner.MockRunHook.gotName, gc.Equals, "config-changed")
	c.Assert(*callbacks.MockNotifyHookCompleted.gotName, gc.Equals, "config-changed")
	c.Assert(*callbacks.MockNotifyHookCompleted.gotContext, gc.Equals, runnerFactory.MockNewHookRunner.runner.context)
	c.Assert(callbacks.MockNotifyHookFailed.gotName, gc.IsNil)
}

func (s *RunHookSuite) TestExecuteRebootError(c *gc.C) {
	runErr := context.ErrReboot
	op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, hooks.ConfigChanged, runErr)
	_, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(stdcontext.Background(), operation.State{})
	c.Assert(err, gc.Equals, operation.ErrNeedsReboot)

	s.assertStateMatches(c, newState, operation.RunHook, operation.Done, hooks.ConfigChanged)

	c.Assert(*runnerFactory.MockNewHookRunner.runner.MockRunHook.gotName, gc.Equals, "config-changed")
	c.Assert(*callbacks.MockNotifyHookCompleted.gotName, gc.Equals, "config-changed")
	c.Assert(*callbacks.MockNotifyHookCompleted.gotContext, gc.Equals, runnerFactory.MockNewHookRunner.runner.context)
	c.Assert(callbacks.MockNotifyHookFailed.gotName, gc.IsNil)
}

func (s *RunHookSuite) TestExecuteOtherError(c *gc.C) {
	runErr := errors.New("graaargh")
	op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, hooks.ConfigChanged, runErr)
	_, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(stdcontext.Background(), operation.State{})
	c.Assert(err, gc.Equals, operation.ErrHookFailed)
	c.Assert(newState, gc.IsNil)
	c.Assert(*runnerFactory.MockNewHookRunner.runner.MockRunHook.gotName, gc.Equals, "config-changed")
	c.Assert(*callbacks.MockNotifyHookFailed.gotName, gc.Equals, "config-changed")
	c.Assert(*callbacks.MockNotifyHookFailed.gotContext, gc.Equals, runnerFactory.MockNewHookRunner.runner.context)
	c.Assert(callbacks.MockNotifyHookCompleted.gotName, gc.IsNil)
}

func (s *RunHookSuite) TestExecuteTerminated(c *gc.C) {
	runErr := runner.ErrTerminated
	op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, hooks.ConfigChanged, runErr)
	_, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(stdcontext.Background(), operation.State{})
	c.Assert(err, gc.Equals, runner.ErrTerminated)

	s.assertStateMatches(c, newState, operation.RunHook, operation.Queued, hooks.ConfigChanged)

	c.Assert(*runnerFactory.MockNewHookRunner.runner.MockRunHook.gotName, gc.Equals, "config-changed")
	c.Assert(callbacks.MockNotifyHookCompleted.gotName, gc.IsNil)
}

func (s *RunHookSuite) TestInstallHookPreservesStatus(c *gc.C) {
	op, callbacks, f := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, hooks.Install, nil)
	err := f.MockNewHookRunner.runner.Context().SetUnitStatus(stdcontext.Background(), jujuc.StatusInfo{Status: "blocked", Info: "no database"})
	c.Assert(err, jc.ErrorIsNil)
	st := operation.State{
		StatusSet: true,
	}
	midState, err := op.Prepare(stdcontext.Background(), st)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(midState, gc.NotNil)

	_, err = op.Execute(stdcontext.Background(), *midState)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(callbacks.executingMessage, gc.Equals, "running install hook")
	status, err := f.MockNewHookRunner.runner.Context().UnitStatus(stdcontext.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Status, gc.Equals, "blocked")
	c.Assert(status.Info, gc.Equals, "no database")
}

func (s *RunHookSuite) TestInstallHookWHenNoStatusSet(c *gc.C) {
	op, callbacks, f := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, hooks.Install, nil)
	st := operation.State{
		StatusSet: false,
	}
	midState, err := op.Prepare(stdcontext.Background(), st)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(midState, gc.NotNil)

	_, err = op.Execute(stdcontext.Background(), *midState)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(callbacks.executingMessage, gc.Equals, "running install hook")
	status, err := f.MockNewHookRunner.runner.Context().UnitStatus(stdcontext.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Status, gc.Equals, "maintenance")
	c.Assert(status.Info, gc.Equals, "installing charm software")
}

func (s *RunHookSuite) testExecuteSuccess(
	c *gc.C, before, after operation.State, setStatusCalled bool,
) {
	op, callbacks, f := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, hooks.ConfigChanged, nil)
	f.MockNewHookRunner.runner.MockRunHook.setStatusCalled = setStatusCalled
	midState, err := op.Prepare(stdcontext.Background(), before)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(midState, gc.NotNil)

	newState, err := op.Execute(stdcontext.Background(), *midState)
	c.Assert(err, jc.ErrorIsNil)

	s.assertStateMatches(c, newState, after.Kind, after.Step, after.Hook.Kind)
	c.Assert(newState.Started, gc.Equals, after.Started)
	c.Assert(newState.StatusSet, gc.Equals, after.StatusSet)

	c.Check(callbacks.executingMessage, gc.Equals, "running config-changed hook")
}

func (s *RunHookSuite) TestExecuteSuccess_BlankSlate(c *gc.C) {
	s.testExecuteSuccess(c,
		operation.State{},
		operation.State{
			Kind:      operation.RunHook,
			Step:      operation.Done,
			Hook:      &hook.Info{Kind: hooks.ConfigChanged},
			StatusSet: true,
		},
		true,
	)
}

func (s *RunHookSuite) TestExecuteSuccess_Preserve(c *gc.C) {
	s.testExecuteSuccess(c,
		overwriteState,
		operation.State{
			Started:   true,
			Kind:      operation.RunHook,
			Step:      operation.Done,
			Hook:      &hook.Info{Kind: hooks.ConfigChanged},
			StatusSet: true,
		},
		true,
	)
}

func (s *RunHookSuite) testExecuteThenCharmStatus(
	c *gc.C, before, after operation.State, kind hooks.Kind, setStatusCalled bool,
) {
	op, _, f := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, kind, nil)
	f.MockNewHookRunner.runner.MockRunHook.setStatusCalled = setStatusCalled
	midState, err := op.Prepare(stdcontext.Background(), before)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(midState, gc.NotNil)

	_, err = f.MockNewHookRunner.runner.Context().UnitStatus(stdcontext.Background())
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(stdcontext.Background(), *midState)
	c.Assert(err, jc.ErrorIsNil)

	s.assertStateMatches(c, newState, after.Kind, after.Step, after.Hook.Kind)
	c.Assert(newState.Started, gc.Equals, after.Started)
	c.Assert(newState.StatusSet, gc.Equals, after.StatusSet)

	status, err := f.MockNewHookRunner.runner.Context().UnitStatus(stdcontext.Background())
	c.Assert(err, jc.ErrorIsNil)
	testAfterHookStatus(c, kind, status, after.StatusSet)
}

func testBeforeHookStatus(c *gc.C, kind hooks.Kind, status *jujuc.StatusInfo) {
	switch kind {
	case hooks.Install:
		c.Assert(status.Status, gc.Equals, "maintenance")
		c.Assert(status.Info, gc.Equals, "installing charm software")
	case hooks.Stop:
		c.Assert(status.Status, gc.Equals, "maintenance")
		c.Assert(status.Info, gc.Equals, "stopping charm software")
	case hooks.Remove:
		c.Assert(status.Status, gc.Equals, "maintenance")
		c.Assert(status.Info, gc.Equals, "cleaning up prior to charm deletion")
	default:
		c.Assert(status.Status, gc.Equals, "")
	}
}

func testAfterHookStatus(c *gc.C, kind hooks.Kind, status *jujuc.StatusInfo, statusSetCalled bool) {
	switch kind {
	case hooks.Install:
		c.Assert(status.Status, gc.Equals, "maintenance")
		c.Assert(status.Info, gc.Equals, "installing charm software")
	case hooks.Start:
		if statusSetCalled {
			c.Assert(status.Status, gc.Equals, "")
		} else {
			c.Assert(status.Status, gc.Equals, "unknown")
		}
	case hooks.Stop:
		c.Assert(status.Status, gc.Equals, "maintenance")
	case hooks.Remove:
		c.Assert(status.Status, gc.Equals, "terminated")
	default:
		c.Assert(status.Status, gc.Equals, "")
	}
}

func (s *RunHookSuite) testBeforeHookExecute(c *gc.C, newHook newHook, kind hooks.Kind) {
	// To check what happens in the beforeHook() call, we run a hook with an error
	// so that it does not complete successfully and thus afterHook() does not run,
	// overwriting the values.
	runErr := errors.New("graaargh")
	op, _, runnerFactory := s.getExecuteRunnerTest(c, newHook, kind, runErr)
	_, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(stdcontext.Background(), operation.State{})
	c.Assert(err, gc.Equals, operation.ErrHookFailed)
	c.Assert(newState, gc.IsNil)

	status, err := runnerFactory.MockNewHookRunner.runner.Context().UnitStatus(stdcontext.Background())
	c.Assert(err, jc.ErrorIsNil)
	testBeforeHookStatus(c, kind, status)
}

func (s *RunHookSuite) TestBeforeHookStatus(c *gc.C) {
	for _, kind := range hooks.UnitHooks() {
		c.Logf("hook %v", kind)
		s.testBeforeHookExecute(c, operation.Factory.NewRunHook, kind)
	}
}

func (s *RunHookSuite) testExecuteHookWithSetStatus(c *gc.C, kind hooks.Kind, setStatusCalled bool) {
	s.testExecuteThenCharmStatus(c,
		overwriteState,
		operation.State{
			Started:   true,
			Kind:      operation.RunHook,
			Step:      operation.Done,
			Hook:      &hook.Info{Kind: kind},
			StatusSet: setStatusCalled,
		},
		kind,
		setStatusCalled,
	)
}

func (s *RunHookSuite) TestExecuteHookWithSetStatus(c *gc.C) {
	for _, kind := range hooks.UnitHooks() {
		c.Logf("hook %v", kind)
		s.testExecuteHookWithSetStatus(c, kind, true)
		s.testExecuteHookWithSetStatus(c, kind, false)
	}
}

func (s *RunHookSuite) testCommitError(c *gc.C, newHook newHook) {
	callbacks := &CommitHookCallbacks{
		MockCommitHook: &MockCommitHook{nil, errors.New("pow")},
	}
	factory := newOpFactory(nil, callbacks)
	op, err := newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Commit(stdcontext.Background(), operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "committing hook.*: pow")
}

func (s *RunHookSuite) TestCommitError_Run(c *gc.C) {
	s.testCommitError(c, operation.Factory.NewRunHook)
}

func (s *RunHookSuite) TestCommitError_Skip(c *gc.C) {
	s.testCommitError(c, operation.Factory.NewSkipHook)
}

func (s *RunHookSuite) testCommitSuccess(c *gc.C, newHook newHook, hookInfo hook.Info, before, after operation.State) {
	callbacks := &CommitHookCallbacks{
		MockCommitHook: &MockCommitHook{},
	}
	factory := newOpFactory(nil, callbacks)
	op, err := newHook(factory, hookInfo)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Commit(stdcontext.Background(), before)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.DeepEquals, &after)
}

func (s *RunHookSuite) TestCommitSuccess_ConfigChanged_QueueStartHook(c *gc.C) {
	for i, newHook := range []newHook{
		operation.Factory.NewRunHook,
		operation.Factory.NewSkipHook,
	} {
		c.Logf("variant %d", i)
		s.testCommitSuccess(c,
			newHook,
			hook.Info{Kind: hooks.ConfigChanged},
			operation.State{},
			operation.State{
				Kind: operation.RunHook,
				Step: operation.Queued,
				Hook: &hook.Info{Kind: hooks.Start},
			},
		)
	}
}

func (s *RunHookSuite) TestCommitSuccess_ConfigChanged_Preserve(c *gc.C) {
	for i, newHook := range []newHook{
		operation.Factory.NewRunHook,
		operation.Factory.NewSkipHook,
	} {
		c.Logf("variant %d", i)
		s.testCommitSuccess(c,
			newHook,
			hook.Info{Kind: hooks.ConfigChanged},
			overwriteState,
			operation.State{
				Started: true,
				Kind:    operation.Continue,
				Step:    operation.Pending,
			},
		)
	}
}

func (s *RunHookSuite) TestCommitSuccess_Start_SetStarted(c *gc.C) {
	for i, newHook := range []newHook{
		operation.Factory.NewRunHook,
		operation.Factory.NewSkipHook,
	} {
		c.Logf("variant %d", i)
		s.testCommitSuccess(c,
			newHook,
			hook.Info{Kind: hooks.Start},
			operation.State{},
			operation.State{
				Started: true,
				Kind:    operation.Continue,
				Step:    operation.Pending,
			},
		)
	}
}

func (s *RunHookSuite) TestCommitSuccess_Start_Preserve(c *gc.C) {
	for i, newHook := range []newHook{
		operation.Factory.NewRunHook,
		operation.Factory.NewSkipHook,
	} {
		c.Logf("variant %d", i)
		s.testCommitSuccess(c,
			newHook,
			hook.Info{Kind: hooks.Start},
			overwriteState,
			operation.State{
				Started: true,
				Kind:    operation.Continue,
				Step:    operation.Pending,
			},
		)
	}
}

func (s *RunHookSuite) assertCommitSuccess_RelationBroken_SetStatus(c *gc.C, suspended, leader bool) {
	ctx := &MockContext{
		isLeader: leader,
		relation: &MockRelation{
			suspended: suspended,
		},
	}
	runnerFactory := &MockRunnerFactory{
		MockNewHookRunner: &MockNewHookRunner{
			runner: &MockRunner{
				MockRunHook: &MockRunHook{},
				context:     ctx,
			},
		},
	}
	callbacks := &ExecuteHookCallbacks{
		PrepareHookCallbacks:    NewPrepareHookCallbacks(hooks.RelationBroken),
		MockNotifyHookCompleted: &MockNotify{},
	}
	factory := newOpFactory(runnerFactory, callbacks)
	op, err := factory.NewRunHook(hook.Info{Kind: hooks.RelationBroken})
	c.Assert(err, jc.ErrorIsNil)

	_, err = op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(stdcontext.Background(), operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	step := operation.Done
	c.Assert(newState, gc.DeepEquals, &operation.State{
		Kind:     operation.RunHook,
		Step:     step,
		Hook:     &hook.Info{Kind: hooks.RelationBroken},
		HookStep: &step,
	})
	if suspended && leader {
		c.Assert(ctx.relation.status, gc.Equals, relation.Suspended)
	} else {
		c.Assert(ctx.relation.status, gc.Equals, relation.Status(""))
	}
}

func (s *RunHookSuite) TestCommitSuccess_RelationBroken_SetStatus(c *gc.C) {
	s.assertCommitSuccess_RelationBroken_SetStatus(c, true, true)
}

func (s *RunHookSuite) TestCommitSuccess_RelationBroken_SetStatusNotLeader(c *gc.C) {
	s.assertCommitSuccess_RelationBroken_SetStatus(c, true, false)
}

func (s *RunHookSuite) TestCommitSuccess_RelationBroken_SetStatusNotSuspended(c *gc.C) {
	s.assertCommitSuccess_RelationBroken_SetStatus(c, false, true)
}

func (s *RunHookSuite) testQueueHook_BlankSlate(c *gc.C, cause hooks.Kind) {
	for i, newHook := range []newHook{
		operation.Factory.NewRunHook,
		operation.Factory.NewSkipHook,
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
				Kind:    operation.RunHook,
				Step:    operation.Queued,
				Stopped: cause == hooks.Stop,
				Hook:    hi,
			},
		)
	}
}

func (s *RunHookSuite) testQueueHook_Preserve(c *gc.C, cause hooks.Kind) {
	for i, newHook := range []newHook{
		operation.Factory.NewRunHook,
		operation.Factory.NewSkipHook,
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
			overwriteState,
			operation.State{
				Kind:    operation.RunHook,
				Step:    operation.Queued,
				Started: true,
				Stopped: cause == hooks.Stop,
				Hook:    hi,
			},
		)
	}
}

func (s *RunHookSuite) TestQueueHook_UpgradeCharm_BlankSlate(c *gc.C) {
	s.testQueueHook_BlankSlate(c, hooks.UpgradeCharm)
}

func (s *RunHookSuite) TestQueueHook_UpgradeCharm_Preserve(c *gc.C) {
	s.testQueueHook_Preserve(c, hooks.UpgradeCharm)
}

func (s *RunHookSuite) testQueueNothing_BlankSlate(c *gc.C, hookInfo hook.Info) {
	for i, newHook := range []newHook{
		operation.Factory.NewRunHook,
		operation.Factory.NewSkipHook,
	} {
		c.Logf("variant %d", i)
		s.testCommitSuccess(c,
			newHook,
			hookInfo,
			operation.State{},
			operation.State{
				Installed: hookInfo.Kind == hooks.Install,
				Kind:      operation.Continue,
				Step:      operation.Pending,
				Stopped:   hookInfo.Kind == hooks.Stop,
			},
		)
	}
}

func (s *RunHookSuite) testQueueNothing_Preserve(c *gc.C, hookInfo hook.Info) {
	for i, newHook := range []newHook{
		operation.Factory.NewRunHook,
		operation.Factory.NewSkipHook,
	} {
		c.Logf("variant %d", i)
		s.testCommitSuccess(c,
			newHook,
			hookInfo,
			overwriteState,
			operation.State{
				Kind:      operation.Continue,
				Step:      operation.Pending,
				Installed: hookInfo.Kind == hooks.Install,
				Started:   true,
				Stopped:   hookInfo.Kind == hooks.Stop,
			},
		)
	}
}

func (s *RunHookSuite) TestQueueNothing_Install_BlankSlate(c *gc.C) {
	s.testQueueNothing_BlankSlate(c, hook.Info{
		Kind: hooks.Install,
	})
}

func (s *RunHookSuite) TestQueueNothing_Install_Preserve(c *gc.C) {
	s.testQueueNothing_Preserve(c, hook.Info{
		Kind: hooks.Install,
	})
}

func (s *RunHookSuite) TestQueueNothing_Stop_BlankSlate(c *gc.C) {
	s.testQueueNothing_BlankSlate(c, hook.Info{
		Kind: hooks.Stop,
	})
}

func (s *RunHookSuite) TestQueueNothing_Stop_Preserve(c *gc.C) {
	s.testQueueNothing_Preserve(c, hook.Info{
		Kind: hooks.Stop,
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationJoined_BlankSlate(c *gc.C) {
	s.testQueueNothing_BlankSlate(c, hook.Info{
		Kind:              hooks.RelationJoined,
		RemoteUnit:        "u/0",
		RemoteApplication: "u",
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationJoined_Preserve(c *gc.C) {
	s.testQueueNothing_Preserve(c, hook.Info{
		Kind:              hooks.RelationJoined,
		RemoteUnit:        "u/0",
		RemoteApplication: "u",
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationChanged_BlankSlate(c *gc.C) {
	s.testQueueNothing_BlankSlate(c, hook.Info{
		Kind:              hooks.RelationChanged,
		RemoteUnit:        "u/0",
		RemoteApplication: "u",
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationChanged_Preserve(c *gc.C) {
	s.testQueueNothing_Preserve(c, hook.Info{
		Kind:              hooks.RelationChanged,
		RemoteUnit:        "u/0",
		RemoteApplication: "u",
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationChangedApp_BlankSlate(c *gc.C) {
	s.testQueueNothing_BlankSlate(c, hook.Info{
		Kind:              hooks.RelationChanged,
		RemoteApplication: "u",
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationChangedApp_Preserve(c *gc.C) {
	s.testQueueNothing_Preserve(c, hook.Info{
		Kind:              hooks.RelationChanged,
		RemoteApplication: "u",
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationDeparted_BlankSlate(c *gc.C) {
	s.testQueueNothing_BlankSlate(c, hook.Info{
		Kind:              hooks.RelationDeparted,
		RemoteUnit:        "u/0",
		RemoteApplication: "u",
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationDeparted_Preserve(c *gc.C) {
	s.testQueueNothing_Preserve(c, hook.Info{
		Kind:              hooks.RelationDeparted,
		RemoteUnit:        "u/0",
		RemoteApplication: "u",
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationBroken_BlankSlate(c *gc.C) {
	s.testQueueNothing_BlankSlate(c, hook.Info{
		Kind: hooks.RelationBroken,
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationBroken_Preserve(c *gc.C) {
	s.testQueueNothing_Preserve(c, hook.Info{
		Kind: hooks.RelationBroken,
	})
}

func (s *RunHookSuite) testNeedsGlobalMachineLock(c *gc.C, newHook newHook, expected bool) {
	factory := newOpFactory(nil, nil)
	op, err := newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.NeedsGlobalMachineLock(), gc.Equals, expected)
}

func (s *RunHookSuite) TestNeedsGlobalMachineLock_Run(c *gc.C) {
	s.testNeedsGlobalMachineLock(c, operation.Factory.NewRunHook, true)
}

func (s *RunHookSuite) TestNeedsGlobalMachineLock_Skip(c *gc.C) {
	s.testNeedsGlobalMachineLock(c, operation.Factory.NewSkipHook, false)
}

func (s *RunHookSuite) TestRunningHookMessageForRelationHooks(c *gc.C) {
	msg := operation.RunningHookMessage(
		"host-relation-created",
		hook.Info{
			Kind:       hooks.RelationCreated,
			RemoteUnit: "alien/0",
		},
	)
	c.Assert(msg, gc.Equals, "running host-relation-created hook for alien/0", gc.Commentf("expected remote unit to be included for a relation hook with a populated remote unit"))

	msg = operation.RunningHookMessage(
		"install",
		hook.Info{
			Kind:       hooks.Install,
			RemoteUnit: "bogus",
		},
	)
	c.Assert(msg, gc.Equals, "running install hook", gc.Commentf("expected remote unit not to be included for a non-relation hook"))
}

func (s *RunHookSuite) TestRunningHookMessageForSecretsHooks(c *gc.C) {
	msg := operation.RunningHookMessage(
		"secret-rotate",
		hook.Info{
			Kind:      hooks.SecretRotate,
			SecretURI: "secret:9m4e2mr0ui3e8a215n4g",
		},
	)
	c.Assert(msg, gc.Equals, `running secret-rotate hook for secret:9m4e2mr0ui3e8a215n4g`)
}

func (s *RunHookSuite) TestRunningHookMessageForSecretHooksWithRevision(c *gc.C) {
	msg := operation.RunningHookMessage(
		"secret-expired",
		hook.Info{
			Kind:           hooks.SecretExpired,
			SecretURI:      "secret:9m4e2mr0ui3e8a215n4g",
			SecretRevision: 666,
		},
	)
	c.Assert(msg, gc.Equals, `running secret-expired hook for secret:9m4e2mr0ui3e8a215n4g/666`)
}

func (s *RunHookSuite) TestCommitSuccess_SecretRotate_SetRotated(c *gc.C) {
	callbacks := &CommitHookCallbacks{
		MockCommitHook: &MockCommitHook{},
	}
	runnerFactory := &MockRunnerFactory{
		MockNewHookRunner: &MockNewHookRunner{
			runner: &MockRunner{
				context: &MockContext{},
			},
		},
	}
	factory := newOpFactory(runnerFactory, callbacks)
	op, err := factory.NewRunHook(hook.Info{
		Kind: hooks.SecretRotate, SecretURI: "secret:9m4e2mr0ui3e8a215n4g",
	})
	c.Assert(err, jc.ErrorIsNil)

	expectState := &operation.State{
		Kind: operation.Continue,
		Step: operation.Pending,
	}

	_, err = op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	newState, err := op.Commit(stdcontext.Background(), operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, jc.DeepEquals, expectState)
	c.Assert(callbacks.rotatedSecretURI, gc.Equals, "secret:9m4e2mr0ui3e8a215n4g")
	c.Assert(callbacks.rotatedOldRevision, gc.Equals, 666)
}

func (s *RunHookSuite) TestPrepareSecretHookError_NotLeader(c *gc.C) {
	s.assertPrepareSecretHookErrorNotLeader(c, hooks.SecretRotate)
	s.assertPrepareSecretHookErrorNotLeader(c, hooks.SecretExpired)
	s.assertPrepareSecretHookErrorNotLeader(c, hooks.SecretRemove)
}

func (s *RunHookSuite) assertPrepareSecretHookErrorNotLeader(c *gc.C, kind hooks.Kind) {
	callbacks := &PrepareHookCallbacks{
		MockPrepareHook: &MockPrepareHook{nil, string(kind), nil},
	}
	runnerFactory := &MockRunnerFactory{
		MockNewHookRunner: &MockNewHookRunner{
			runner: &MockRunner{
				context: &MockContext{isLeader: false},
			},
		},
	}
	factory := newOpFactory(runnerFactory, callbacks)

	op, err := factory.NewRunHook(hook.Info{
		Kind: kind, SecretURI: "secret:9m4e2mr0ui3e8a215n4g", SecretRevision: 666,
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)
}
