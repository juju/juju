// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	ft "github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

type NewExecutorSuite struct {
	testing.IsolationSuite
	basePath string
}

var _ = gc.Suite(&NewExecutorSuite{})

func failGetInstallCharm() (*corecharm.URL, error) {
	return nil, errors.New("lol!")
}

func failAcquireLock(string) (func() error, error) {
	return nil, errors.New("wat")
}

func (s *NewExecutorSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.basePath = c.MkDir()
}

func (s *NewExecutorSuite) path(path string) string {
	return filepath.Join(s.basePath, path)
}

func (s *NewExecutorSuite) TestNewExecutorNoFileNoCharm(c *gc.C) {
	executor, err := operation.NewExecutor(s.path("missing"), failGetInstallCharm, failAcquireLock)
	c.Assert(executor, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "lol!")
}

func (s *NewExecutorSuite) TestNewExecutorInvalidFile(c *gc.C) {
	ft.File{"existing", "", 0666}.Create(c, s.basePath)
	executor, err := operation.NewExecutor(s.path("existing"), failGetInstallCharm, failAcquireLock)
	c.Assert(executor, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `cannot read ".*": invalid operation state: .*`)
}

func (s *NewExecutorSuite) TestNewExecutorNoFile(c *gc.C) {
	charmURL := corecharm.MustParseURL("cs:quantal/nyancat-323")
	getInstallCharm := func() (*corecharm.URL, error) {
		return charmURL, nil
	}
	executor, err := operation.NewExecutor(s.path("missing"), getInstallCharm, failAcquireLock)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(executor.State(), gc.DeepEquals, operation.State{
		Kind:     operation.Install,
		Step:     operation.Queued,
		CharmURL: charmURL,
	})
	ft.Removed{"missing"}.Check(c, s.basePath)
}

func (s *NewExecutorSuite) TestNewExecutorValidFile(c *gc.C) {
	// note: this content matches valid persistent state as of 1.21; we expect
	// that "hook" will have to become "last-hook" to enable action execution
	// during hook error states. If you do this, please leave at least one test
	// with this form of the yaml in place.
	ft.File{"existing", `
started: true
op: continue
opstep: pending
`[1:], 0666}.Create(c, s.basePath)
	executor, err := operation.NewExecutor(s.path("existing"), failGetInstallCharm, failAcquireLock)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(executor.State(), gc.DeepEquals, operation.State{
		Kind:    operation.Continue,
		Step:    operation.Pending,
		Started: true,
	})
}

type ExecutorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ExecutorSuite{})

func assertWroteState(c *gc.C, path string, expect operation.State) {
	actual, err := operation.NewStateFile(path).Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*actual, gc.DeepEquals, expect)
}

func newExecutor(c *gc.C, st *operation.State) (operation.Executor, string) {
	path := filepath.Join(c.MkDir(), "state")
	err := operation.NewStateFile(path).Write(st)
	c.Assert(err, jc.ErrorIsNil)
	executor, err := operation.NewExecutor(path, failGetInstallCharm, failAcquireLock)
	c.Assert(err, jc.ErrorIsNil)
	return executor, path
}

func justInstalledState() operation.State {
	return operation.State{
		Kind: operation.Continue,
		Step: operation.Pending,
	}
}

func (s *ExecutorSuite) TestSucceedNoStateChanges(c *gc.C) {
	initialState := justInstalledState()
	executor, statePath := newExecutor(c, &initialState)

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
	assertWroteState(c, statePath, initialState)
	c.Assert(executor.State(), gc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestSucceedWithStateChanges(c *gc.C) {
	initialState := justInstalledState()
	executor, statePath := newExecutor(c, &initialState)
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
			Kind: operation.RunHook,
			Step: operation.Queued,
			Hook: &hook.Info{Kind: hooks.Start},
		}, nil),
	}

	err := executor.Run(op)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(op.prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(op.execute.gotState, gc.DeepEquals, *op.prepare.newState)
	c.Assert(op.commit.gotState, gc.DeepEquals, *op.execute.newState)
	assertWroteState(c, statePath, *op.commit.newState)
	c.Assert(executor.State(), gc.DeepEquals, *op.commit.newState)
}

