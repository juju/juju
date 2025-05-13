// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/operation/mocks"
	"github.com/juju/juju/rpc/params"
)

type StateOpsSuite struct {
	mockStateRW *mocks.MockUnitStateReadWriter
}

var _ = tc.Suite(&StateOpsSuite{})

func (s *StateOpsSuite) TearDownTest(c *tc.C) {
	s.mockStateRW = nil
}

var stcurl = "ch:quantal/application-name-123"
var relhook = &hook.Info{
	Kind:              hooks.RelationJoined,
	RemoteUnit:        "some-thing/123",
	RemoteApplication: "some-thing",
}

type stateTest struct {
	description string
	st          operation.State
	err         string
}

var stateTests = []stateTest{
	// Invalid op/step.
	{
		description: "unknown operation kind",
		st:          operation.State{Kind: operation.Kind("bloviate")},
		err:         `unknown operation "bloviate"`,
	}, {
		description: "unknown operation step",
		st: operation.State{
			Kind: operation.Continue,
			Step: operation.Step("dudelike"),
		},
		err: `unknown operation step "dudelike"`,
	},
	// Install operation.
	{
		description: "mismatched operation and hook",
		st: operation.State{
			Kind:      operation.Install,
			Installed: true,
			Step:      operation.Pending,
			CharmURL:  stcurl,
			Hook:      &hook.Info{Kind: hooks.ConfigChanged},
		},
		err: `unexpected hook info with Kind Install`,
	}, {
		description: "missing charm URL",
		st: operation.State{
			Kind: operation.Install,
			Step: operation.Pending,
		},
		err: `missing charm URL`,
	}, {
		description: "install with action-id",
		st: operation.State{
			Kind:     operation.Install,
			Step:     operation.Pending,
			CharmURL: stcurl,
			ActionId: &someActionId,
		},
		err: `unexpected action id`,
	}, {
		description: "install with charm url",
		st: operation.State{
			Kind:     operation.Install,
			Step:     operation.Pending,
			CharmURL: stcurl,
		},
	},
	// RunAction operation.
	{
		description: "run action without action id",
		st: operation.State{
			Kind: operation.RunAction,
			Step: operation.Pending,
		},
		err: `missing action id`,
	}, {
		description: "run action with spurious charmURL",
		st: operation.State{
			Kind:     operation.RunAction,
			Step:     operation.Pending,
			ActionId: &someActionId,
			CharmURL: stcurl,
		},
		err: `unexpected charm URL`,
	}, {
		description: "run action with proper action id",
		st: operation.State{
			Kind:     operation.RunAction,
			Step:     operation.Pending,
			ActionId: &someActionId,
		},
	},
	// RunHook operation.
	{
		description: "run-hook with unknown hook",
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.Kind("machine-exploded")},
		},
		err: `unknown hook kind "machine-exploded"`,
	}, {
		description: "run-hook without remote unit",
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.RelationJoined},
		},
		err: `"relation-joined" hook requires a remote unit`,
	}, {
		description: "run-hook relation-joined without remote application",
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{
				Kind:       hooks.RelationJoined,
				RemoteUnit: "some-thing/0",
			},
		},
		err: `"relation-joined" hook has a remote unit but no application`,
	}, {
		description: "run-hook relation-changed without remote application",
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{
				Kind:       hooks.RelationChanged,
				RemoteUnit: "some-thing/0",
			},
		},
		err: `"relation-changed" hook has a remote unit but no application`,
	}, {
		description: "run-hook with actionId",
		st: operation.State{
			Kind:     operation.RunHook,
			Step:     operation.Pending,
			Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			ActionId: &someActionId,
		},
		err: `unexpected action id`,
	}, {
		description: "run-hook with charm URL",
		st: operation.State{
			Kind:     operation.RunHook,
			Step:     operation.Pending,
			Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			CharmURL: stcurl,
		},
		err: `unexpected charm URL`,
	}, {
		description: "run-hook config-changed",
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		},
	}, {
		description: "run-hook relation-joined",
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: relhook,
		},
	},
	// Upgrade operation.
	{
		description: "upgrade without charmURL",
		st: operation.State{
			Kind: operation.Upgrade,
			Step: operation.Pending,
		},
		err: `missing charm URL`,
	}, {
		description: "upgrade with actionID",
		st: operation.State{
			Kind:     operation.Upgrade,
			Step:     operation.Pending,
			CharmURL: stcurl,
			ActionId: &someActionId,
		},
		err: `unexpected action id`,
	}, {
		description: "upgrade operation",
		st: operation.State{
			Kind:     operation.Upgrade,
			Step:     operation.Pending,
			CharmURL: stcurl,
		},
	}, {
		description: "upgrade operation with a relation hook (?)",
		st: operation.State{
			Kind:     operation.Upgrade,
			Step:     operation.Pending,
			Hook:     relhook,
			CharmURL: stcurl,
		},
	},
	// Continue operation.
	{
		description: "continue operation with charmURL",
		st: operation.State{
			Kind:     operation.Continue,
			Step:     operation.Pending,
			CharmURL: stcurl,
		},
		err: `unexpected charm URL`,
	}, {
		description: "continue operation with actionID",
		st: operation.State{
			Kind:     operation.Continue,
			Step:     operation.Pending,
			ActionId: &someActionId,
		},
		err: `unexpected action id`,
	}, {
		description: "continue operation",
		st: operation.State{
			Kind:   operation.Continue,
			Step:   operation.Pending,
			Leader: true,
		},
	},
}

