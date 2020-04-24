// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/charm/v7/hooks"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

type FailActionSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FailActionSuite{})

func (s *FailActionSuite) TestPrepare(c *gc.C) {
	factory := newOpFactory(nil, nil)
	op, err := factory.NewFailAction(someActionId)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, jc.DeepEquals, &operation.State{
		Kind:     operation.RunAction,
		Step:     operation.Pending,
		ActionId: &someActionId,
	})
}

func (s *FailActionSuite) TestExecuteSuccess(c *gc.C) {
	var stateChangeTests = []struct {
		description string
		before      operation.State
		after       operation.State
	}{{
		description: "empty state",
		after: operation.State{
			Kind:     operation.RunAction,
			Step:     operation.Done,
			ActionId: &someActionId,
		},
	}, {
		description: "preserves appropriate fields",
		before:      overwriteState,
		after: operation.State{
			Kind:     operation.RunAction,
			Step:     operation.Done,
			ActionId: &someActionId,
			Hook:     &hook.Info{Kind: hooks.Install},
			Started:  true,
		},
	}}

	for i, test := range stateChangeTests {
		c.Logf("test %d: %s", i, test.description)
		callbacks := &RunActionCallbacks{MockFailAction: &MockFailAction{}}
		factory := newOpFactory(nil, callbacks)
		op, err := factory.NewFailAction(someActionId)
		c.Assert(err, jc.ErrorIsNil)
		midState, err := op.Prepare(test.before)
		c.Assert(midState, gc.NotNil)
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Execute(*midState)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(newState, jc.DeepEquals, &test.after)
		c.Assert(*callbacks.MockFailAction.gotMessage, gc.Equals, "action terminated")
		c.Assert(*callbacks.MockFailAction.gotActionId, gc.Equals, someActionId)
	}
}

func (s *FailActionSuite) TestExecuteFail(c *gc.C) {
	st := operation.State{
		Kind:     operation.RunAction,
		Step:     operation.Done,
		ActionId: &someActionId,
	}
	callbacks := &RunActionCallbacks{MockFailAction: &MockFailAction{err: errors.New("squelch")}}
	factory := newOpFactory(nil, callbacks)
	op, err := factory.NewFailAction(someActionId)
	c.Assert(err, jc.ErrorIsNil)
	midState, err := op.Prepare(st)
	c.Assert(midState, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = op.Execute(*midState)
	c.Assert(err, gc.ErrorMatches, "squelch")
}

func (s *FailActionSuite) TestCommit(c *gc.C) {
	var stateChangeTests = []struct {
		description string
		before      operation.State
		after       operation.State
	}{{
		description: "empty state",
		after: operation.State{
			Kind: operation.Continue,
			Step: operation.Pending,
		},
	}, {
		description: "preserves only appropriate fields, no hook",
		before: operation.State{
			Kind:     operation.Continue,
			Step:     operation.Pending,
			Started:  true,
			CharmURL: curl("cs:quantal/wordpress-2"),
			ActionId: &randomActionId,
		},
		after: operation.State{
			Kind:    operation.Continue,
			Step:    operation.Pending,
			Started: true,
		},
	}, {
		description: "preserves only appropriate fields, with hook",
		before: operation.State{
			Kind:     operation.Continue,
			Step:     operation.Pending,
			Started:  true,
			CharmURL: curl("cs:quantal/wordpress-2"),
			ActionId: &randomActionId,
			Hook:     &hook.Info{Kind: hooks.Install},
		},
		after: operation.State{
			Kind:    operation.RunHook,
			Step:    operation.Pending,
			Hook:    &hook.Info{Kind: hooks.Install},
			Started: true,
		},
	}}

	for i, test := range stateChangeTests {
		c.Logf("test %d: %s", i, test.description)
		factory := newOpFactory(nil, nil)
		op, err := factory.NewFailAction(someActionId)
		c.Assert(err, jc.ErrorIsNil)

		newState, err := op.Commit(test.before)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(newState, jc.DeepEquals, &test.after)
	}
}

func (s *FailActionSuite) TestNeedsGlobalMachineLock(c *gc.C) {
	factory := newOpFactory(nil, nil)
	op, err := factory.NewFailAction(someActionId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.NeedsGlobalMachineLock(), jc.IsTrue)
}
