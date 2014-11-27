// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/context"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

type RunHookSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RunHookSuite{})

func (s *RunHookSuite) TestPrepareHookError(c *gc.C) {
	callbacks := &PrepareHookCallbacks{
		MockPrepareHook: &MockPrepareHook{err: errors.New("pow")},
	}
	factory := operation.NewFactory(nil, nil, callbacks, nil)
	op, err := factory.NewHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "pow")
	c.Assert(callbacks.MockPrepareHook.gotHook, gc.DeepEquals, &hook.Info{
		Kind: hooks.ConfigChanged,
	})
}

func (s *RunHookSuite) TestPrepareRunnerError(c *gc.C) {
	callbacks := NewPrepareHookCallbacks()
	runnerFactory := &MockRunnerFactory{
		MockNewHookRunner: &MockNewHookRunner{err: errors.New("splat")},
	}
	factory := operation.NewFactory(nil, runnerFactory, callbacks, nil)
	op, err := factory.NewHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "splat")
	c.Assert(runnerFactory.MockNewHookRunner.gotHook, gc.DeepEquals, &hook.Info{
		Kind: hooks.ConfigChanged,
	})
}

func (s *RunHookSuite) TestPrepareSuccess(c *gc.C) {
	var stateChangeTests = []struct {
		before operation.State
		after  operation.State
	}{{
		operation.State{},
		operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		},
	}, {
		overwriteState,
		operation.State{
			Started:            true,
			CollectMetricsTime: 1234567,
			Kind:               operation.RunHook,
			Step:               operation.Pending,
			Hook:               &hook.Info{Kind: hooks.ConfigChanged},
		},
	}}

	for i, test := range stateChangeTests {
		c.Logf("test %d", i)
		runnerFactory := NewRunHookRunnerFactory(errors.New("should not call"))
		callbacks := NewPrepareHookCallbacks()
		factory := operation.NewFactory(nil, runnerFactory, callbacks, nil)
		op, err := factory.NewHook(hook.Info{Kind: hooks.ConfigChanged})
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Prepare(test.before)
		c.Assert(newState, gc.DeepEquals, &test.after)
	}
}

func (s *RunHookSuite) TestExecuteLockError(c *gc.C) {
	runnerFactory := NewRunHookRunnerFactory(errors.New("should not call"))
	callbacks := &ExecuteHookCallbacks{
		PrepareHookCallbacks:     NewPrepareHookCallbacks(),
		MockAcquireExecutionLock: &MockAcquireExecutionLock{err: errors.New("blart")},
	}
	factory := operation.NewFactory(nil, runnerFactory, callbacks, nil)
	op, err := factory.NewHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)
	_, err = op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "blart")
	c.Assert(*callbacks.MockAcquireExecutionLock.gotMessage, gc.Equals, "running hook some-hook-name")
}

func (s *RunHookSuite) getExecuteRunnerTest(c *gc.C, runErr error) (operation.Operation, *ExecuteHookCallbacks, *MockRunnerFactory) {
	runnerFactory := NewRunHookRunnerFactory(runErr)
	callbacks := &ExecuteHookCallbacks{
		PrepareHookCallbacks:     NewPrepareHookCallbacks(),
		MockAcquireExecutionLock: &MockAcquireExecutionLock{},
		MockNotifyHookCompleted:  &MockNotify{},
		MockNotifyHookFailed:     &MockNotify{},
	}
	factory := operation.NewFactory(nil, runnerFactory, callbacks, nil)
	op, err := factory.NewHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)
	return op, callbacks, runnerFactory
}

