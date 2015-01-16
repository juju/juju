// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/runner"
)

type RunHookSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RunHookSuite{})

type newHook func(operation.Factory, hook.Info) (operation.Operation, error)

func newRunHook(factory operation.Factory, hookInfo hook.Info) (operation.Operation, error) {
	return factory.NewRunHook(hookInfo)
}

func newRetryHook(factory operation.Factory, hookInfo hook.Info) (operation.Operation, error) {
	return factory.NewRetryHook(hookInfo)
}

func newSkipHook(factory operation.Factory, hookInfo hook.Info) (operation.Operation, error) {
	return factory.NewSkipHook(hookInfo)
}

type hookInfo struct {
	newHook                 newHook
	expectSkip              bool
	expectClearResolvedFlag bool
}

var allHookInfo = []hookInfo{{
	newHook: newRunHook,
}, {
	newHook:                 newRetryHook,
	expectClearResolvedFlag: true,
}, {
	newHook:                 newSkipHook,
	expectSkip:              true,
	expectClearResolvedFlag: true,
}}

func (s *RunHookSuite) TestClearResolvedFlagError(c *gc.C) {
	for i, info := range allHookInfo {
		c.Logf("variant %d", i)
		callbacks := &PrepareHookCallbacks{
			MockClearResolvedFlag: &MockNoArgs{err: errors.New("biff")},
		}
		factory := operation.NewFactory(nil, nil, callbacks, nil)
		op, err := info.newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
		c.Assert(err, jc.ErrorIsNil)
		if !info.expectClearResolvedFlag {
			// Nothing more worth testing -- and trying to prepare this op will
			// panic. We checked we could create it and that's what matters.
			continue
		}

		newState, err := op.Prepare(operation.State{})
		c.Check(newState, gc.IsNil)
		c.Check(callbacks.MockClearResolvedFlag.called, jc.IsTrue)
		c.Check(err, gc.ErrorMatches, "biff")
	}
}

func (s *RunHookSuite) TestPrepareHookError(c *gc.C) {
	for i, info := range allHookInfo {
		c.Logf("variant %d", i)
		callbacks := &PrepareHookCallbacks{
			MockPrepareHook:       &MockPrepareHook{err: errors.New("pow")},
			MockClearResolvedFlag: &MockNoArgs{},
		}
		factory := operation.NewFactory(nil, nil, callbacks, nil)
		op, err := info.newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Prepare(operation.State{})
		c.Check(newState, gc.IsNil)
		c.Check(callbacks.MockClearResolvedFlag.called, gc.Equals, info.expectClearResolvedFlag)
		if info.expectSkip {
			c.Check(err, gc.Equals, operation.ErrSkipExecute)
			c.Check(callbacks.MockPrepareHook.gotHook, gc.IsNil)
		} else {
			c.Check(err, gc.ErrorMatches, "pow")
			c.Check(callbacks.MockPrepareHook.gotHook, gc.DeepEquals, &hook.Info{
				Kind: hooks.ConfigChanged,
			})
		}
	}
}

func (s *RunHookSuite) TestPrepareRunnerError(c *gc.C) {
	for i, info := range allHookInfo {
		c.Logf("variant %d", i)
		callbacks := NewPrepareHookCallbacks()
		runnerFactory := &MockRunnerFactory{
			MockNewHookRunner: &MockNewHookRunner{err: errors.New("splat")},
		}
		factory := operation.NewFactory(nil, runnerFactory, callbacks, nil)
		op, err := info.newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Prepare(operation.State{})
		c.Check(newState, gc.IsNil)
		c.Check(callbacks.MockClearResolvedFlag.called, gc.Equals, info.expectClearResolvedFlag)
		if info.expectSkip {
			c.Check(err, gc.Equals, operation.ErrSkipExecute)
			c.Check(callbacks.MockPrepareHook.gotHook, gc.IsNil)
		} else {
			c.Check(err, gc.ErrorMatches, "splat")
			c.Check(runnerFactory.MockNewHookRunner.gotHook, gc.DeepEquals, &hook.Info{
				Kind: hooks.ConfigChanged,
			})
		}
	}
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
		for j, info := range allHookInfo {
			c.Logf("variant %d", j)
			runnerFactory := NewRunHookRunnerFactory(errors.New("should not call"))
			callbacks := NewPrepareHookCallbacks()
			factory := operation.NewFactory(nil, runnerFactory, callbacks, nil)
			op, err := info.newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
			c.Assert(err, jc.ErrorIsNil)

			newState, err := op.Prepare(test.before)
			c.Check(callbacks.MockClearResolvedFlag.called, gc.Equals, info.expectClearResolvedFlag)
			if info.expectSkip {
				c.Check(newState, gc.IsNil)
				c.Check(err, gc.Equals, operation.ErrSkipExecute)
				c.Check(callbacks.MockPrepareHook.gotHook, gc.IsNil)
			} else {
				c.Check(err, jc.ErrorIsNil)
				c.Assert(newState, gc.DeepEquals, &test.after)
			}
		}
	}
}

