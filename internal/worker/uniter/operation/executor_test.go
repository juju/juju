// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/internal/charm/hooks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/operation/mocks"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/rpc/params"
)

type NewExecutorSuite struct {
	testing.IsolationSuite

	mockStateRW *mocks.MockUnitStateReadWriter
}

var _ = tc.Suite(&NewExecutorSuite{})

func failAcquireLock(_, _ string) (func(), error) {
	return nil, errors.New("wat")
}

func (s *NewExecutorSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *NewExecutorSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctlr := gomock.NewController(c)
	s.mockStateRW = mocks.NewMockUnitStateReadWriter(ctlr)
	return ctlr
}

func (s *NewExecutorSuite) expectState(c *tc.C, st operation.State) {
	data, err := yaml.Marshal(st)
	c.Assert(err, jc.ErrorIsNil)
	stStr := string(data)

	mExp := s.mockStateRW.EXPECT()
	mExp.State(gomock.Any()).Return(params.UnitStateResult{UniterState: stStr}, nil)
}

func (s *NewExecutorSuite) expectStateNil() {
	mExp := s.mockStateRW.EXPECT()
	mExp.State(gomock.Any()).Return(params.UnitStateResult{}, nil)
}

func (s *NewExecutorSuite) TestNewExecutorInvalidStateRead(c *tc.C) {
	defer s.setupMocks(c).Finish()
	initialState := operation.State{Step: operation.Queued}
	s.expectState(c, initialState)
	cfg := operation.ExecutorConfig{
		StateReadWriter: s.mockStateRW,
		InitialState:    initialState,
		AcquireLock:     failAcquireLock,
		Logger:          loggertesting.WrapCheckLog(c),
	}
	executor, err := operation.NewExecutor(context.Background(), "test", cfg)
	c.Assert(executor, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, `validation of uniter state: invalid operation state: .*`)
}

func (s *NewExecutorSuite) TestNewExecutorNoInitialState(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateNil()
	initialState := operation.State{Step: operation.Queued}
	cfg := operation.ExecutorConfig{
		StateReadWriter: s.mockStateRW,
		InitialState:    initialState,
		AcquireLock:     failAcquireLock,
		Logger:          loggertesting.WrapCheckLog(c),
	}
	executor, err := operation.NewExecutor(context.Background(), "test", cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(executor.State(), tc.DeepEquals, initialState)
}

func (s *NewExecutorSuite) TestNewExecutorValidFile(c *tc.C) {
	// note: this content matches valid persistent state as of 1.21; we expect
	// that "hook" will have to become "last-hook" to enable action execution
	// during hook error states. If you do this, please leave at least one test
	// with this form of the yaml in place.
	defer s.setupMocks(c).Finish()
	s.mockStateRW.EXPECT().State(gomock.Any()).Return(params.UnitStateResult{UniterState: "started: true\nop: continue\nopstep: pending\n"}, nil)
	cfg := operation.ExecutorConfig{
		StateReadWriter: s.mockStateRW,
		InitialState:    operation.State{Step: operation.Queued},
		AcquireLock:     failAcquireLock,
		Logger:          loggertesting.WrapCheckLog(c),
	}
	executor, err := operation.NewExecutor(context.Background(), "test", cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(executor.State(), tc.DeepEquals, operation.State{
		Kind:    operation.Continue,
		Step:    operation.Pending,
		Started: true,
	})
}

type ExecutorSuite struct {
	testing.IsolationSuite
	mockStateRW *mocks.MockUnitStateReadWriter
}

var _ = tc.Suite(&ExecutorSuite{})

func (s *ExecutorSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctlr := gomock.NewController(c)
	s.mockStateRW = mocks.NewMockUnitStateReadWriter(ctlr)
	return ctlr
}

func (s *ExecutorSuite) expectSetState(c *tc.C, st operation.State) {
	data, err := yaml.Marshal(st)
	c.Assert(err, jc.ErrorIsNil)
	strUniterState := string(data)

	mExp := s.mockStateRW.EXPECT()
	mExp.SetState(gomock.Any(), unitStateMatcher{c: c, expected: strUniterState}).Return(nil)
}

func (s *ExecutorSuite) expectState(c *tc.C, st operation.State) {
	data, err := yaml.Marshal(st)
	c.Assert(err, jc.ErrorIsNil)
	strState := string(data)

	mExp := s.mockStateRW.EXPECT()
	mExp.State(gomock.Any()).Return(params.UnitStateResult{UniterState: strState}, nil)
}

func (s *ExecutorSuite) expectConfigChangedPendingOp(c *tc.C) operation.State {
	op := operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{Kind: hooks.ConfigChanged},
	}
	s.expectSetState(c, op)
	return op
}

func (s *ExecutorSuite) expectConfigChangedDoneOp(c *tc.C) operation.State {
	op := operation.State{
		Kind: operation.RunHook,
		Step: operation.Done,
		Hook: &hook.Info{Kind: hooks.ConfigChanged},
	}
	s.expectSetState(c, op)
	return op
}

func (s *ExecutorSuite) expectStartQueuedOp(c *tc.C) operation.State {
	op := operation.State{
		Kind: operation.RunHook,
		Step: operation.Queued,
		Hook: &hook.Info{Kind: hooks.Start},
	}
	s.expectSetState(c, op)
	return op
}

func (s *ExecutorSuite) expectStartPendingOp(c *tc.C) operation.State {
	op := operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{Kind: hooks.Start},
	}
	s.expectSetState(c, op)
	return op
}

