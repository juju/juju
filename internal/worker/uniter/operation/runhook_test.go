// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	stdcontext "context"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/runner"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type RunHookSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&RunHookSuite{})

type newHook func(operation.Factory, hook.Info) (operation.Operation, error)

func (s *RunHookSuite) testPrepareHookError(
	c *tc.C, newHook newHook, expectSkip bool,
) {
	callbacks := &PrepareHookCallbacks{
		MockPrepareHook: &MockPrepareHook{err: errors.New("pow")},
	}
	factory := newOpFactory(c, nil, callbacks)
	op, err := newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Check(newState, tc.IsNil)
	if expectSkip {
		c.Check(err, tc.Equals, operation.ErrSkipExecute)
		c.Check(callbacks.MockPrepareHook.gotHook, tc.IsNil)
		return
	}
	c.Check(err, tc.ErrorMatches, "pow")
	c.Check(callbacks.MockPrepareHook.gotHook, tc.DeepEquals, &hook.Info{
		Kind: hooks.ConfigChanged,
	})
}

func (s *RunHookSuite) TestPrepareHookCtxCalled(c *tc.C) {
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
	factory := newOpFactory(c, runnerFactory, callbacks)

	op, err := factory.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Check(newState, tc.NotNil)
	c.Assert(err, tc.ErrorIsNil)

	ctx.CheckCall(c, 0, "Prepare")
}

func (s *RunHookSuite) TestPrepareHookCtxError(c *tc.C) {
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
	factory := newOpFactory(c, runnerFactory, callbacks)

	op, err := factory.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Check(newState, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, `ctx prepare error`)

	ctx.CheckCall(c, 0, "Prepare")
}

func (s *RunHookSuite) TestPrepareHookError_Run(c *tc.C) {
	s.testPrepareHookError(c, operation.Factory.NewRunHook, false)
}

func (s *RunHookSuite) TestPrepareHookError_Skip(c *tc.C) {
	s.testPrepareHookError(c, operation.Factory.NewSkipHook, true)
}

func (s *RunHookSuite) TestPrepareHookError_LeaderElectedNotLeader(c *tc.C) {
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
	factory := newOpFactory(c, runnerFactory, callbacks)

	op, err := operation.Factory.NewRunHook(factory, hook.Info{Kind: hooks.LeaderElected})
	c.Assert(err, tc.ErrorIsNil)

	_, err = op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, tc.Equals, operation.ErrSkipExecute)
}

func (s *RunHookSuite) testPrepareRunnerError(c *tc.C, newHook newHook) {
	callbacks := NewPrepareHookCallbacks(hooks.ConfigChanged)
	runnerFactory := &MockRunnerFactory{
		MockNewHookRunner: &MockNewHookRunner{err: errors.New("splat")},
	}
	factory := newOpFactory(c, runnerFactory, callbacks)
	op, err := newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Check(newState, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "splat")
	c.Check(runnerFactory.MockNewHookRunner.gotHook, tc.DeepEquals, &hook.Info{
		Kind: hooks.ConfigChanged,
	})
}

func (s *RunHookSuite) TestPrepareRunnerError_Run(c *tc.C) {
	s.testPrepareRunnerError(c, operation.Factory.NewRunHook)
}

func (s *RunHookSuite) testPrepareSuccess(
	c *tc.C, newHook newHook, before, after operation.State,
) {
	runnerFactory := NewRunHookRunnerFactory(errors.New("should not call"))
	callbacks := NewPrepareHookCallbacks(hooks.ConfigChanged)
	factory := newOpFactory(c, runnerFactory, callbacks)
	op, err := newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), before)
	c.Check(err, tc.ErrorIsNil)
	c.Check(newState, tc.DeepEquals, &after)
}

