// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actions_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/actions"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

type actionsSuite struct{}

var _ = gc.Suite(&actionsSuite{})

func (s *actionsSuite) TestNoActions(c *gc.C) {
	actionResolver := actions.NewResolver()
	localState := resolver.LocalState{}
	remoteState := remotestate.Snapshot{}
	_, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, gc.DeepEquals, resolver.ErrNoOperation)
}

func (s *actionsSuite) TestActionStateKindContinue(c *gc.C) {
	actionResolver := actions.NewResolver()
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
	c.Assert(op, jc.DeepEquals, mockOp("actionA"))
}

func (s *actionsSuite) TestActionRunHook(c *gc.C) {
	actionResolver := actions.NewResolver()
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
	c.Assert(op, jc.DeepEquals, mockOp("actionA"))
}

func (s *actionsSuite) TestNextAction(c *gc.C) {
	actionResolver := actions.NewResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
		CompletedActions: map[string]struct{}{"actionA": {}},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{"actionA", "actionB"},
	}
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op, jc.DeepEquals, mockOp("actionB"))
}

func (s *actionsSuite) TestNextActionBlocked(c *gc.C) {
	actionResolver := actions.NewResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
		CompletedActions: map[string]struct{}{"actionA": {}},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{"actionA", "actionB"},
		ActionsBlocked: true,
	}
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, gc.DeepEquals, resolver.ErrNoOperation)
	c.Assert(op, gc.IsNil)
}

func (s *actionsSuite) TestNextActionBlockedRemoteInit(c *gc.C) {
	actionResolver := actions.NewResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
		CompletedActions:    map[string]struct{}{"actionA": {}},
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

func (s *actionsSuite) TestActionStateKindRunAction(c *gc.C) {
	actionResolver := actions.NewResolver()
	var actionA string = "actionA"

	localState := resolver.LocalState{
		State: operation.State{
			Kind:     operation.RunAction,
			ActionId: &actionA,
		},
		CompletedActions: map[string]struct{}{},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{},
	}
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op, jc.DeepEquals, mockOp("actionA"))
}

func (s *actionsSuite) TestActionStateKindRunActionPendingRemote(c *gc.C) {
	actionResolver := actions.NewResolver()
	var actionA string = "actionA"

	localState := resolver.LocalState{
		State: operation.State{
			Kind:     operation.RunAction,
			ActionId: &actionA,
		},
		CompletedActions: map[string]struct{}{},
	}
	remoteState := remotestate.Snapshot{
		ActionsPending: []string{"actionA", "actionB"},
	}
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op, jc.DeepEquals, mockFailAction("actionA"))
}

type mockOperations struct {
	operation.Factory
}

func (m *mockOperations) NewAction(id string) (operation.Operation, error) {
	return mockOp(id), nil
}

func (m *mockOperations) NewFailAction(id string) (operation.Operation, error) {
	return mockFailAction(id), nil
}

func mockOp(name string) operation.Operation {
	return &mockOperation{name: name}
}

func mockFailAction(name string) operation.Operation {
	return &mockFailOp{name: name}
}

type mockOperation struct {
	operation.Operation
	name string
}

func (op *mockOperation) String() string {
	return op.name
}

type mockFailOp struct {
	operation.Operation
	name string
}

func (op *mockFailOp) String() string {
	return op.name
}
