// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6/hooks"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/operation/mocks"
	"github.com/juju/juju/worker/uniter/remotestate"
)

type NewExecutorSuite struct {
	testing.IsolationSuite
	basePath string

	mockStateRW *mocks.MockUnitStateReadWriter
}

var _ = gc.Suite(&NewExecutorSuite{})

func failAcquireLock(_ string) (func(), error) {
	return nil, errors.New("wat")
}

func (s *NewExecutorSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *NewExecutorSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctlr := gomock.NewController(c)
	s.mockStateRW = mocks.NewMockUnitStateReadWriter(ctlr)
	return ctlr
}

func (s *NewExecutorSuite) expectState(c *gc.C, st operation.State) {
	data, err := yaml.Marshal(st)
	c.Assert(err, jc.ErrorIsNil)
	stStr := string(data)

	mExp := s.mockStateRW.EXPECT()
	mExp.State().Return(params.UnitStateResult{UniterState: stStr}, nil)
}

func (s *NewExecutorSuite) expectStateNil() {
	mExp := s.mockStateRW.EXPECT()
	mExp.State().Return(params.UnitStateResult{}, nil)
}

func (s *NewExecutorSuite) TestNewExecutorInvalidStateRead(c *gc.C) {
	defer s.setupMocks(c).Finish()
	initialState := operation.State{Step: operation.Queued}
	s.expectState(c, initialState)
	cfg := operation.ExecutorConfig{
		StateReadWriter: s.mockStateRW,
		InitialState:    initialState,
		AcquireLock:     failAcquireLock,
	}
	executor, err := operation.NewExecutor(cfg)
	c.Assert(executor, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `validation of uniter state: invalid operation state: .*`)
}

func (s *NewExecutorSuite) TestNewExecutorNoInitialState(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateNil()
	initialState := operation.State{Step: operation.Queued}
	cfg := operation.ExecutorConfig{
		StateReadWriter: s.mockStateRW,
		InitialState:    initialState,
		AcquireLock:     failAcquireLock}
	executor, err := operation.NewExecutor(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(executor.State(), gc.DeepEquals, initialState)
}

func (s *NewExecutorSuite) TestNewExecutorValidFile(c *gc.C) {
	// note: this content matches valid persistent state as of 1.21; we expect
	// that "hook" will have to become "last-hook" to enable action execution
	// during hook error states. If you do this, please leave at least one test
	// with this form of the yaml in place.
	defer s.setupMocks(c).Finish()
	s.mockStateRW.EXPECT().State().Return(params.UnitStateResult{UniterState: "started: true\nop: continue\nopstep: pending\n"}, nil)
	cfg := operation.ExecutorConfig{
		StateReadWriter: s.mockStateRW,
		InitialState:    operation.State{Step: operation.Queued},
		AcquireLock:     failAcquireLock,
	}
	executor, err := operation.NewExecutor(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(executor.State(), gc.DeepEquals, operation.State{
		Kind:    operation.Continue,
		Step:    operation.Pending,
		Started: true,
	})
}

type ExecutorSuite struct {
	testing.IsolationSuite
	mockStateRW *mocks.MockUnitStateReadWriter
}

var _ = gc.Suite(&ExecutorSuite{})

func (s *ExecutorSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctlr := gomock.NewController(c)
	s.mockStateRW = mocks.NewMockUnitStateReadWriter(ctlr)
	return ctlr
}

func (s *ExecutorSuite) expectSetState(c *gc.C, st operation.State) {
	data, err := yaml.Marshal(st)
	c.Assert(err, jc.ErrorIsNil)
	strUniterState := string(data)

	mExp := s.mockStateRW.EXPECT()
	mExp.SetState(unitStateMatcher{c: c, expected: strUniterState}).Return(nil)
}

func (s *ExecutorSuite) expectState(c *gc.C, st operation.State) {
	data, err := yaml.Marshal(st)
	c.Assert(err, jc.ErrorIsNil)
	strState := string(data)

	mExp := s.mockStateRW.EXPECT()
	mExp.State().Return(params.UnitStateResult{UniterState: strState}, nil)
}

func (s *ExecutorSuite) expectConfigChangedPendingOp(c *gc.C) operation.State {
	op := operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{Kind: hooks.ConfigChanged},
	}
	s.expectSetState(c, op)
	return op
}

func (s *ExecutorSuite) expectConfigChangedDoneOp(c *gc.C) operation.State {
	op := operation.State{
		Kind: operation.RunHook,
		Step: operation.Done,
		Hook: &hook.Info{Kind: hooks.ConfigChanged},
	}
	s.expectSetState(c, op)
	return op
}

func (s *ExecutorSuite) expectStartQueuedOp(c *gc.C) operation.State {
	op := operation.State{
		Kind: operation.RunHook,
		Step: operation.Queued,
		Hook: &hook.Info{Kind: hooks.Start},
	}
	s.expectSetState(c, op)
	return op
}

func (s *ExecutorSuite) expectStartPendingOp(c *gc.C) operation.State {
	op := operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{Kind: hooks.Start},
	}
	s.expectSetState(c, op)
	return op
}