func (s *RunHookSuite) TestPrepareSuccess_BlankSlate(c *tc.C) {
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

func (s *RunHookSuite) TestPrepareSuccess_Preserve(c *tc.C) {
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
	c *tc.C, newHook newHook, kind hooks.Kind, runErr error, contextOps ...func(*MockContext),
) (operation.Operation, *ExecuteHookCallbacks, *MockRunnerFactory) {
	runnerFactory := NewRunHookRunnerFactory(runErr, contextOps...)
	callbacks := &ExecuteHookCallbacks{
		PrepareHookCallbacks:    NewPrepareHookCallbacks(kind),
		MockNotifyHookCompleted: &MockNotify{},
		MockNotifyHookFailed:    &MockNotify{},
	}
	factory := newOpFactory(c, runnerFactory, callbacks)

	op, err := newHook(factory, hook.Info{Kind: kind})

	c.Assert(err, tc.ErrorIsNil)
	return op, callbacks, runnerFactory
}

func (s *RunHookSuite) TestExecuteMissingHookError(c *tc.C) {
	runErr := charmrunner.NewMissingHookError("blah-blah")
	for _, kind := range hooks.UnitHooks() {
		c.Logf("hook %v", kind)
		op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, kind, runErr)
		_, err := op.Prepare(stdcontext.Background(), operation.State{})
		c.Assert(err, tc.ErrorIsNil)

		newState, err := op.Execute(stdcontext.Background(), operation.State{})
		c.Assert(err, tc.ErrorIsNil)

		s.assertStateMatches(c, newState, operation.RunHook, operation.Done, kind)

		c.Assert(*runnerFactory.MockNewHookRunner.runner.MockRunHook.gotName, tc.Equals, string(kind))
		c.Assert(callbacks.MockNotifyHookCompleted.gotName, tc.IsNil)
		c.Assert(callbacks.MockNotifyHookFailed.gotName, tc.IsNil)

		status, err := runnerFactory.MockNewHookRunner.runner.Context().UnitStatus(stdcontext.Background())
		c.Assert(err, tc.ErrorIsNil)
		testAfterHookStatus(c, kind, status, false)
	}
}

func (s *RunHookSuite) assertStateMatches(
	c *tc.C, st *operation.State, opKind operation.Kind, step operation.Step, hookKind hooks.Kind,
) {
	c.Assert(st.Kind, tc.Equals, opKind)
	c.Assert(st.Step, tc.Equals, step)
	c.Assert(st.Hook, tc.NotNil)
	c.Assert(st.Hook.Kind, tc.Equals, hookKind)
	if step == operation.Queued {
		c.Assert(st.HookStep, tc.NotNil)
		c.Assert(*st.HookStep, tc.Equals, step)
	}
}

func (s *RunHookSuite) TestExecuteRequeueRebootError(c *tc.C) {
	runErr := context.ErrRequeueAndReboot
	op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, hooks.ConfigChanged, runErr)
	_, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Execute(stdcontext.Background(), operation.State{})
	c.Assert(err, tc.Equals, operation.ErrNeedsReboot)

	s.assertStateMatches(c, newState, operation.RunHook, operation.Queued, hooks.ConfigChanged)

	c.Assert(*runnerFactory.MockNewHookRunner.runner.MockRunHook.gotName, tc.Equals, "config-changed")
	c.Assert(*callbacks.MockNotifyHookCompleted.gotName, tc.Equals, "config-changed")
	c.Assert(*callbacks.MockNotifyHookCompleted.gotContext, tc.Equals, runnerFactory.MockNewHookRunner.runner.context)
	c.Assert(callbacks.MockNotifyHookFailed.gotName, tc.IsNil)
}

func (s *RunHookSuite) TestExecuteRebootError(c *tc.C) {
	runErr := context.ErrReboot
	op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, hooks.ConfigChanged, runErr)
	_, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Execute(stdcontext.Background(), operation.State{})
	c.Assert(err, tc.Equals, operation.ErrNeedsReboot)

	s.assertStateMatches(c, newState, operation.RunHook, operation.Done, hooks.ConfigChanged)

	c.Assert(*runnerFactory.MockNewHookRunner.runner.MockRunHook.gotName, tc.Equals, "config-changed")
	c.Assert(*callbacks.MockNotifyHookCompleted.gotName, tc.Equals, "config-changed")
	c.Assert(*callbacks.MockNotifyHookCompleted.gotContext, tc.Equals, runnerFactory.MockNewHookRunner.runner.context)
	c.Assert(callbacks.MockNotifyHookFailed.gotName, tc.IsNil)
}