func (s *RunHookSuite) TestExecuteMissingHookError(c *gc.C) {
	op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, context.NewMissingHookError("blah-blah"))
	_, err := op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.DeepEquals, &operation.State{
		Kind: operation.RunHook,
		Step: operation.Done,
		Hook: &hook.Info{Kind: hooks.ConfigChanged},
	})
	c.Assert(*callbacks.MockAcquireExecutionLock.gotMessage, gc.Equals, "running hook some-hook-name")
	c.Assert(callbacks.MockAcquireExecutionLock.didUnlock, jc.IsTrue)
	c.Assert(*runnerFactory.MockNewHookRunner.runner.MockRunHook.gotName, gc.Equals, "some-hook-name")
	c.Assert(callbacks.MockNotifyHookCompleted.gotName, gc.IsNil)
	c.Assert(callbacks.MockNotifyHookFailed.gotName, gc.IsNil)
}

func (s *RunHookSuite) TestExecuteRequeueRebootError(c *gc.C) {
	op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, context.ErrRequeueAndReboot)
	_, err := op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(operation.State{})
	c.Assert(err, gc.Equals, operation.ErrNeedsReboot)
	c.Assert(newState, gc.DeepEquals, &operation.State{
		Kind: operation.RunHook,
		Step: operation.Queued,
		Hook: &hook.Info{Kind: hooks.ConfigChanged},
	})
	c.Assert(*callbacks.MockAcquireExecutionLock.gotMessage, gc.Equals, "running hook some-hook-name")
	c.Assert(callbacks.MockAcquireExecutionLock.didUnlock, jc.IsTrue)
	c.Assert(*runnerFactory.MockNewHookRunner.runner.MockRunHook.gotName, gc.Equals, "some-hook-name")
	c.Assert(*callbacks.MockNotifyHookCompleted.gotName, gc.Equals, "some-hook-name")
	c.Assert(*callbacks.MockNotifyHookCompleted.gotContext, gc.Equals, runnerFactory.MockNewHookRunner.runner.context)
	c.Assert(callbacks.MockNotifyHookFailed.gotName, gc.IsNil)
}

func (s *RunHookSuite) TestExecuteRebootError(c *gc.C) {
	op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, context.ErrReboot)
	_, err := op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(operation.State{})
	c.Assert(err, gc.Equals, operation.ErrNeedsReboot)
	c.Assert(newState, gc.DeepEquals, &operation.State{
		Kind: operation.RunHook,
		Step: operation.Done,
		Hook: &hook.Info{Kind: hooks.ConfigChanged},
	})
	c.Assert(*callbacks.MockAcquireExecutionLock.gotMessage, gc.Equals, "running hook some-hook-name")
	c.Assert(callbacks.MockAcquireExecutionLock.didUnlock, jc.IsTrue)
	c.Assert(*runnerFactory.MockNewHookRunner.runner.MockRunHook.gotName, gc.Equals, "some-hook-name")
	c.Assert(*callbacks.MockNotifyHookCompleted.gotName, gc.Equals, "some-hook-name")
	c.Assert(*callbacks.MockNotifyHookCompleted.gotContext, gc.Equals, runnerFactory.MockNewHookRunner.runner.context)
	c.Assert(callbacks.MockNotifyHookFailed.gotName, gc.IsNil)
}

func (s *RunHookSuite) TestExecuteOtherError(c *gc.C) {
	op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, errors.New("graaargh"))
	_, err := op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Execute(operation.State{})
	c.Assert(err, gc.Equals, operation.ErrHookFailed)
	c.Assert(newState, gc.IsNil)
	c.Assert(*callbacks.MockAcquireExecutionLock.gotMessage, gc.Equals, "running hook some-hook-name")
	c.Assert(callbacks.MockAcquireExecutionLock.didUnlock, jc.IsTrue)
	c.Assert(*runnerFactory.MockNewHookRunner.runner.MockRunHook.gotName, gc.Equals, "some-hook-name")
	c.Assert(*callbacks.MockNotifyHookFailed.gotName, gc.Equals, "some-hook-name")
	c.Assert(*callbacks.MockNotifyHookFailed.gotContext, gc.Equals, runnerFactory.MockNewHookRunner.runner.context)
	c.Assert(callbacks.MockNotifyHookCompleted.gotName, gc.IsNil)
}

