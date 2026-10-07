// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actions_test

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/actions"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
)

type actionsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&actionsSuite{})

func (s *actionsSuite) newResolver() resolver.Resolver {
	return actions.NewResolver(loggo.GetLogger("test"), func(_ string) {})
}

func (s *actionsSuite) TestNoActions(c *gc.C) {
	actionResolver := s.newResolver()
	localState := resolver.LocalState{}
	remoteState := remotestate.Snapshot{}
	_, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, gc.DeepEquals, resolver.ErrNoOperation)
}

func (s *actionsSuite) TestActionStateKindContinue(c *gc.C) {
	actionResolver := s.newResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{"actionA", "actionB"},
	}
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operation.Unwrap(op), jc.DeepEquals, mockOp("actionA"))
}

func (s *actionsSuite) TestActionRunHook(c *gc.C) {
	actionResolver := s.newResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
		},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{"actionA", "actionB"},
	}
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operation.Unwrap(op), jc.DeepEquals, mockOp("actionA"))
}

func (s *actionsSuite) TestNextActionBlocked(c *gc.C) {
	actionResolver := s.newResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{"actionA", "actionB"},
		ActionsBlocked: true,
	}
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, gc.DeepEquals, resolver.ErrNoOperation)
	c.Assert(op, gc.IsNil)
}

func (s *actionsSuite) TestNextActionNotAvailable(c *gc.C) {
	actionResolver := s.newResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{"actionA", "actionB"},
	}
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{err: charmrunner.ErrActionNotAvailable})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operation.Unwrap(op), jc.DeepEquals, mockFailAction("actionA"))
}

func (s *actionsSuite) TestNextActionBlockedRemoteInit(c *gc.C) {
	actionResolver := s.newResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
		OutdatedRemoteCharm: true,
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{"actionA", "actionB"},
		ActionsBlocked: false,
	}
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, gc.DeepEquals, resolver.ErrNoOperation)
	c.Assert(op, gc.IsNil)
}

func (s *actionsSuite) TestNextActionBlockedRemoteInitInProgress(c *gc.C) {
	var completed []string
	actionResolver := actions.NewResolver(loggo.GetLogger("test"), func(id string) {
		completed = append(completed, id)
	})
	actionId := "actionB"
	localState := resolver.LocalState{
		State: operation.State{
			Kind:     operation.RunAction,
			ActionId: &actionId,
		},
		OutdatedRemoteCharm: true,
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{"actionA", "actionB"},
		ActionsBlocked: false,
	}
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operation.Unwrap(op), gc.DeepEquals, mockFailAction(actionId))
	c.Check(completed, gc.HasLen, 0)

	_, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(completed, jc.DeepEquals, []string{actionId})
}

func (s *actionsSuite) TestNextActionBlockedRemoteInitSkipHook(c *gc.C) {
	actionResolver := s.newResolver()
	actionId := "actionBad"
	localState := resolver.LocalState{
		State: operation.State{
			Kind:     operation.RunAction,
			ActionId: &actionId,
			Hook:     &hook.Info{Kind: "test"},
		},
		OutdatedRemoteCharm: false,
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{"actionA", "actionB"},
		ActionsBlocked: true,
	}
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op, gc.DeepEquals, mockSkipHook(*localState.Hook))
}

func (s *actionsSuite) TestActionStateKindRunAction(c *gc.C) {
	actionResolver := s.newResolver()
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
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operation.Unwrap(op), jc.DeepEquals, mockOp(actionA))
}

func (s *actionsSuite) TestActionStateKindRunActionSkipHook(c *gc.C) {
	actionResolver := s.newResolver()
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
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op, jc.DeepEquals, mockSkipHook(*localState.Hook))
}

func (s *actionsSuite) TestActionStateKindRunActionPendingRemote(c *gc.C) {
	var completed []string
	actionResolver := actions.NewResolver(loggo.GetLogger("test"), func(id string) {
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
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operation.Unwrap(op), jc.DeepEquals, mockFailAction(actionA))
	c.Check(completed, gc.HasLen, 0)

	_, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(completed, jc.DeepEquals, []string{actionA})
}

func (s *actionsSuite) TestPendingActionNotAvailable(c *gc.C) {
	actionResolver := s.newResolver()
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
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(operation.Unwrap(op), jc.DeepEquals, mockFailAction(actionA))
}

func (s *actionsSuite) TestActionCompletedOnCommit(c *gc.C) {
	var completed []string
	actionResolver := actions.NewResolver(loggo.GetLogger("test"), func(id string) {
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
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(completed, gc.HasLen, 0)

	_, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(completed, jc.DeepEquals, []string{"actionA"})
}

func (s *actionsSuite) TestFailActionCompletedOnCommit(c *gc.C) {
	var completed []string
	actionResolver := actions.NewResolver(loggo.GetLogger("test"), func(id string) {
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
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{err: charmrunner.ErrActionNotAvailable})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(completed, gc.HasLen, 0)

	_, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(completed, jc.DeepEquals, []string{"actionA"})
}

func (s *actionsSuite) TestActionNotCompletedOnCommitError(c *gc.C) {
	var completed []string
	actionResolver := actions.NewResolver(loggo.GetLogger("test"), func(id string) {
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
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{commitErr: errors.New("boom")})
	c.Assert(err, jc.ErrorIsNil)

	_, err = op.Commit(operation.State{})
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Check(completed, gc.HasLen, 0)
}

type mockOperations struct {
	operation.Factory
	err       error
	commitErr error
}

func (m *mockOperations) NewAction(id string) (operation.Operation, error) {
	if m.err != nil {
		return nil, errors.Annotate(m.err, "action error")
	}
	if id == "666" {
		return nil, charmrunner.ErrActionNotAvailable
	}
	return &mockOperation{name: id, commitErr: m.commitErr}, nil
}

func (m *mockOperations) NewFailAction(id string) (operation.Operation, error) {
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

func (op *mockOperation) Commit(st operation.State) (*operation.State, error) {
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

func (op *mockFailOp) Commit(st operation.State) (*operation.State, error) {
	return &st, nil
}

type mockSkipHookOp struct {
	operation.Operation
	hookInfo hook.Info
}

func (op *mockSkipHookOp) String() string {
	return string(op.hookInfo.Kind)
}
