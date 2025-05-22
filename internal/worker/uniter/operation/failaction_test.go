// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
)

type FailActionSuite struct {
	testhelpers.IsolationSuite
}

func TestFailActionSuite(t *testing.T) {
	tc.Run(t, &FailActionSuite{})
}

func (s *FailActionSuite) TestPrepare(c *tc.C) {
	factory := newOpFactory(c, nil, nil)
	op, err := factory.NewFailAction(someActionId)
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(newState, tc.DeepEquals, &operation.State{
		Kind:     operation.RunAction,
		Step:     operation.Pending,
		ActionId: &someActionId,
	})
}

func (s *FailActionSuite) TestExecuteSuccess(c *tc.C) {
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
		factory := newOpFactory(c, nil, callbacks)
		op, err := factory.NewFailAction(someActionId)
		c.Assert(err, tc.ErrorIsNil)
		midState, err := op.Prepare(c.Context(), test.before)
		c.Assert(midState, tc.NotNil)
		c.Assert(err, tc.ErrorIsNil)

		newState, err := op.Execute(c.Context(), *midState)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(newState, tc.DeepEquals, &test.after)
		c.Assert(*callbacks.MockFailAction.gotMessage, tc.Equals, "action terminated")
		c.Assert(*callbacks.MockFailAction.gotActionId, tc.Equals, someActionId)
	}
}

func (s *FailActionSuite) TestExecuteFail(c *tc.C) {
	st := operation.State{
		Kind:     operation.RunAction,
		Step:     operation.Done,
		ActionId: &someActionId,
	}
	callbacks := &RunActionCallbacks{MockFailAction: &MockFailAction{err: errors.New("squelch")}}
	factory := newOpFactory(c, nil, callbacks)
	op, err := factory.NewFailAction(someActionId)
	c.Assert(err, tc.ErrorIsNil)
	midState, err := op.Prepare(c.Context(), st)
	c.Assert(midState, tc.NotNil)
	c.Assert(err, tc.ErrorIsNil)

	_, err = op.Execute(c.Context(), *midState)
	c.Assert(err, tc.ErrorMatches, "squelch")
}

func (s *FailActionSuite) TestCommit(c *tc.C) {
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
			CharmURL: "ch:quantal/wordpress-2",
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
			CharmURL: "ch:quantal/wordpress-2",
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
		factory := newOpFactory(c, nil, nil)
		op, err := factory.NewFailAction(someActionId)
		c.Assert(err, tc.ErrorIsNil)

		newState, err := op.Commit(c.Context(), test.before)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(newState, tc.DeepEquals, &test.after)
	}
}

func (s *FailActionSuite) TestNeedsGlobalMachineLock(c *tc.C) {
	factory := newOpFactory(c, nil, nil)
	op, err := factory.NewFailAction(someActionId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.NeedsGlobalMachineLock(), tc.IsTrue)
}