func (s *StateOpsSuite) TestStates(c *tc.C) {
	for i, t := range stateTests {
		c.Logf("test %d: %s", i, t.description)
		s.runTest(c, t)
	}
}

func (s *StateOpsSuite) runTest(c *tc.C, t stateTest) {
	defer s.setupMocks(c).Finish()
	ops := operation.NewStateOps(s.mockStateRW)
	_, err := ops.Read(context.Background())
	c.Assert(err, tc.Equals, operation.ErrNoSavedState)

	if t.err == "" {
		s.expectSetState(c, t.st, t.err)
	}
	err = ops.Write(context.Background(), &t.st)
	if t.err == "" {
		c.Assert(err, tc.ErrorIsNil)
	} else {
		c.Assert(err, tc.ErrorMatches, "invalid operation state: "+t.err)
		s.expectState(c, t.st)
		_, err = ops.Read(context.Background())
		c.Assert(err, tc.ErrorMatches, `validation of uniter state: invalid operation state: `+t.err)
		return
	}
	s.expectState(c, t.st)
	st, err := ops.Read(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(st, tc.DeepEquals, &t.st)
}

func (s *StateOpsSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctlr := gomock.NewController(c)
	s.mockStateRW = mocks.NewMockUnitStateReadWriter(ctlr)

	mExp := s.mockStateRW.EXPECT()
	mExp.State(gomock.Any()).Return(params.UnitStateResult{}, nil)
	return ctlr
}

func (s *StateOpsSuite) expectSetState(c *tc.C, st operation.State, errStr string) {
	data, err := yaml.Marshal(st)
	c.Assert(err, tc.ErrorIsNil)
	strUniterState := string(data)
	if errStr != "" {
		err = errors.New(`validation of uniter state: invalid operation state: ` + errStr)
	}

	mExp := s.mockStateRW.EXPECT()
	mExp.SetState(gomock.Any(), unitStateMatcher{c: c, expected: strUniterState}).Return(err)
}

func (s *StateOpsSuite) expectState(c *tc.C, st operation.State) {
	data, err := yaml.Marshal(st)
	c.Assert(err, tc.ErrorIsNil)
	stStr := string(data)

	mExp := s.mockStateRW.EXPECT()
	mExp.State(gomock.Any()).Return(params.UnitStateResult{UniterState: stStr}, nil)
}

type unitStateMatcher struct {
	c        *tc.C
	expected string
}

func (m unitStateMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(params.SetUnitStateArg)
	if !ok {
		return false
	}

	if obtained.UniterState == nil || m.expected != *obtained.UniterState {
		m.c.Fatalf("unitStateMatcher: expected (%s) obtained (%s)", m.expected, *obtained.UniterState)
		return false
	}

	return true
}

func (m unitStateMatcher) String() string {
	return "Match the contents of the UniterState pointer in params.SetUnitStateArg"
}
