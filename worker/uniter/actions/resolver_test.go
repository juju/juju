// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actions_test

import (
	"fmt"

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
		Actions: []string{"actionA", "actionB"},
	}
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "action actionA")
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
		Actions: []string{"actionA", "actionB"},
	}
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "action actionA")
}

func (s *actionsSuite) TestNextAction(c *gc.C) {
	actionResolver := actions.NewResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
		CompletedActions: map[string]struct{}{"actionA": struct{}{}},
	}
	remoteState := remotestate.Snapshot{
		Actions: []string{"actionA", "actionB"},
	}
	op, err := actionResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "action actionB")
}

type mockOperations struct {
	operation.Factory
}

func (m *mockOperations) NewAction(id string) (operation.Operation, error) {
	return &mockOperation{fmt.Sprintf("action %s", id)}, nil
}

type mockOperation struct {
	name string
}

func (m *mockOperation) String() string {
	return m.name
}

func (m *mockOperation) NeedsGlobalMachineLock() bool {
	return false
}

func (m *mockOperation) Prepare(state operation.State) (*operation.State, error) {
	return &state, nil
}

func (m *mockOperation) Execute(state operation.State) (*operation.State, error) {
	return &state, nil
}

func (m *mockOperation) Commit(state operation.State) (*operation.State, error) {
	return &state, nil
}