func (s *RunHookSuite) TestExecuteOtherError(c *tc.C) {
	runErr := errors.New("graaargh")
	op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, hooks.ConfigChanged, runErr)
	_, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Execute(stdcontext.Background(), operation.State{})
	c.Assert(err, tc.Equals, operation.ErrHookFailed)
	c.Assert(newState, tc.IsNil)
	c.Assert(*runnerFactory.MockNewHookRunner.runner.MockRunHook.gotName, tc.Equals, "config-changed")
	c.Assert(*callbacks.MockNotifyHookFailed.gotName, tc.Equals, "config-changed")
	c.Assert(*callbacks.MockNotifyHookFailed.gotContext, tc.Equals, runnerFactory.MockNewHookRunner.runner.context)
	c.Assert(callbacks.MockNotifyHookCompleted.gotName, tc.IsNil)
}

func (s *RunHookSuite) TestExecuteTerminated(c *tc.C) {
	runErr := runner.ErrTerminated
	op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, hooks.ConfigChanged, runErr)
	_, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Execute(stdcontext.Background(), operation.State{})
	c.Assert(err, tc.Equals, runner.ErrTerminated)

	s.assertStateMatches(c, newState, operation.RunHook, operation.Queued, hooks.ConfigChanged)

	c.Assert(*runnerFactory.MockNewHookRunner.runner.MockRunHook.gotName, tc.Equals, "config-changed")
	c.Assert(callbacks.MockNotifyHookCompleted.gotName, tc.IsNil)
}

func (s *RunHookSuite) TestInstallHookPreservesStatus(c *tc.C) {
	op, callbacks, f := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, hooks.Install, nil)
	err := f.MockNewHookRunner.runner.Context().SetUnitStatus(stdcontext.Background(), jujuc.StatusInfo{Status: "blocked", Info: "no database"})
	c.Assert(err, tc.ErrorIsNil)
	st := operation.State{
		StatusSet: true,
	}
	midState, err := op.Prepare(stdcontext.Background(), st)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(midState, tc.NotNil)

	_, err = op.Execute(stdcontext.Background(), *midState)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(callbacks.executingMessage, tc.Equals, "running install hook")
	status, err := f.MockNewHookRunner.runner.Context().UnitStatus(stdcontext.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(status.Status, tc.Equals, "blocked")
	c.Assert(status.Info, tc.Equals, "no database")
}

func (s *RunHookSuite) TestInstallHookWHenNoStatusSet(c *tc.C) {
	op, callbacks, f := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, hooks.Install, nil)
	st := operation.State{
		StatusSet: false,
	}
	midState, err := op.Prepare(stdcontext.Background(), st)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(midState, tc.NotNil)

	_, err = op.Execute(stdcontext.Background(), *midState)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(callbacks.executingMessage, tc.Equals, "running install hook")
	status, err := f.MockNewHookRunner.runner.Context().UnitStatus(stdcontext.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(status.Status, tc.Equals, "maintenance")
	c.Assert(status.Info, tc.Equals, "installing charm software")
}

func (s *RunHookSuite) testExecuteSuccess(
	c *tc.C, before, after operation.State, setStatusCalled bool,
) {
	op, callbacks, f := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, hooks.ConfigChanged, nil)
	f.MockNewHookRunner.runner.MockRunHook.setStatusCalled = setStatusCalled
	midState, err := op.Prepare(stdcontext.Background(), before)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(midState, tc.NotNil)

	newState, err := op.Execute(stdcontext.Background(), *midState)
	c.Assert(err, tc.ErrorIsNil)

	s.assertStateMatches(c, newState, after.Kind, after.Step, after.Hook.Kind)
	c.Assert(newState.Started, tc.Equals, after.Started)
	c.Assert(newState.StatusSet, tc.Equals, after.StatusSet)

	c.Check(callbacks.executingMessage, tc.Equals, "running config-changed hook")
}