func (s *ExecutorSuite) TestErrSkipExecute(c *gc.C) {
	initialState := justInstalledState()
	executor, statePath := newExecutor(c, &initialState)
	op := &mockOperation{
		prepare: newStep(&operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		}, operation.ErrSkipExecute),
		commit: newStep(&operation.State{
			Kind: operation.RunHook,
			Step: operation.Queued,
			Hook: &hook.Info{Kind: hooks.Start},
		}, nil),
	}

	err := executor.Run(op)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(op.prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(op.commit.gotState, gc.DeepEquals, *op.prepare.newState)
	assertWroteState(c, statePath, *op.commit.newState)
	c.Assert(executor.State(), gc.DeepEquals, *op.commit.newState)
}

func (s *ExecutorSuite) TestValidateStateChange(c *gc.C) {
	initialState := justInstalledState()
	executor, statePath := newExecutor(c, &initialState)
	op := &mockOperation{
		prepare: newStep(&operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
		}, nil),
	}

	err := executor.Run(op)
	c.Assert(err, gc.ErrorMatches, `preparing operation "mock operation": invalid operation state: missing hook info with Kind RunHook`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "missing hook info with Kind RunHook")

	assertWroteState(c, statePath, initialState)
	c.Assert(executor.State(), gc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestFailPrepareNoStateChange(c *gc.C) {
	initialState := justInstalledState()
	executor, statePath := newExecutor(c, &initialState)
	op := &mockOperation{
		prepare: newStep(nil, errors.New("pow")),
	}

	err := executor.Run(op)
	c.Assert(err, gc.ErrorMatches, `preparing operation "mock operation": pow`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "pow")

	c.Assert(op.prepare.gotState, gc.DeepEquals, initialState)
	assertWroteState(c, statePath, initialState)
	c.Assert(executor.State(), gc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestFailPrepareWithStateChange(c *gc.C) {
	initialState := justInstalledState()
	executor, statePath := newExecutor(c, &initialState)
	op := &mockOperation{
		prepare: newStep(&operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.Start},
		}, errors.New("blam")),
	}

	err := executor.Run(op)
	c.Assert(err, gc.ErrorMatches, `preparing operation "mock operation": blam`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "blam")

	c.Assert(op.prepare.gotState, gc.DeepEquals, initialState)
	assertWroteState(c, statePath, *op.prepare.newState)
	c.Assert(executor.State(), gc.DeepEquals, *op.prepare.newState)
}

func (s *ExecutorSuite) TestFailExecuteNoStateChange(c *gc.C) {
	initialState := justInstalledState()
	executor, statePath := newExecutor(c, &initialState)
	op := &mockOperation{
		prepare: newStep(nil, nil),
		execute: newStep(nil, errors.New("splat")),
	}

	err := executor.Run(op)
	c.Assert(err, gc.ErrorMatches, `executing operation "mock operation": splat`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "splat")

	c.Assert(op.prepare.gotState, gc.DeepEquals, initialState)
	assertWroteState(c, statePath, initialState)
	c.Assert(executor.State(), gc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestFailExecuteWithStateChange(c *gc.C) {
	initialState := justInstalledState()
	executor, statePath := newExecutor(c, &initialState)
	op := &mockOperation{
		prepare: newStep(nil, nil),
		execute: newStep(&operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.Start},
		}, errors.New("kerblooie")),
	}

	err := executor.Run(op)
	c.Assert(err, gc.ErrorMatches, `executing operation "mock operation": kerblooie`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "kerblooie")

	c.Assert(op.prepare.gotState, gc.DeepEquals, initialState)
	assertWroteState(c, statePath, *op.execute.newState)
	c.Assert(executor.State(), gc.DeepEquals, *op.execute.newState)
}

func (s *ExecutorSuite) TestFailCommitNoStateChange(c *gc.C) {
	initialState := justInstalledState()
	executor, statePath := newExecutor(c, &initialState)
	op := &mockOperation{
		prepare: newStep(nil, nil),
		execute: newStep(nil, nil),
		commit:  newStep(nil, errors.New("whack")),
	}

	err := executor.Run(op)
	c.Assert(err, gc.ErrorMatches, `committing operation "mock operation": whack`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "whack")

	c.Assert(op.prepare.gotState, gc.DeepEquals, initialState)
	assertWroteState(c, statePath, initialState)
	c.Assert(executor.State(), gc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestFailCommitWithStateChange(c *gc.C) {
	initialState := justInstalledState()
	executor, statePath := newExecutor(c, &initialState)
	op := &mockOperation{
		prepare: newStep(nil, nil),
		execute: newStep(nil, nil),
		commit: newStep(&operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.Start},
		}, errors.New("take that you bandit")),
	}

	err := executor.Run(op)
	c.Assert(err, gc.ErrorMatches, `committing operation "mock operation": take that you bandit`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "take that you bandit")

	c.Assert(op.prepare.gotState, gc.DeepEquals, initialState)
	assertWroteState(c, statePath, *op.commit.newState)
	c.Assert(executor.State(), gc.DeepEquals, *op.commit.newState)
}

func (s *ExecutorSuite) initLockTest(c *gc.C, lockFunc func(string) (func() error, error)) operation.Executor {

	initialState := justInstalledState()
	statePath := filepath.Join(c.MkDir(), "state")
	err := operation.NewStateFile(statePath).Write(&initialState)
	c.Assert(err, jc.ErrorIsNil)
	executor, err := operation.NewExecutor(statePath, failGetInstallCharm, lockFunc)
	c.Assert(err, jc.ErrorIsNil)

	return executor
}

func (s *ExecutorSuite) TestLockSucceedsStepsCalled(c *gc.C) {
	op := &mockOperation{
		needsLock: true,
		prepare:   newStep(nil, nil),
		execute:   newStep(nil, nil),
		commit:    newStep(nil, nil),
	}

	mockLock := &mockLockFunc{op: op}
	lockFunc := mockLock.newSucceedingLockUnlockSucceeds()
	executor := s.initLockTest(c, lockFunc)

	err := executor.Run(op)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(mockLock.calledLock, jc.IsTrue)
	c.Assert(mockLock.calledUnlock, jc.IsTrue)
	c.Assert(mockLock.noStepsCalledOnLock, jc.IsTrue)

	expectedStepsOnUnlock := []bool{true, true, true}
	c.Assert(mockLock.stepsCalledOnUnlock, gc.DeepEquals, expectedStepsOnUnlock)
}

func (s *ExecutorSuite) TestLockSucceedsStepsCalledUnlockFails(c *gc.C) {
	op := &mockOperation{
		needsLock: true,
		prepare:   newStep(nil, nil),
		execute:   newStep(nil, nil),
		commit:    newStep(nil, nil),
	}

	mockLock := &mockLockFunc{op: op}
	lockFunc := mockLock.newSucceedingLockUnlockFails()
	executor := s.initLockTest(c, lockFunc)

	err := executor.Run(op)
	c.Assert(err, gc.ErrorMatches, "but why")

	c.Assert(mockLock.calledLock, jc.IsTrue)
	c.Assert(mockLock.calledUnlock, jc.IsTrue)
	c.Assert(mockLock.noStepsCalledOnLock, jc.IsTrue)

	expectedStepsOnUnlock := []bool{true, true, true}
	c.Assert(mockLock.stepsCalledOnUnlock, gc.DeepEquals, expectedStepsOnUnlock)
}

func (s *ExecutorSuite) TestOpFailsUnlockFailsUnlockErrPropagated(c *gc.C) {
	op := &mockOperation{
		needsLock: true,
		prepare:   newStep(nil, errors.New("kerblooie")),
		execute:   newStep(nil, nil),
		commit:    newStep(nil, nil),
	}

	mockLock := &mockLockFunc{op: op}
	lockFunc := mockLock.newSucceedingLockUnlockFails()
	executor := s.initLockTest(c, lockFunc)

	err := executor.Run(op)
	c.Assert(err, gc.ErrorMatches, "but why")

	c.Assert(mockLock.calledLock, jc.IsTrue)
	c.Assert(mockLock.calledUnlock, jc.IsTrue)
	c.Assert(mockLock.noStepsCalledOnLock, jc.IsTrue)

	expectedStepsOnUnlock := []bool{true, false, false}
	c.Assert(mockLock.stepsCalledOnUnlock, gc.DeepEquals, expectedStepsOnUnlock)
}

func (s *ExecutorSuite) TestLockFailsOpsStepsNotCalled(c *gc.C) {
	op := &mockOperation{
		needsLock: true,
		prepare:   newStep(nil, nil),
		execute:   newStep(nil, nil),
		commit:    newStep(nil, nil),
	}

	mockLock := &mockLockFunc{op: op}
	lockFunc := mockLock.newFailingLock()
	executor := s.initLockTest(c, lockFunc)

	err := executor.Run(op)
	c.Assert(err, gc.ErrorMatches, "could not acquire lock: wat")

	c.Assert(mockLock.calledLock, jc.IsFalse)
	c.Assert(mockLock.calledUnlock, jc.IsFalse)
	c.Assert(mockLock.noStepsCalledOnLock, jc.IsTrue)

	c.Assert(op.prepare.called, jc.IsFalse)
	c.Assert(op.execute.called, jc.IsFalse)
	c.Assert(op.commit.called, jc.IsFalse)
}

func (s *ExecutorSuite) testLockUnlocksOnError(c *gc.C, op *mockOperation) (error, *mockLockFunc) {
	mockLock := &mockLockFunc{op: op}
	lockFunc := mockLock.newSucceedingLockUnlockSucceeds()
	executor := s.initLockTest(c, lockFunc)

	err := executor.Run(op)

	c.Assert(mockLock.calledLock, jc.IsTrue)
	c.Assert(mockLock.calledUnlock, jc.IsTrue)
	c.Assert(mockLock.noStepsCalledOnLock, jc.IsTrue)

	return err, mockLock
}

func (s *ExecutorSuite) TestLockUnlocksOnError_Prepare(c *gc.C) {
	op := &mockOperation{
		needsLock: true,
		prepare:   newStep(nil, errors.New("kerblooie")),
		execute:   newStep(nil, nil),
		commit:    newStep(nil, nil),
	}

	err, mockLock := s.testLockUnlocksOnError(c, op)
	c.Assert(err, gc.ErrorMatches, `preparing operation "mock operation": kerblooie`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "kerblooie")

	expectedStepsOnUnlock := []bool{true, false, false}
	c.Assert(mockLock.stepsCalledOnUnlock, gc.DeepEquals, expectedStepsOnUnlock)
}

func (s *ExecutorSuite) TestLockUnlocksOnError_Execute(c *gc.C) {
	op := &mockOperation{
		needsLock: true,
		prepare:   newStep(nil, nil),
		execute:   newStep(nil, errors.New("you asked for it")),
		commit:    newStep(nil, nil),
	}

	err, mockLock := s.testLockUnlocksOnError(c, op)
	c.Assert(err, gc.ErrorMatches, `executing operation "mock operation": you asked for it`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "you asked for it")

	expectedStepsOnUnlock := []bool{true, true, false}
	c.Assert(mockLock.stepsCalledOnUnlock, gc.DeepEquals, expectedStepsOnUnlock)
}

func (s *ExecutorSuite) TestLockUnlocksOnError_Commit(c *gc.C) {
	op := &mockOperation{
		needsLock: true,
		prepare:   newStep(nil, nil),
		execute:   newStep(nil, nil),
		commit:    newStep(nil, errors.New("well, shit")),
	}

	err, mockLock := s.testLockUnlocksOnError(c, op)
	c.Assert(err, gc.ErrorMatches, `committing operation "mock operation": well, shit`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "well, shit")

	expectedStepsOnUnlock := []bool{true, true, true}
	c.Assert(mockLock.stepsCalledOnUnlock, gc.DeepEquals, expectedStepsOnUnlock)
}

type mockLockFunc struct {
	noStepsCalledOnLock bool
	stepsCalledOnUnlock []bool
	calledLock          bool
	calledUnlock        bool
	op                  *mockOperation
}

func (mock *mockLockFunc) newFailingLock() func(string) (func() error, error) {
	return func(string) (func() error, error) {
		mock.noStepsCalledOnLock = mock.op.prepare.called == false &&
			mock.op.commit.called == false &&
			mock.op.prepare.called == false
		return nil, errors.New("wat")
	}

}

func (mock *mockLockFunc) newSucceedingLock(unlockFails bool) func(string) (func() error, error) {
	return func(string) (func() error, error) {
		mock.calledLock = true
		// Ensure that when we lock no operation has been called
		mock.noStepsCalledOnLock = mock.op.prepare.called == false &&
			mock.op.commit.called == false &&
			mock.op.prepare.called == false
		return func() error {
			// Record steps called when unlocking
			mock.stepsCalledOnUnlock = []bool{mock.op.prepare.called,
				mock.op.execute.called,
				mock.op.commit.called}
			mock.calledUnlock = true
			if unlockFails {
				return errors.New("but why")
			}
			return nil
		}, nil
	}
}

func (mock *mockLockFunc) newSucceedingLockUnlockFails() func(string) (func() error, error) {
	return mock.newSucceedingLock(true)
}

func (mock *mockLockFunc) newSucceedingLockUnlockSucceeds() func(string) (func() error, error) {
	return mock.newSucceedingLock(false)
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

func (op *mockOperation) NeedsGlobalMachineLock() bool {
	return op.needsLock
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
