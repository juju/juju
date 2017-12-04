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
)

type ExecutorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ExecutorSuite{})

func (s *ExecutorSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *ExecutorSuite) TestNewExecutor(c *gc.C) {
	executor, err := operation.NewExecutor()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(executor.State(), gc.DeepEquals, operation.State{
		Kind: operation.Continue,
		Step: operation.Pending,
	})
}

func newExecutor(c *gc.C) operation.Executor {
	executor, err := operation.NewExecutor()
	c.Assert(err, jc.ErrorIsNil)
	return executor
}

func justStartedState() operation.State {
	return operation.State{
		Kind: operation.Continue,
		Step: operation.Pending,
	}
}

func (s *ExecutorSuite) TestSucceedNoStateChanges(c *gc.C) {
	initialState := justStartedState()
	executor := newExecutor(c)

	op := &mockOperation{
		prepare: newStep(nil, nil),
		execute: newStep(nil, nil),
		commit:  newStep(nil, nil),
	}

	err := executor.Run(op)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(op.prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(op.execute.gotState, gc.DeepEquals, initialState)
	c.Assert(op.commit.gotState, gc.DeepEquals, initialState)
	c.Assert(executor.State(), gc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestSucceedWithStateChanges(c *gc.C) {
	initialState := justStartedState()
	executor := newExecutor(c)
	op := &mockOperation{
		prepare: newStep(&operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		}, nil),
		execute: newStep(&operation.State{
			Kind: operation.RunHook,
			Step: operation.Done,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		}, nil),
		commit: newStep(&operation.State{
			Kind: operation.Continue,
			Step: operation.Pending,
		}, nil),
	}

	err := executor.Run(op)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(op.prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(op.execute.gotState, gc.DeepEquals, *op.prepare.newState)
	c.Assert(op.commit.gotState, gc.DeepEquals, *op.execute.newState)
	c.Assert(executor.State(), gc.DeepEquals, *op.commit.newState)
}

func (s *ExecutorSuite) TestErrSkipExecute(c *gc.C) {
	initialState := justStartedState()
	executor := newExecutor(c)
	op := &mockOperation{
		prepare: newStep(&operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		}, operation.ErrSkipExecute),
		commit: newStep(&operation.State{
			Kind: operation.Continue,
			Step: operation.Pending,
		}, nil),
	}

	err := executor.Run(op)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(op.prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(op.commit.gotState, gc.DeepEquals, *op.prepare.newState)
	c.Assert(executor.State(), gc.DeepEquals, *op.commit.newState)
}

func (s *ExecutorSuite) TestValidateStateChange(c *gc.C) {
	initialState := justStartedState()
	executor := newExecutor(c)
	op := &mockOperation{
		prepare: newStep(&operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
		}, nil),
	}

	err := executor.Run(op)
	c.Assert(err, gc.ErrorMatches, `preparing operation "mock operation": invalid operation state: missing hook info with Kind RunHook`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "missing hook info with Kind RunHook")

	c.Assert(executor.State(), gc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestFailPrepareNoStateChange(c *gc.C) {
	initialState := justStartedState()
	executor := newExecutor(c)
	op := &mockOperation{
		prepare: newStep(nil, errors.New("pow")),
	}

	err := executor.Run(op)
	c.Assert(err, gc.ErrorMatches, `preparing operation "mock operation": pow`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "pow")

	c.Assert(op.prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(executor.State(), gc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestFailPrepareWithStateChange(c *gc.C) {
	initialState := justStartedState()
	executor := newExecutor(c)
	op := &mockOperation{
		prepare: newStep(&operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		}, errors.New("blam")),
	}

	err := executor.Run(op)
	c.Assert(err, gc.ErrorMatches, `preparing operation "mock operation": blam`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "blam")

	c.Assert(op.prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(executor.State(), gc.DeepEquals, *op.prepare.newState)
}

func (s *ExecutorSuite) TestFailExecuteNoStateChange(c *gc.C) {
	initialState := justStartedState()
	executor := newExecutor(c)
	op := &mockOperation{
		prepare: newStep(nil, nil),
		execute: newStep(nil, errors.New("splat")),
	}

	err := executor.Run(op)
	c.Assert(err, gc.ErrorMatches, `executing operation "mock operation": splat`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "splat")

	c.Assert(op.prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(executor.State(), gc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestFailExecuteWithStateChange(c *gc.C) {
	initialState := justStartedState()
	executor := newExecutor(c)
	op := &mockOperation{
		prepare: newStep(nil, nil),
		execute: newStep(&operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		}, errors.New("kerblooie")),
	}

	err := executor.Run(op)
	c.Assert(err, gc.ErrorMatches, `executing operation "mock operation": kerblooie`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "kerblooie")

	c.Assert(op.prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(executor.State(), gc.DeepEquals, *op.execute.newState)
}

func (s *ExecutorSuite) TestFailCommitNoStateChange(c *gc.C) {
	initialState := justStartedState()
	executor := newExecutor(c)
	op := &mockOperation{
		prepare: newStep(nil, nil),
		execute: newStep(nil, nil),
		commit:  newStep(nil, errors.New("whack")),
	}

	err := executor.Run(op)
	c.Assert(err, gc.ErrorMatches, `committing operation "mock operation": whack`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "whack")

	c.Assert(op.prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(executor.State(), gc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestFailCommitWithStateChange(c *gc.C) {
	initialState := justStartedState()
	executor := newExecutor(c)
	op := &mockOperation{
		prepare: newStep(nil, nil),
		execute: newStep(nil, nil),
		commit: newStep(&operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		}, errors.New("take that you bandit")),
	}

	err := executor.Run(op)
	c.Assert(err, gc.ErrorMatches, `committing operation "mock operation": take that you bandit`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "take that you bandit")

	c.Assert(op.prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(executor.State(), gc.DeepEquals, *op.commit.newState)
}

type mockStep struct {
	gotState operation.State
	newState *operation.State
	err      error
	called   bool
}

func newStep(newState *operation.State, err error) *mockStep {
	return &mockStep{newState: newState, err: err}
}

func (step *mockStep) run(state operation.State) (*operation.State, error) {
	step.called = true
	step.gotState = state
	return step.newState, step.err
}

type mockOperation struct {
	needsLock bool
	prepare   *mockStep
	execute   *mockStep
	commit    *mockStep
}

func (op *mockOperation) String() string {
	return "mock operation"
}

func (op *mockOperation) Prepare(state operation.State) (*operation.State, error) {
	return op.prepare.run(state)
}

func (op *mockOperation) Execute(state operation.State) (*operation.State, error) {
	return op.execute.run(state)
}

func (op *mockOperation) Commit(state operation.State) (*operation.State, error) {
	return op.commit.run(state)
}