func (s *RunHookSuite) TestExecuteSuccess(c *gc.C) {
	var stateChangeTests = []struct {
		before operation.State
		after  operation.State
	}{{
		operation.State{},
		operation.State{
			Kind: operation.RunHook,
			Step: operation.Done,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		},
	}, {
		overwriteState,
		operation.State{
			Started:            true,
			CollectMetricsTime: 1234567,
			Kind:               operation.RunHook,
			Step:               operation.Done,
			Hook:               &hook.Info{Kind: hooks.ConfigChanged},
		},
	}}

	for i, test := range stateChangeTests {
		c.Logf("test %d", i)
		op, _, _ := s.getExecuteRunnerTest(c, nil)
		midState, err := op.Prepare(test.before)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(midState, gc.NotNil)
		newState, err := op.Execute(*midState)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(newState, gc.NotNil)
		c.Assert(*newState, gc.DeepEquals, test.after)
	}
}

func (s *RunHookSuite) TestCommitError(c *gc.C) {
	callbacks := &CommitHookCallbacks{
		MockCommitHook: &MockCommitHook{nil, errors.New("pow")},
	}
	factory := operation.NewFactory(nil, nil, callbacks, nil)
	op, err := factory.NewHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Commit(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "pow")
}

func (s *RunHookSuite) commitHook(c *gc.C, hookInfo hook.Info, beforeState operation.State) operation.State {
	callbacks := &CommitHookCallbacks{
		MockCommitHook: &MockCommitHook{},
	}
	factory := operation.NewFactory(nil, nil, callbacks, nil)
	op, err := factory.NewHook(hookInfo)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Commit(beforeState)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.NotNil)
	return *newState
}

func (s *RunHookSuite) TestCommitSuccessEmptyInitial(c *gc.C) {
	hookInfo := hook.Info{Kind: hooks.ConfigChanged}
	beforeState := operation.State{}
	afterState := s.commitHook(c, hookInfo, beforeState)
	c.Assert(afterState, gc.DeepEquals, operation.State{
		Kind: operation.Continue,
		Step: operation.Pending,
		Hook: &hookInfo,
	})
}

func (s *RunHookSuite) TestCommitSuccessYuckyInitial(c *gc.C) {
	hookInfo := hook.Info{Kind: hooks.ConfigChanged}
	afterState := s.commitHook(c, hookInfo, overwriteState)
	c.Assert(afterState, gc.DeepEquals, operation.State{
		Started:            true,
		CollectMetricsTime: 1234567,
		Kind:               operation.Continue,
		Step:               operation.Pending,
		Hook:               &hookInfo,
	})
}

func (s *RunHookSuite) TestCommitSuccessSetsStarted(c *gc.C) {
	hookInfo := hook.Info{Kind: hooks.Start}
	beforeState := overwriteState
	beforeState.Started = false
	afterState := s.commitHook(c, hookInfo, beforeState)
	c.Assert(afterState, gc.DeepEquals, operation.State{
		Started:            true,
		CollectMetricsTime: 1234567,
		Kind:               operation.Continue,
		Step:               operation.Pending,
		Hook:               &hookInfo,
	})
}

func (s *RunHookSuite) TestCommitSuccessSetsCollectMetricsTime(c *gc.C) {
	hookInfo := hook.Info{Kind: hooks.CollectMetrics}
	nowBefore := time.Now().Unix()
	afterState := s.commitHook(c, hookInfo, overwriteState)
	nowAfter := time.Now().Unix()
	nowWritten := afterState.CollectMetricsTime
	c.Logf("%d <= %d <= %d", nowBefore, nowWritten, nowAfter)
	c.Check(nowBefore <= nowWritten, jc.IsTrue)
	c.Check(nowWritten <= nowAfter, jc.IsTrue)

	// Check the other fields match.
	afterState.CollectMetricsTime = 0
	c.Assert(afterState, gc.DeepEquals, operation.State{
		Started: true,
		Kind:    operation.Continue,
		Step:    operation.Pending,
		Hook:    &hookInfo,
	})
}
