// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actions_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/actions"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
)

type actionsSuite struct {
	testhelpers.IsolationSuite
}

func TestActionsSuite(t *testing.T) {
	tc.Run(t, &actionsSuite{})
}

func (s *actionsSuite) newResolver(c *tc.C) resolver.Resolver {
	return actions.NewResolver(loggertesting.WrapCheckLog(c), func(_ string) {})
}

func (s *actionsSuite) TestNoActions(c *tc.C) {
	actionResolver := s.newResolver(c)
	localState := resolver.LocalState{}
	remoteState := remotestate.Snapshot{}
	_, err := actionResolver.NextOp(c.Context(), localState, remoteState, &mockOperations{})
	c.Assert(err, tc.DeepEquals, resolver.ErrNoOperation)
}

func (s *actionsSuite) TestActionStateKindContinue(c *tc.C) {
	actionResolver := s.newResolver(c)
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{"actionA", "actionB"},
	}
	op, err := actionResolver.NextOp(c.Context(), localState, remoteState, &mockOperations{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operation.Unwrap(op), tc.DeepEquals, mockOp("actionA"))
}

func (s *actionsSuite) TestActionRunHook(c *tc.C) {
	actionResolver := s.newResolver(c)
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
		},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{"actionA", "actionB"},
	}
	op, err := actionResolver.NextOp(c.Context(), localState, remoteState, &mockOperations{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operation.Unwrap(op), tc.DeepEquals, mockOp("actionA"))
}

func (s *actionsSuite) TestNextActionNotAvailable(c *tc.C) {
	actionResolver := s.newResolver(c)
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{"actionA", "actionB"},
	}
	op, err := actionResolver.NextOp(c.Context(), localState, remoteState, &mockOperations{err: charmrunner.ErrActionNotAvailable})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operation.Unwrap(op), tc.DeepEquals, mockFailAction("actionA"))
}

func (s *actionsSuite) TestActionStateKindRunAction(c *tc.C) {
	actionResolver := s.newResolver(c)
	actionA := "actionA"

	localState := resolver.LocalState{
		State: operation.State{
			Kind:     operation.RunAction,
			ActionId: &actionA,
		},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{},
	}
	op, err := actionResolver.NextOp(c.Context(), localState, remoteState, &mockOperations{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operation.Unwrap(op), tc.DeepEquals, mockOp(actionA))
}

func (s *actionsSuite) TestActionStateKindRunActionSkipHook(c *tc.C) {
	actionResolver := s.newResolver(c)
	actionA := "actionA"

	localState := resolver.LocalState{
		State: operation.State{
			Kind:     operation.RunAction,
			ActionId: &actionA,
			Hook:     &hook.Info{Kind: "test"},
		},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{},
	}
	op, err := actionResolver.NextOp(c.Context(), localState, remoteState, &mockOperations{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op, tc.DeepEquals, mockSkipHook(*localState.Hook))
}

func (s *actionsSuite) TestActionStateKindRunActionPendingRemote(c *tc.C) {
	var completed []string
	actionResolver := actions.NewResolver(loggertesting.WrapCheckLog(c), func(id string) {
		completed = append(completed, id)
	})
	actionA := "actionA"

	localState := resolver.LocalState{
		State: operation.State{
			Kind:     operation.RunAction,
			ActionId: &actionA,
		},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{actionA, "actionB"},
	}
	op, err := actionResolver.NextOp(c.Context(), localState, remoteState, &mockOperations{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operation.Unwrap(op), tc.DeepEquals, mockFailAction(actionA))
	c.Check(completed, tc.HasLen, 0)

	_, err = op.Commit(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(completed, tc.DeepEquals, []string{actionA})
}

func (s *actionsSuite) TestPendingActionNotAvailable(c *tc.C) {
	actionResolver := s.newResolver(c)
	actionA := "666"

	localState := resolver.LocalState{
		State: operation.State{
			Kind:     operation.RunAction,
			Step:     operation.Pending,
			ActionId: &actionA,
		},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{"666"},
	}
	op, err := actionResolver.NextOp(c.Context(), localState, remoteState, &mockOperations{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(operation.Unwrap(op), tc.DeepEquals, mockFailAction(actionA))
}

func (s *actionsSuite) TestActionCompletedOnCommit(c *tc.C) {
	var completed []string
	actionResolver := actions.NewResolver(loggertesting.WrapCheckLog(c), func(id string) {
		completed = append(completed, id)
	})
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{"actionA", "actionB"},
	}
	op, err := actionResolver.NextOp(c.Context(), localState, remoteState, &mockOperations{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(completed, tc.HasLen, 0)

	_, err = op.Commit(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(completed, tc.DeepEquals, []string{"actionA"})
}

func (s *actionsSuite) TestFailActionCompletedOnCommit(c *tc.C) {
	var completed []string
	actionResolver := actions.NewResolver(loggertesting.WrapCheckLog(c), func(id string) {
		completed = append(completed, id)
	})
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{"actionA", "actionB"},
	}
	op, err := actionResolver.NextOp(c.Context(), localState, remoteState, &mockOperations{err: charmrunner.ErrActionNotAvailable})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(completed, tc.HasLen, 0)

	_, err = op.Commit(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(completed, tc.DeepEquals, []string{"actionA"})
}

func (s *actionsSuite) TestActionNotCompletedOnCommitError(c *tc.C) {
	var completed []string
	actionResolver := actions.NewResolver(loggertesting.WrapCheckLog(c), func(id string) {
		completed = append(completed, id)
	})
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{"actionA"},
	}
	op, err := actionResolver.NextOp(c.Context(), localState, remoteState, &mockOperations{commitErr: errors.New("boom")})
	c.Assert(err, tc.ErrorIsNil)

	_, err = op.Commit(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorMatches, "boom")
	c.Check(completed, tc.HasLen, 0)
}

type mockOperations struct {
	operation.Factory
	err       error
	commitErr error
}

func (m *mockOperations) NewAction(_ context.Context, id string) (operation.Operation, error) {
	if m.err != nil {
		return nil, errors.Annotate(m.err, "action error")
	}
	if id == "666" {
		return nil, charmrunner.ErrActionNotAvailable
	}
	return &mockOperation{name: id, commitErr: m.commitErr}, nil
}

func (m *mockOperations) NewFailAction(_ context.Context, id string) (operation.Operation, error) {
	return mockFailAction(id), nil
}

func (m *mockOperations) NewSkipHook(hookInfo hook.Info) (operation.Operation, error) {
	return mockSkipHook(hookInfo), nil
}

func mockOp(name string) operation.Operation {
	return &mockOperation{name: name}
}

func mockFailAction(name string) operation.Operation {
	return &mockFailOp{name: name}
}

func mockSkipHook(hookInfo hook.Info) operation.Operation {
	return &mockSkipHookOp{hookInfo: hookInfo}
}

type mockOperation struct {
	operation.Operation
	name      string
	commitErr error
}

func (op *mockOperation) String() string {
	return op.name
}

func (op *mockOperation) Commit(_ context.Context, st operation.State) (*operation.State, error) {
	if op.commitErr != nil {
		return nil, op.commitErr
	}
	return &st, nil
}

type mockFailOp struct {
	operation.Operation
	name string
}

func (op *mockFailOp) String() string {
	return op.name
}

func (op *mockFailOp) Commit(_ context.Context, st operation.State) (*operation.State, error) {
	return &st, nil
}

type mockSkipHookOp struct {
	operation.Operation
	hookInfo hook.Info
}

func (op *mockSkipHookOp) String() string {
	return string(op.hookInfo.Kind)
}