func (s *RunHookSuite) TestExecuteLockError(c *gc.C) {
	for i, info := range allHookInfo {
		c.Logf("variant %d", i)
		runnerFactory := NewRunHookRunnerFactory(errors.New("should not call"))
		callbacks := &ExecuteHookCallbacks{
			PrepareHookCallbacks:     NewPrepareHookCallbacks(),
			MockAcquireExecutionLock: &MockAcquireExecutionLock{err: errors.New("blart")},
		}
		factory := operation.NewFactory(nil, runnerFactory, callbacks, nil)
		op, err := info.newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
		c.Assert(err, jc.ErrorIsNil)
		_, err = op.Prepare(operation.State{})
		if info.expectSkip {
			c.Check(err, gc.Equals, operation.ErrSkipExecute)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Execute(operation.State{})
		c.Assert(newState, gc.IsNil)
		c.Assert(err, gc.ErrorMatches, "blart")
		c.Assert(*callbacks.MockAcquireExecutionLock.gotMessage, gc.Equals, "running hook some-hook-name")
	}
}

func (s *RunHookSuite) getExecuteRunnerTest(c *gc.C, newHook newHook, runErr error) (operation.Operation, *ExecuteHookCallbacks, *MockRunnerFactory) {
	runnerFactory := NewRunHookRunnerFactory(runErr)
	callbacks := &ExecuteHookCallbacks{
		PrepareHookCallbacks:     NewPrepareHookCallbacks(),
		MockAcquireExecutionLock: &MockAcquireExecutionLock{},
		MockNotifyHookCompleted:  &MockNotify{},
		MockNotifyHookFailed:     &MockNotify{},
	}
	factory := operation.NewFactory(nil, runnerFactory, callbacks, nil)
	op, err := newHook(factory, hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)
	return op, callbacks, runnerFactory
}

func (s *RunHookSuite) TestExecuteMissingHookError(c *gc.C) {
	for i, info := range allHookInfo {
		c.Logf("variant %d", i)
		runErr := runner.NewMissingHookError("blah-blah")
		op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, info.newHook, runErr)
		_, err := op.Prepare(operation.State{})
		if info.expectSkip {
			c.Check(err, gc.Equals, operation.ErrSkipExecute)
			continue
		}
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
}

func (s *RunHookSuite) TestExecuteRequeueRebootError(c *gc.C) {
	for i, info := range allHookInfo {
		c.Logf("variant %d", i)
		runErr := runner.ErrRequeueAndReboot
		op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, info.newHook, runErr)
		_, err := op.Prepare(operation.State{})
		if info.expectSkip {
			c.Check(err, gc.Equals, operation.ErrSkipExecute)
			continue
		}
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
}

func (s *RunHookSuite) TestExecuteRebootError(c *gc.C) {
	for i, info := range allHookInfo {
		c.Logf("variant %d", i)
		runErr := runner.ErrReboot
		op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, info.newHook, runErr)
		_, err := op.Prepare(operation.State{})
		if info.expectSkip {
			c.Check(err, gc.Equals, operation.ErrSkipExecute)
			continue
		}
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
}

func (s *RunHookSuite) TestExecuteOtherError(c *gc.C) {
	for i, info := range allHookInfo {
		c.Logf("variant %d", i)
		runErr := errors.New("graaargh")
		op, callbacks, runnerFactory := s.getExecuteRunnerTest(c, info.newHook, runErr)
		_, err := op.Prepare(operation.State{})
		if info.expectSkip {
			c.Check(err, gc.Equals, operation.ErrSkipExecute)
			continue
		}
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
		for j, info := range allHookInfo {
			c.Logf("variant %d", j)
			op, _, _ := s.getExecuteRunnerTest(c, info.newHook, nil)
			midState, err := op.Prepare(test.before)
			if info.expectSkip {
				c.Check(midState, gc.IsNil)
				c.Check(err, gc.Equals, operation.ErrSkipExecute)
				continue
			}
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(midState, gc.NotNil)
			newState, err := op.Execute(*midState)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(newState, gc.NotNil)
			c.Assert(*newState, gc.DeepEquals, test.after)
		}
	}
}

func (s *RunHookSuite) TestCommitError(c *gc.C) {
	callbacks := &CommitHookCallbacks{
		MockCommitHook: &MockCommitHook{nil, errors.New("pow")},
	}
	factory := operation.NewFactory(nil, nil, callbacks, nil)
	op, err := factory.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Commit(operation.State{})
	c.Assert(newState, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "pow")
}

type commitTest struct {
	description string
	commit      hook.Info
	before      operation.State
	after       operation.State
}

func (s *RunHookSuite) TestCommitSuccess(c *gc.C) {
	tests := []commitTest{{
		description: "config-changed hook queues start hook when not started",
		commit:      hook.Info{Kind: hooks.ConfigChanged},
		after: operation.State{
			Kind: operation.RunHook,
			Step: operation.Queued,
			Hook: &hook.Info{Kind: hooks.Start},
		},
	}, {
		description: "config-changed hook queues nothing and preserves state when already started",
		before:      overwriteState,
		commit:      hook.Info{Kind: hooks.ConfigChanged},
		after: operation.State{
			Started:            true,
			CollectMetricsTime: 1234567,
			Kind:               operation.Continue,
			Step:               operation.Pending,
			Hook:               &hook.Info{Kind: hooks.ConfigChanged},
		},
	}, {
		description: "start hook sets started and queues nothing on empty state",
		commit:      hook.Info{Kind: hooks.Start},
		after: operation.State{
			Started: true,
			Kind:    operation.Continue,
			Step:    operation.Pending,
			Hook:    &hook.Info{Kind: hooks.Start},
		},
	}, {
		description: "start hook queues nothing and preserves state when already started",
		before:      overwriteState,
		commit:      hook.Info{Kind: hooks.Start},
		after: operation.State{
			Started:            true,
			CollectMetricsTime: 1234567,
			Kind:               operation.Continue,
			Step:               operation.Pending,
			Hook:               &hook.Info{Kind: hooks.Start},
		},
	}}

	addQueueHookTests := func(commit, expect hooks.Kind) {
		tests = append(tests, commitTest{
			description: fmt.Sprintf("%s hook queues %s hook on empty state", commit, expect),
			commit:      hook.Info{Kind: commit},
			after: operation.State{
				Kind: operation.RunHook,
				Step: operation.Queued,
				Hook: &hook.Info{Kind: expect},
			},
		}, commitTest{
			description: fmt.Sprintf("%s hook queues %s hook and preserves state", commit, expect),
			before:      overwriteState,
			commit:      hook.Info{Kind: commit},
			after: operation.State{
				Started:            true,
				CollectMetricsTime: 1234567,
				Kind:               operation.RunHook,
				Step:               operation.Queued,
				Hook:               &hook.Info{Kind: expect},
			},
		})
	}
	addQueueHookTests(hooks.Install, hooks.ConfigChanged)
	addQueueHookTests(hooks.UpgradeCharm, hooks.ConfigChanged)

	addQueueNothingTests := func(commit hook.Info) {
		tests = append(tests, commitTest{
			description: fmt.Sprintf("%s hook queues nothing on empty state", commit.Kind),
			commit:      commit,
			after: operation.State{
				Kind: operation.Continue,
				Step: operation.Pending,
				Hook: &commit,
			},
		}, commitTest{
			description: fmt.Sprintf("%s hook queues nothing and preserves state", commit.Kind),
			before:      overwriteState,
			commit:      commit,
			after: operation.State{
				Started:            true,
				CollectMetricsTime: 1234567,
				Kind:               operation.Continue,
				Step:               operation.Pending,
				Hook:               &commit,
			},
		})
	}
	addQueueNothingTests(hook.Info{Kind: hooks.Stop})
	addQueueNothingTests(hook.Info{Kind: hooks.RelationJoined, RemoteUnit: "u/0"})
	addQueueNothingTests(hook.Info{Kind: hooks.RelationChanged, RemoteUnit: "u/0"})
	addQueueNothingTests(hook.Info{Kind: hooks.RelationDeparted, RemoteUnit: "u/0"})
	addQueueNothingTests(hook.Info{Kind: hooks.RelationBroken})

	for i, test := range tests {
		c.Logf("test %d: %s", i, test.description)
		for j, info := range allHookInfo {
			c.Logf("variant %d", j)
			after := s.commitHook(c, info.newHook, test.commit, test.before)
			c.Check(after, gc.DeepEquals, test.after)
		}
	}
}

func (s *RunHookSuite) TestCommitSuccessSetsCollectMetricsTime(c *gc.C) {
	for i, info := range allHookInfo {
		c.Logf("variant %d", i)
		hookInfo := hook.Info{Kind: hooks.CollectMetrics}
		nowBefore := time.Now().Unix()
		afterState := s.commitHook(c, info.newHook, hookInfo, overwriteState)
		nowAfter := time.Now().Unix()
		nowWritten := afterState.CollectMetricsTime
		c.Logf("%d <= %d <= %d", nowBefore, nowWritten, nowAfter)
		c.Check(nowBefore <= nowWritten, jc.IsTrue)
		c.Check(nowWritten <= nowAfter, jc.IsTrue)

		// Check the other fields match.
		afterState.CollectMetricsTime = 0
		c.Check(afterState, gc.DeepEquals, operation.State{
			Started: true,
			Kind:    operation.Continue,
			Step:    operation.Pending,
			Hook:    &hookInfo,
		})
	}
}

func (s *RunHookSuite) commitHook(c *gc.C, newHook newHook, hookInfo hook.Info, beforeState operation.State) operation.State {
	callbacks := &CommitHookCallbacks{
		MockCommitHook: &MockCommitHook{},
	}
	factory := operation.NewFactory(nil, nil, callbacks, nil)
	op, err := newHook(factory, hookInfo)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Commit(beforeState)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.NotNil)
	return *newState
}