func (s *ExecutorSuite) newExecutor(c *tc.C, st *operation.State) operation.Executor {
	// ensure s.setupMocks called first.
	c.Assert(s.mockStateRW, tc.NotNil)

	s.expectState(c, *st)
	cfg := operation.ExecutorConfig{
		StateReadWriter: s.mockStateRW,
		InitialState:    operation.State{Step: operation.Queued},
		AcquireLock:     failAcquireLock,
		Logger:          loggertesting.WrapCheckLog(c),
	}
	executor, err := operation.NewExecutor(context.Background(), "test", cfg)
	c.Assert(err, jc.ErrorIsNil)
	return executor
}

func justInstalledState() operation.State {
	return operation.State{
		Kind: operation.Continue,
		Step: operation.Pending,
	}
}

func (s *ExecutorSuite) TestSucceedNoStateChanges(c *tc.C) {
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

	err := executor.Run(context.Background(), op, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(prepare.gotState, tc.DeepEquals, initialState)
	c.Assert(execute.gotState, tc.DeepEquals, initialState)
	c.Assert(commit.gotState, tc.DeepEquals, initialState)
	c.Assert(executor.State(), tc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestSucceedWithStateChanges(c *tc.C) {
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

	err := executor.Run(context.Background(), op, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(prepare.gotState, tc.DeepEquals, initialState)
	c.Assert(execute.gotState, tc.DeepEquals, *prepare.newState)
	c.Assert(commit.gotState, tc.DeepEquals, *execute.newState)
	c.Assert(executor.State(), tc.DeepEquals, *commit.newState)
}

func (s *ExecutorSuite) TestSucceedWithRemoteStateChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	initialState := justInstalledState()
	executor := s.newExecutor(c, &initialState)

	remoteStateUpdated := make(chan struct{}, 1)
	prepare := newStep(nil, nil)
	execute := mockStepFunc(func(ctx context.Context, state operation.State) (*operation.State, error) {
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
			c.Assert(snapshot, tc.DeepEquals, remotestate.Snapshot{
				ConfigHash: "test",
			})
			remoteStateUpdated <- struct{}{}
		},
	}

	rs := make(chan remotestate.Snapshot, 1)
	rs <- remotestate.Snapshot{
		ConfigHash: "test",
	}
	err := executor.Run(context.Background(), op, rs)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ExecutorSuite) TestErrSkipExecute(c *tc.C) {
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

	err := executor.Run(context.Background(), op, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(prepare.gotState, tc.DeepEquals, initialState)
	c.Assert(commit.gotState, tc.DeepEquals, *prepare.newState)
	c.Assert(executor.State(), tc.DeepEquals, *commit.newState)
}

func (s *ExecutorSuite) TestValidateStateChange(c *tc.C) {
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

	err := executor.Run(context.Background(), op, nil)
	c.Assert(err, tc.ErrorMatches, `preparing operation "mock operation" for test: invalid operation state: missing hook info with Kind RunHook`)
	c.Assert(errors.Cause(err), tc.ErrorMatches, "missing hook info with Kind RunHook")
	c.Assert(executor.State(), tc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestFailPrepareNoStateChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	initialState := justInstalledState()
	executor := s.newExecutor(c, &initialState)

	prepare := newStep(nil, errors.New("pow"))
	op := &mockOperation{
		prepare: prepare,
	}

	err := executor.Run(context.Background(), op, nil)
	c.Assert(err, tc.ErrorMatches, `preparing operation "mock operation" for test: pow`)
	c.Assert(errors.Cause(err), tc.ErrorMatches, "pow")

	c.Assert(prepare.gotState, tc.DeepEquals, initialState)
	c.Assert(executor.State(), tc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestFailPrepareWithStateChange(c *tc.C) {
	defer s.setupMocks(c).Finish()
	prepareOp := s.expectStartPendingOp(c)

	initialState := justInstalledState()
	executor := s.newExecutor(c, &initialState)

	prepare := newStep(&prepareOp, errors.New("blam"))
	op := &mockOperation{
		prepare: prepare,
	}

	err := executor.Run(context.Background(), op, nil)
	c.Assert(err, tc.ErrorMatches, `preparing operation "mock operation" for test: blam`)
	c.Assert(errors.Cause(err), tc.ErrorMatches, "blam")

	c.Assert(prepare.gotState, tc.DeepEquals, initialState)
	c.Assert(executor.State(), tc.DeepEquals, *prepare.newState)
}

func (s *ExecutorSuite) TestFailExecuteNoStateChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	initialState := justInstalledState()
	executor := s.newExecutor(c, &initialState)

	prepare := newStep(nil, nil)
	execute := newStep(nil, errors.New("splat"))
	op := &mockOperation{
		prepare: prepare,
		execute: execute,
	}

	err := executor.Run(context.Background(), op, nil)
	c.Assert(err, tc.ErrorMatches, `executing operation "mock operation" for test: splat`)
	c.Assert(errors.Cause(err), tc.ErrorMatches, "splat")

	c.Assert(prepare.gotState, tc.DeepEquals, initialState)
	c.Assert(executor.State(), tc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestFailExecuteWithStateChange(c *tc.C) {
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

	err := executor.Run(context.Background(), op, nil)
	c.Assert(err, tc.ErrorMatches, `executing operation "mock operation" for test: kerblooie`)
	c.Assert(errors.Cause(err), tc.ErrorMatches, "kerblooie")

	c.Assert(prepare.gotState, tc.DeepEquals, initialState)
	c.Assert(executor.State(), tc.DeepEquals, *execute.newState)
}

func (s *ExecutorSuite) TestFailCommitNoStateChange(c *tc.C) {
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

	err := executor.Run(context.Background(), op, nil)
	c.Assert(err, tc.ErrorMatches, `committing operation "mock operation" for test: whack`)
	c.Assert(errors.Cause(err), tc.ErrorMatches, "whack")

	c.Assert(prepare.gotState, tc.DeepEquals, initialState)
	c.Assert(executor.State(), tc.DeepEquals, initialState)
}

func (s *ExecutorSuite) TestFailCommitWithStateChange(c *tc.C) {
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

	err := executor.Run(context.Background(), op, nil)
	c.Assert(err, tc.ErrorMatches, `committing operation "mock operation" for test: take that you bandit`)
	c.Assert(errors.Cause(err), tc.ErrorMatches, "take that you bandit")

	c.Assert(prepare.gotState, tc.DeepEquals, initialState)
	c.Assert(executor.State(), tc.DeepEquals, *commit.newState)
}

func (s *ExecutorSuite) initLockTest(c *tc.C, lockFunc func(string, string) (func(), error)) operation.Executor {
	initialState := justInstalledState()
	err := operation.NewStateOps(s.mockStateRW).Write(context.Background(), &initialState)
	c.Assert(err, jc.ErrorIsNil)
	cfg := operation.ExecutorConfig{
		StateReadWriter: s.mockStateRW,
		InitialState:    operation.State{Step: operation.Queued},
		AcquireLock:     lockFunc,
		Logger:          loggertesting.WrapCheckLog(c),
	}
	executor, err := operation.NewExecutor(context.Background(), "test", cfg)
	c.Assert(err, jc.ErrorIsNil)

	return executor
}

func (s *ExecutorSuite) TestLockSucceedsStepsCalled(c *tc.C) {
	op := &mockOperation{
		needsLock: true,
		prepare:   newStep(nil, nil),
		execute:   newStep(nil, nil),
		commit:    newStep(nil, nil),
	}

	mockLock := &mockLockFunc{op: op}
	lockFunc := mockLock.newSucceedingLock()
	executor := s.initLockTest(c, lockFunc)

	err := executor.Run(context.Background(), op, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(mockLock.calledLock, jc.IsTrue)
	c.Assert(mockLock.calledUnlock, jc.IsTrue)
	c.Assert(mockLock.noStepsCalledOnLock, jc.IsTrue)

	expectedStepsOnUnlock := []bool{true, true, true}
	c.Assert(mockLock.stepsCalledOnUnlock, tc.DeepEquals, expectedStepsOnUnlock)
}

func (s *ExecutorSuite) TestLockFailsOpsStepsNotCalled(c *tc.C) {
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

	err := executor.Run(context.Background(), op, nil)
	c.Assert(err, tc.ErrorMatches, "could not acquire lock: wat")

	c.Assert(mockLock.calledLock, jc.IsFalse)
	c.Assert(mockLock.calledUnlock, jc.IsFalse)
	c.Assert(mockLock.noStepsCalledOnLock, jc.IsTrue)

	c.Assert(prepare.called, jc.IsFalse)
	c.Assert(execute.called, jc.IsFalse)
	c.Assert(commit.called, jc.IsFalse)
}

func (s *ExecutorSuite) testLockUnlocksOnError(c *tc.C, op *mockOperation) (error, *mockLockFunc) {
	mockLock := &mockLockFunc{op: op}
	lockFunc := mockLock.newSucceedingLock()
	executor := s.initLockTest(c, lockFunc)

	err := executor.Run(context.Background(), op, nil)

	c.Assert(mockLock.calledLock, jc.IsTrue)
	c.Assert(mockLock.calledUnlock, jc.IsTrue)
	c.Assert(mockLock.noStepsCalledOnLock, jc.IsTrue)

	return err, mockLock
}

func (s *ExecutorSuite) TestLockUnlocksOnError_Prepare(c *tc.C) {
	op := &mockOperation{
		needsLock: true,
		prepare:   newStep(nil, errors.New("kerblooie")),
		execute:   newStep(nil, nil),
		commit:    newStep(nil, nil),
	}

	err, mockLock := s.testLockUnlocksOnError(c, op)
	c.Assert(err, tc.ErrorMatches, `preparing operation "mock operation": kerblooie`)
	c.Assert(errors.Cause(err), tc.ErrorMatches, "kerblooie")

	expectedStepsOnUnlock := []bool{true, false, false}
	c.Assert(mockLock.stepsCalledOnUnlock, tc.DeepEquals, expectedStepsOnUnlock)
}

func (s *ExecutorSuite) TestLockUnlocksOnError_Execute(c *tc.C) {
	op := &mockOperation{
		needsLock: true,
		prepare:   newStep(nil, nil),
		execute:   newStep(nil, errors.New("you asked for it")),
		commit:    newStep(nil, nil),
	}

	err, mockLock := s.testLockUnlocksOnError(c, op)
	c.Assert(err, tc.ErrorMatches, `executing operation "mock operation": you asked for it`)
	c.Assert(errors.Cause(err), tc.ErrorMatches, "you asked for it")

	expectedStepsOnUnlock := []bool{true, true, false}
	c.Assert(mockLock.stepsCalledOnUnlock, tc.DeepEquals, expectedStepsOnUnlock)
}

func (s *ExecutorSuite) TestLockUnlocksOnError_Commit(c *tc.C) {
	op := &mockOperation{
		needsLock: true,
		prepare:   newStep(nil, nil),
		execute:   newStep(nil, nil),
		commit:    newStep(nil, errors.New("well, shit")),
	}

	err, mockLock := s.testLockUnlocksOnError(c, op)
	c.Assert(err, tc.ErrorMatches, `committing operation "mock operation": well, shit`)
	c.Assert(errors.Cause(err), tc.ErrorMatches, "well, shit")

	expectedStepsOnUnlock := []bool{true, true, true}
	c.Assert(mockLock.stepsCalledOnUnlock, tc.DeepEquals, expectedStepsOnUnlock)
}

type mockLockFunc struct {
	noStepsCalledOnLock bool
	stepsCalledOnUnlock []bool
	calledLock          bool
	calledUnlock        bool
	op                  *mockOperation
}

func (mock *mockLockFunc) newFailingLock() func(string, string) (func(), error) {
	return func(string, string) (func(), error) {
		mock.noStepsCalledOnLock = mock.op.prepare.(*mockStep).called == false &&
			mock.op.commit.(*mockStep).called == false
		return nil, errors.New("wat")
	}
}

func (mock *mockLockFunc) newSucceedingLock() func(string, string) (func(), error) {
	return func(string, string) (func(), error) {
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
	Run(ctx context.Context, state operation.State) (*operation.State, error)
}

type mockStepFunc func(ctx context.Context, state operation.State) (*operation.State, error)

func (m mockStepFunc) Run(ctx context.Context, state operation.State) (*operation.State, error) {
	return m(ctx, state)
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

func (step *mockStep) Run(ctx context.Context, state operation.State) (*operation.State, error) {
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

func (m *mockOperation) ExecutionGroup() string {
	return ""
}

func (op *mockOperation) Prepare(ctx context.Context, state operation.State) (*operation.State, error) {
	return op.prepare.Run(ctx, state)
}

func (op *mockOperation) Execute(ctx context.Context, state operation.State) (*operation.State, error) {
	return op.execute.Run(ctx, state)
}

func (op *mockOperation) Commit(ctx context.Context, state operation.State) (*operation.State, error) {
	return op.commit.Run(ctx, state)
}

func (op *mockOperation) RemoteStateChanged(snapshot remotestate.Snapshot) {
	if op.remoteStateFunc != nil {
		op.remoteStateFunc(snapshot)
	}
}