func (s *RunHookSuite) TestExecuteSuccess_BlankSlate(c *tc.C) {
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

func (s *RunHookSuite) TestExecuteSuccess_Preserve(c *tc.C) {
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
	c *tc.C, before, after operation.State, kind hooks.Kind, setStatusCalled bool,
) {
	op, _, f := s.getExecuteRunnerTest(c, operation.Factory.NewRunHook, kind, nil)
	f.MockNewHookRunner.runner.MockRunHook.setStatusCalled = setStatusCalled
	midState, err := op.Prepare(stdcontext.Background(), before)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(midState, tc.NotNil)

	_, err = f.MockNewHookRunner.runner.Context().UnitStatus(stdcontext.Background())
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Execute(stdcontext.Background(), *midState)
	c.Assert(err, tc.ErrorIsNil)

	s.assertStateMatches(c, newState, after.Kind, after.Step, after.Hook.Kind)
	c.Assert(newState.Started, tc.Equals, after.Started)
	c.Assert(newState.StatusSet, tc.Equals, after.StatusSet)

	status, err := f.MockNewHookRunner.runner.Context().UnitStatus(stdcontext.Background())
	c.Assert(err, tc.ErrorIsNil)
	testAfterHookStatus(c, kind, status, after.StatusSet)
}

func testBeforeHookStatus(c *tc.C, kind hooks.Kind, status *jujuc.StatusInfo) {
	switch kind {
	case hooks.Install:
		c.Assert(status.Status, tc.Equals, "maintenance")
		c.Assert(status.Info, tc.Equals, "installing charm software")
	case hooks.Stop:
		c.Assert(status.Status, tc.Equals, "maintenance")
		c.Assert(status.Info, tc.Equals, "stopping charm software")
	case hooks.Remove:
		c.Assert(status.Status, tc.Equals, "maintenance")
		c.Assert(status.Info, tc.Equals, "cleaning up prior to charm deletion")
	default:
		c.Assert(status.Status, tc.Equals, "")
	}
}

func testAfterHookStatus(c *tc.C, kind hooks.Kind, status *jujuc.StatusInfo, statusSetCalled bool) {
	switch kind {
	case hooks.Install:
		c.Assert(status.Status, tc.Equals, "maintenance")
		c.Assert(status.Info, tc.Equals, "installing charm software")
	case hooks.Start:
		if statusSetCalled {
			c.Assert(status.Status, tc.Equals, "")
		} else {
			c.Assert(status.Status, tc.Equals, "unknown")
		}
	case hooks.Stop:
		c.Assert(status.Status, tc.Equals, "maintenance")
	case hooks.Remove:
		c.Assert(status.Status, tc.Equals, "terminated")
	default:
		c.Assert(status.Status, tc.Equals, "")
	}
}

func (s *RunHookSuite) testBeforeHookExecute(c *tc.C, newHook newHook, kind hooks.Kind) {
	// To check what happens in the beforeHook() call, we run a hook with an error
	// so that it does not complete successfully and thus afterHook() does not run,
	// overwriting the values.
	runErr := errors.New("graaargh")
	op, _, runnerFactory := s.getExecuteRunnerTest(c, newHook, kind, runErr)
	_, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Execute(stdcontext.Background(), operation.State{})
	c.Assert(err, tc.Equals, operation.ErrHookFailed)
	c.Assert(newState, tc.IsNil)

	status, err := runnerFactory.MockNewHookRunner.runner.Context().UnitStatus(stdcontext.Background())
	c.Assert(err, tc.ErrorIsNil)
	testBeforeHookStatus(c, kind, status)
}

func (s *RunHookSuite) TestBeforeHookStatus(c *tc.C) {
	for _, kind := range hooks.UnitHooks() {
		c.Logf("hook %v", kind)
		s.testBeforeHookExecute(c, operation.Factory.NewRunHook, kind)
	}
}

func (s *RunHookSuite) testExecuteHookWithSetStatus(c *tc.C, kind hooks.Kind, setStatusCalled bool) {
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

func (s *RunHookSuite) TestExecuteHookWithSetStatus(c *tc.C) {
	for _, kind := range hooks.UnitHooks() {
		c.Logf("hook %v", kind)
		s.testExecuteHookWithSetStatus(c, kind, true)
		s.testExecuteHookWithSetStatus(c, kind, false)
	}
}

func (s *RunHookSuite) testCommitError(c *tc.C, newHook newHook) {
	callbacks := &CommitHookCallbacks{
		MockCommitHook: &MockCommitHook{nil, errors.New("pow")},
	}
	factory := newOpFactory(c, nil, callbacks)
	op, err := newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Commit(stdcontext.Background(), operation.State{})
	c.Assert(newState, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "committing hook.*: pow")
}

func (s *RunHookSuite) TestCommitError_Run(c *tc.C) {
	s.testCommitError(c, operation.Factory.NewRunHook)
}

func (s *RunHookSuite) TestCommitError_Skip(c *tc.C) {
	s.testCommitError(c, operation.Factory.NewSkipHook)
}

func (s *RunHookSuite) testCommitSuccess(c *tc.C, newHook newHook, hookInfo hook.Info, before, after operation.State) {
	callbacks := &CommitHookCallbacks{
		MockCommitHook: &MockCommitHook{},
	}
	factory := newOpFactory(c, nil, callbacks)
	op, err := newHook(factory, hookInfo)
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Commit(stdcontext.Background(), before)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newState, tc.DeepEquals, &after)
}

func (s *RunHookSuite) TestCommitSuccess_ConfigChanged_QueueStartHook(c *tc.C) {
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

func (s *RunHookSuite) TestCommitSuccess_ConfigChanged_Preserve(c *tc.C) {
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

func (s *RunHookSuite) TestCommitSuccess_Start_SetStarted(c *tc.C) {
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

func (s *RunHookSuite) TestCommitSuccess_Start_Preserve(c *tc.C) {
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

func (s *RunHookSuite) assertCommitSuccess_RelationBroken_SetStatus(c *tc.C, suspended, leader bool) {
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
	factory := newOpFactory(c, runnerFactory, callbacks)
	op, err := factory.NewRunHook(hook.Info{Kind: hooks.RelationBroken})
	c.Assert(err, tc.ErrorIsNil)

	_, err = op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Execute(stdcontext.Background(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	step := operation.Done
	c.Assert(newState, tc.DeepEquals, &operation.State{
		Kind:     operation.RunHook,
		Step:     step,
		Hook:     &hook.Info{Kind: hooks.RelationBroken},
		HookStep: &step,
	})
	if suspended && leader {
		c.Assert(ctx.relation.status, tc.Equals, relation.Suspended)
	} else {
		c.Assert(ctx.relation.status, tc.Equals, relation.Status(""))
	}
}

func (s *RunHookSuite) TestCommitSuccess_RelationBroken_SetStatus(c *tc.C) {
	s.assertCommitSuccess_RelationBroken_SetStatus(c, true, true)
}

func (s *RunHookSuite) TestCommitSuccess_RelationBroken_SetStatusNotLeader(c *tc.C) {
	s.assertCommitSuccess_RelationBroken_SetStatus(c, true, false)
}

func (s *RunHookSuite) TestCommitSuccess_RelationBroken_SetStatusNotSuspended(c *tc.C) {
	s.assertCommitSuccess_RelationBroken_SetStatus(c, false, true)
}

func (s *RunHookSuite) testQueueHook_BlankSlate(c *tc.C, cause hooks.Kind) {
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

func (s *RunHookSuite) testQueueHook_Preserve(c *tc.C, cause hooks.Kind) {
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

func (s *RunHookSuite) TestQueueHook_UpgradeCharm_BlankSlate(c *tc.C) {
	s.testQueueHook_BlankSlate(c, hooks.UpgradeCharm)
}

func (s *RunHookSuite) TestQueueHook_UpgradeCharm_Preserve(c *tc.C) {
	s.testQueueHook_Preserve(c, hooks.UpgradeCharm)
}

func (s *RunHookSuite) testQueueNothing_BlankSlate(c *tc.C, hookInfo hook.Info) {
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

func (s *RunHookSuite) testQueueNothing_Preserve(c *tc.C, hookInfo hook.Info) {
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

func (s *RunHookSuite) TestQueueNothing_Install_BlankSlate(c *tc.C) {
	s.testQueueNothing_BlankSlate(c, hook.Info{
		Kind: hooks.Install,
	})
}

func (s *RunHookSuite) TestQueueNothing_Install_Preserve(c *tc.C) {
	s.testQueueNothing_Preserve(c, hook.Info{
		Kind: hooks.Install,
	})
}

func (s *RunHookSuite) TestQueueNothing_Stop_BlankSlate(c *tc.C) {
	s.testQueueNothing_BlankSlate(c, hook.Info{
		Kind: hooks.Stop,
	})
}

func (s *RunHookSuite) TestQueueNothing_Stop_Preserve(c *tc.C) {
	s.testQueueNothing_Preserve(c, hook.Info{
		Kind: hooks.Stop,
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationJoined_BlankSlate(c *tc.C) {
	s.testQueueNothing_BlankSlate(c, hook.Info{
		Kind:              hooks.RelationJoined,
		RemoteUnit:        "u/0",
		RemoteApplication: "u",
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationJoined_Preserve(c *tc.C) {
	s.testQueueNothing_Preserve(c, hook.Info{
		Kind:              hooks.RelationJoined,
		RemoteUnit:        "u/0",
		RemoteApplication: "u",
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationChanged_BlankSlate(c *tc.C) {
	s.testQueueNothing_BlankSlate(c, hook.Info{
		Kind:              hooks.RelationChanged,
		RemoteUnit:        "u/0",
		RemoteApplication: "u",
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationChanged_Preserve(c *tc.C) {
	s.testQueueNothing_Preserve(c, hook.Info{
		Kind:              hooks.RelationChanged,
		RemoteUnit:        "u/0",
		RemoteApplication: "u",
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationChangedApp_BlankSlate(c *tc.C) {
	s.testQueueNothing_BlankSlate(c, hook.Info{
		Kind:              hooks.RelationChanged,
		RemoteApplication: "u",
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationChangedApp_Preserve(c *tc.C) {
	s.testQueueNothing_Preserve(c, hook.Info{
		Kind:              hooks.RelationChanged,
		RemoteApplication: "u",
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationDeparted_BlankSlate(c *tc.C) {
	s.testQueueNothing_BlankSlate(c, hook.Info{
		Kind:              hooks.RelationDeparted,
		RemoteUnit:        "u/0",
		RemoteApplication: "u",
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationDeparted_Preserve(c *tc.C) {
	s.testQueueNothing_Preserve(c, hook.Info{
		Kind:              hooks.RelationDeparted,
		RemoteUnit:        "u/0",
		RemoteApplication: "u",
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationBroken_BlankSlate(c *tc.C) {
	s.testQueueNothing_BlankSlate(c, hook.Info{
		Kind: hooks.RelationBroken,
	})
}

func (s *RunHookSuite) TestQueueNothing_RelationBroken_Preserve(c *tc.C) {
	s.testQueueNothing_Preserve(c, hook.Info{
		Kind: hooks.RelationBroken,
	})
}

func (s *RunHookSuite) testNeedsGlobalMachineLock(c *tc.C, newHook newHook, expected bool) {
	factory := newOpFactory(c, nil, nil)
	op, err := newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.NeedsGlobalMachineLock(), tc.Equals, expected)
}

func (s *RunHookSuite) TestNeedsGlobalMachineLock_Run(c *tc.C) {
	s.testNeedsGlobalMachineLock(c, operation.Factory.NewRunHook, true)
}

func (s *RunHookSuite) TestNeedsGlobalMachineLock_Skip(c *tc.C) {
	s.testNeedsGlobalMachineLock(c, operation.Factory.NewSkipHook, false)
}

func (s *RunHookSuite) TestRunningHookMessageForRelationHooks(c *tc.C) {
	msg := operation.RunningHookMessage(
		"host-relation-created",
		hook.Info{
			Kind:       hooks.RelationCreated,
			RemoteUnit: "alien/0",
		},
	)
	c.Assert(msg, tc.Equals, "running host-relation-created hook for alien/0", tc.Commentf("expected remote unit to be included for a relation hook with a populated remote unit"))

	msg = operation.RunningHookMessage(
		"install",
		hook.Info{
			Kind:       hooks.Install,
			RemoteUnit: "bogus",
		},
	)
	c.Assert(msg, tc.Equals, "running install hook", tc.Commentf("expected remote unit not to be included for a non-relation hook"))
}

func (s *RunHookSuite) TestRunningHookMessageForSecretsHooks(c *tc.C) {
	msg := operation.RunningHookMessage(
		"secret-rotate",
		hook.Info{
			Kind:      hooks.SecretRotate,
			SecretURI: "secret:9m4e2mr0ui3e8a215n4g",
		},
	)
	c.Assert(msg, tc.Equals, `running secret-rotate hook for secret:9m4e2mr0ui3e8a215n4g`)
}

func (s *RunHookSuite) TestRunningHookMessageForSecretHooksWithRevision(c *tc.C) {
	msg := operation.RunningHookMessage(
		"secret-expired",
		hook.Info{
			Kind:           hooks.SecretExpired,
			SecretURI:      "secret:9m4e2mr0ui3e8a215n4g",
			SecretRevision: 666,
		},
	)
	c.Assert(msg, tc.Equals, `running secret-expired hook for secret:9m4e2mr0ui3e8a215n4g/666`)
}

func (s *RunHookSuite) TestCommitSuccess_SecretRotate_SetRotated(c *tc.C) {
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
	factory := newOpFactory(c, runnerFactory, callbacks)
	op, err := factory.NewRunHook(hook.Info{
		Kind: hooks.SecretRotate, SecretURI: "secret:9m4e2mr0ui3e8a215n4g",
	})
	c.Assert(err, tc.ErrorIsNil)

	expectState := &operation.State{
		Kind: operation.Continue,
		Step: operation.Pending,
	}

	_, err = op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	newState, err := op.Commit(stdcontext.Background(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newState, tc.DeepEquals, expectState)
	c.Assert(callbacks.rotatedSecretURI, tc.Equals, "secret:9m4e2mr0ui3e8a215n4g")
	c.Assert(callbacks.rotatedOldRevision, tc.Equals, 666)
}

func (s *RunHookSuite) TestPrepareSecretHookError_NotLeader(c *tc.C) {
	s.assertPrepareSecretHookErrorNotLeader(c, hooks.SecretRotate)
	s.assertPrepareSecretHookErrorNotLeader(c, hooks.SecretExpired)
	s.assertPrepareSecretHookErrorNotLeader(c, hooks.SecretRemove)
}

func (s *RunHookSuite) assertPrepareSecretHookErrorNotLeader(c *tc.C, kind hooks.Kind) {
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
	factory := newOpFactory(c, runnerFactory, callbacks)

	op, err := factory.NewRunHook(hook.Info{
		Kind: kind, SecretURI: "secret:9m4e2mr0ui3e8a215n4g", SecretRevision: 666,
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = op.Prepare(stdcontext.Background(), operation.State{})
	c.Assert(err, tc.Equals, operation.ErrSkipExecute)
}