func (s *ExecutorSuite) newExecutor(c *gc.C, st *operation.State) operation.Executor {
	// ensure s.setupMocks called first.
	c.Assert(s.mockStateRW, gc.NotNil)

	s.expectState(c, *st)
	cfg := operation.ExecutorConfig{
		StateReadWriter: s.mockStateRW,
		InitialState:    operation.State{Step: operation.Queued},
		AcquireLock:     failAcquireLock,
	}
	executor, err := operation.NewExecutor(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return executor
}

func justInstalledState() operation.State {
	return operation.State{
		Kind: operation.Continue,
		Step: operation.Pending,
	}
}

func (s *ExecutorSuite) TestSucceedNoStateChanges(c *gc.C) {
	defer s.setupMocks(c).Finish()
	initialState := justInstalledState()
	executor := s.newExecutor(c, &initialState)

	prepare := newStep(nil, nil)
	execute := newStep(nil, nil)
	commit := newStep(nil, nil)
	op := &mockOperation{
		prepare: prepare,
		execute: execute,
		commit:  commit,
	}

	err := executor.Run(op, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(execute.gotState, gc.DeepEquals, initialState)
	c.Assert(commit.gotState, gc.DeepEquals, initialState)
	c.Assert(executor.State(), gc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestSucceedWithStateChanges(c *gc.C) {
	defer s.setupMocks(c).Finish()

	prepareOp := s.expectConfigChangedPendingOp(c)
	executeOp := s.expectConfigChangedDoneOp(c)
	commitOp := s.expectStartQueuedOp(c)

	initialState := justInstalledState()
	executor := s.newExecutor(c, &initialState)

	prepare := newStep(&prepareOp, nil)
	execute := newStep(&executeOp, nil)
	commit := newStep(&commitOp, nil)
	op := &mockOperation{
		prepare: prepare,
		execute: execute,
		commit:  commit,
	}

	err := executor.Run(op, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(execute.gotState, gc.DeepEquals, *prepare.newState)
	c.Assert(commit.gotState, gc.DeepEquals, *execute.newState)
	c.Assert(executor.State(), gc.DeepEquals, *commit.newState)
}

func (s *ExecutorSuite) TestSucceedWithRemoteStateChanges(c *gc.C) {
	defer s.setupMocks(c).Finish()

	initialState := justInstalledState()
	executor := s.newExecutor(c, &initialState)

	remoteStateUpdated := make(chan struct{}, 1)
	prepare := newStep(nil, nil)
	execute := mockStepFunc(func(state operation.State) (*operation.State, error) {
		select {
		case <-remoteStateUpdated:
			return nil, nil
		case <-time.After(testing.ShortWait):
			c.Fatal("remote state wasn't updated")
			return nil, nil
		}
	})
	commit := newStep(nil, nil)
	op := &mockOperation{
		prepare: prepare,
		execute: execute,
		commit:  commit,
		remoteStateFunc: func(snapshot remotestate.Snapshot) {
			c.Assert(snapshot, gc.DeepEquals, remotestate.Snapshot{
				ConfigHash: "test",
			})
			remoteStateUpdated <- struct{}{}
		},
	}

	rs := make(chan remotestate.Snapshot, 1)
	rs <- remotestate.Snapshot{
		ConfigHash: "test",
	}
	err := executor.Run(op, rs)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ExecutorSuite) TestErrSkipExecute(c *gc.C) {
	defer s.setupMocks(c).Finish()

	prepareOp := s.expectConfigChangedPendingOp(c)
	commitOp := s.expectStartQueuedOp(c)

	initialState := justInstalledState()
	executor := s.newExecutor(c, &initialState)

	prepare := newStep(&prepareOp, operation.ErrSkipExecute)
	commit := newStep(&commitOp, nil)
	op := &mockOperation{
		prepare: prepare,
		commit:  commit,
	}

	err := executor.Run(op, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(commit.gotState, gc.DeepEquals, *prepare.newState)
	c.Assert(executor.State(), gc.DeepEquals, *commit.newState)
}

func (s *ExecutorSuite) TestValidateStateChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	initialState := justInstalledState()
	executor := s.newExecutor(c, &initialState)

	prepare := newStep(&operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
	}, nil)
	op := &mockOperation{
		prepare: prepare,
	}

	err := executor.Run(op, nil)
	c.Assert(err, gc.ErrorMatches, `preparing operation "mock operation": invalid operation state: missing hook info with Kind RunHook`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "missing hook info with Kind RunHook")
	c.Assert(executor.State(), gc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestFailPrepareNoStateChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	initialState := justInstalledState()
	executor := s.newExecutor(c, &initialState)

	prepare := newStep(nil, errors.New("pow"))
	op := &mockOperation{
		prepare: prepare,
	}

	err := executor.Run(op, nil)
	c.Assert(err, gc.ErrorMatches, `preparing operation "mock operation": pow`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "pow")

	c.Assert(prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(executor.State(), gc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestFailPrepareWithStateChange(c *gc.C) {
	defer s.setupMocks(c).Finish()
	prepareOp := s.expectStartPendingOp(c)

	initialState := justInstalledState()
	executor := s.newExecutor(c, &initialState)

	prepare := newStep(&prepareOp, errors.New("blam"))
	op := &mockOperation{
		prepare: prepare,
	}

	err := executor.Run(op, nil)
	c.Assert(err, gc.ErrorMatches, `preparing operation "mock operation": blam`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "blam")

	c.Assert(prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(executor.State(), gc.DeepEquals, *prepare.newState)
}

func (s *ExecutorSuite) TestFailExecuteNoStateChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	initialState := justInstalledState()
	executor := s.newExecutor(c, &initialState)

	prepare := newStep(nil, nil)
	execute := newStep(nil, errors.New("splat"))
	op := &mockOperation{
		prepare: prepare,
		execute: execute,
	}

	err := executor.Run(op, nil)
	c.Assert(err, gc.ErrorMatches, `executing operation "mock operation": splat`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "splat")

	c.Assert(prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(executor.State(), gc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestFailExecuteWithStateChange(c *gc.C) {
	defer s.setupMocks(c).Finish()
	executeOp := s.expectStartPendingOp(c)

	initialState := justInstalledState()
	executor := s.newExecutor(c, &initialState)

	prepare := newStep(nil, nil)
	execute := newStep(&executeOp, errors.New("kerblooie"))
	op := &mockOperation{
		prepare: prepare,
		execute: execute,
	}

	err := executor.Run(op, nil)
	c.Assert(err, gc.ErrorMatches, `executing operation "mock operation": kerblooie`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "kerblooie")

	c.Assert(prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(executor.State(), gc.DeepEquals, *execute.newState)
}

func (s *ExecutorSuite) TestFailCommitNoStateChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	initialState := justInstalledState()
	executor := s.newExecutor(c, &initialState)

	prepare := newStep(nil, nil)
	execute := newStep(nil, nil)
	commit := newStep(nil, errors.New("whack"))
	op := &mockOperation{
		prepare: prepare,
		execute: execute,
		commit:  commit,
	}

	err := executor.Run(op, nil)
	c.Assert(err, gc.ErrorMatches, `committing operation "mock operation": whack`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "whack")

	c.Assert(prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(executor.State(), gc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestFailCommitWithStateChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	initialState := justInstalledState()
	commitOp := s.expectStartPendingOp(c)

	executor := s.newExecutor(c, &initialState)
	prepare := newStep(nil, nil)
	execute := newStep(nil, nil)
	commit := newStep(&commitOp, errors.New("take that you bandit"))
	op := &mockOperation{
		prepare: prepare,
		execute: execute,
		commit:  commit,
	}

	err := executor.Run(op, nil)
	c.Assert(err, gc.ErrorMatches, `committing operation "mock operation": take that you bandit`)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "take that you bandit")

	c.Assert(prepare.gotState, gc.DeepEquals, initialState)
	c.Assert(executor.State(), gc.DeepEquals, *commit.newState)
}

func (s *ExecutorSuite) initLockTest(c *gc.C, lockFunc func(string) (func(), error)) operation.Executor {
	initialState := justInstalledState()
	err := operation.NewStateOps(s.mockStateRW).Write(&initialState)
	c.Assert(err, jc.ErrorIsNil)
	cfg := operation.ExecutorConfig{
		StateReadWriter: s.mockStateRW,
		InitialState:    operation.State{Step: operation.Queued},
		AcquireLock:     lockFunc,
	}
	executor, err := operation.NewExecutor(cfg)
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
	lockFunc := mockLock.newSucceedingLock()
	executor := s.initLockTest(c, lockFunc)

	err := executor.Run(op, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(mockLock.calledLock, jc.IsTrue)
	c.Assert(mockLock.calledUnlock, jc.IsTrue)
	c.Assert(mockLock.noStepsCalledOnLock, jc.IsTrue)

	expectedStepsOnUnlock := []bool{true, true, true}
	c.Assert(mockLock.stepsCalledOnUnlock, gc.DeepEquals, expectedStepsOnUnlock)
}

func (s *ExecutorSuite) TestLockFailsOpsStepsNotCalled(c *gc.C) {
	prepare := newStep(nil, nil)
	execute := newStep(nil, nil)
	commit := newStep(nil, nil)
	op := &mockOperation{
		needsLock: true,
		prepare:   prepare,
		execute:   execute,
		commit:    commit,
	}

	mockLock := &mockLockFunc{op: op}
	lockFunc := mockLock.newFailingLock()
	executor := s.initLockTest(c, lockFunc)

	err := executor.Run(op, nil)
	c.Assert(err, gc.ErrorMatches, "could not acquire lock: wat")

	c.Assert(mockLock.calledLock, jc.IsFalse)
	c.Assert(mockLock.calledUnlock, jc.IsFalse)
	c.Assert(mockLock.noStepsCalledOnLock, jc.IsTrue)

	c.Assert(prepare.called, jc.IsFalse)
	c.Assert(execute.called, jc.IsFalse)
	c.Assert(commit.called, jc.IsFalse)
}

func (s *ExecutorSuite) testLockUnlocksOnError(c *gc.C, op *mockOperation) (error, *mockLockFunc) {
	mockLock := &mockLockFunc{op: op}
	lockFunc := mockLock.newSucceedingLock()
	executor := s.initLockTest(c, lockFunc)

	err := executor.Run(op, nil)

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

func (mock *mockLockFunc) newFailingLock() func(string) (func(), error) {
	return func(string) (func(), error) {
		mock.noStepsCalledOnLock = mock.op.prepare.(*mockStep).called == false &&
			mock.op.commit.(*mockStep).called == false
		return nil, errors.New("wat")
	}
}

func (mock *mockLockFunc) newSucceedingLock() func(string) (func(), error) {
	return func(string) (func(), error) {
		mock.calledLock = true
		// Ensure that when we lock no operation has been called
		mock.noStepsCalledOnLock = mock.op.prepare.(*mockStep).called == false &&
			mock.op.commit.(*mockStep).called == false
		return func() {
			// Record steps called when unlocking
			mock.stepsCalledOnUnlock = []bool{mock.op.prepare.(*mockStep).called,
				mock.op.execute.(*mockStep).called,
				mock.op.commit.(*mockStep).called}
			mock.calledUnlock = true
		}, nil
	}
}

type mockStepInterface interface {
	Run(state operation.State) (*operation.State, error)
}

type mockStepFunc func(state operation.State) (*operation.State, error)

func (m mockStepFunc) Run(state operation.State) (*operation.State, error) {
	return m(state)
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

func (step *mockStep) Run(state operation.State) (*operation.State, error) {
	step.called = true
	step.gotState = state
	return step.newState, step.err
}

type mockOperation struct {
	needsLock       bool
	prepare         mockStepInterface
	execute         mockStepInterface
	commit          mockStepInterface
	remoteStateFunc func(snapshot remotestate.Snapshot)
}

func (op *mockOperation) String() string {
	return "mock operation"
}

func (op *mockOperation) NeedsGlobalMachineLock() bool {
	return op.needsLock
}

func (op *mockOperation) Prepare(state operation.State) (*operation.State, error) {
	return op.prepare.Run(state)
}

func (op *mockOperation) Execute(state operation.State) (*operation.State, error) {
	return op.execute.Run(state)
}

func (op *mockOperation) Commit(state operation.State) (*operation.State, error) {
	return op.commit.Run(state)
}

func (op *mockOperation) RemoteStateChanged(snapshot remotestate.Snapshot) {
	if op.remoteStateFunc != nil {
		op.remoteStateFunc(snapshot)
	}
}
