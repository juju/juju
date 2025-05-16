// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm/hooks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/relation"
)

type stateSuite struct {
}

type msi map[string]int64

func TestStateSuite(t *stdtesting.T) { tc.Run(t, &stateSuite{}) }

// writeTests verify the behaviour of sequences of HookInfos on a relation
// state that starts off containing defaultMembers.
var writeTests = []struct {
	description string
	hooks       []hook.Info
	members     msi
	appMembers  msi
	pending     string
	err         string
	deleted     bool
}{
	// Verify that valid changes work.
	{
		description: "relation-changed foo/1 to version 1",
		hooks: []hook.Info{{
			Kind:              hooks.RelationChanged,
			RelationId:        123,
			RemoteUnit:        "foo/1",
			RemoteApplication: "foo",
			ChangeVersion:     1,
		}},
		members: msi{"foo/1": 1, "foo/2": 0},
	}, {
		description: "relation-joined foo/3 at 0",
		hooks: []hook.Info{{
			Kind:              hooks.RelationJoined,
			RelationId:        123,
			RemoteUnit:        "foo/3",
			RemoteApplication: "foo",
		}},
		members: msi{"foo/1": 0, "foo/2": 0, "foo/3": 0},
		pending: "foo/3",
	}, {
		description: "relation-departed foo/1",
		hooks: []hook.Info{{
			Kind:              hooks.RelationDeparted,
			RelationId:        123,
			RemoteUnit:        "foo/1",
			RemoteApplication: "foo",
		}},
		members: msi{"foo/2": 0},
	},
	// Verify detection of various error conditions.
	{
		description: "wrong relation id",
		hooks: []hook.Info{{
			Kind:              hooks.RelationJoined,
			RelationId:        456,
			RemoteUnit:        "foo/1",
			RemoteApplication: "foo",
		}},
		err: "expected relation 123, got relation 456",
	}, {
		description: "relation-joined of a joined unit",
		hooks: []hook.Info{{
			Kind:              hooks.RelationJoined,
			RelationId:        123,
			RemoteUnit:        "foo/1",
			RemoteApplication: "foo",
		}},
		err: "unit already joined",
	}, {
		description: "relation-changed of an unjoined unit",
		hooks: []hook.Info{{
			Kind:              hooks.RelationChanged,
			RelationId:        123,
			RemoteUnit:        "foo/3",
			RemoteApplication: "foo",
		}},
		err: "unit has not joined",
	}, {
		description: "relation-departed of a non-existent unit",
		hooks: []hook.Info{{
			Kind:              hooks.RelationDeparted,
			RelationId:        123,
			RemoteUnit:        "foo/3",
			RemoteApplication: "foo",
		}},
		err: "unit has not joined",
	}, {
		description: "relation-broken with existing units",
		hooks: []hook.Info{{
			Kind:       hooks.RelationBroken,
			RelationId: 123,
		}},
		err: `cannot run "relation-broken" while units still present`,
	},
}

func (s *stateSuite) TestWriteOrValidateSingleHook(c *tc.C) {
	for i, t := range writeTests {
		c.Logf("test %d: %v", i, t.description)
		for i, hi := range t.hooks {
			st := s.setupTestState()
			c.Logf("  hook %d %v %v %v", i, hi.Kind, hi.RemoteUnit, hi.RemoteApplication)
			if i == len(t.hooks)-1 && t.err != "" {
				err := st.Validate(hi)
				expect := fmt.Sprintf(`inappropriate %q for %q: %s`, hi.Kind, hi.RemoteUnit, t.err)
				c.Assert(err, tc.ErrorMatches, expect)
			} else {
				expectedState := s.setupTestState()
				if t.pending != "" {
					expectedState.ChangedPending = t.pending
				}
				if t.members != nil {
					expectedState.Members = t.members

				}
				if t.appMembers != nil {
					expectedState.ApplicationMembers = t.appMembers
				}
				runWriteHookTest(c, st, expectedState, hi)
			}
		}
	}
}

func (s *stateSuite) TestWriteMultiHookJoinedChanged(c *tc.C) {
	c.Log("relation - joined foo / 3 and relation - changed foo / 3")

	// Setup initial state
	st := s.setupTestState()

	// Setup Joined Hook
	hiJoined := hook.Info{
		Kind:              hooks.RelationJoined,
		RelationId:        123,
		RemoteUnit:        "foo/3",
		RemoteApplication: "foo",
	}
	expectedState := s.setupTestState()
	expectedState.Members["foo/3"] = 0
	expectedState.ChangedPending = "foo/3"

	runWriteHookTest(c, st, expectedState, hiJoined)

	// Setup Changed Hook
	hiChanged := hook.Info{
		Kind:              hooks.RelationChanged,
		RelationId:        123,
		RemoteUnit:        "foo/3",
		RemoteApplication: "foo",
	}
	expectedState.ChangedPending = ""

	runWriteHookTest(c, st, expectedState, hiChanged)
}

func (s *stateSuite) TestWriteMultiHookDepartedJoinedJoined(c *tc.C) {
	c.Log("relation-departed foo/1 and relation-joined foo/1")

	// Setup initial state
	st := s.setupTestState()

	// Setup Departed Hook mocks
	hiDeparted := hook.Info{
		Kind:              hooks.RelationDeparted,
		RelationId:        123,
		RemoteUnit:        "foo/1",
		RemoteApplication: "foo",
	}
	expectedState := s.setupTestState()
	delete(expectedState.Members, "foo/1")

	runWriteHookTest(c, st, expectedState, hiDeparted)

	// Setup Changed Hook mocks
	hiJoined := hook.Info{
		Kind:              hooks.RelationJoined,
		RelationId:        123,
		RemoteUnit:        "foo/1",
		RemoteApplication: "foo",
	}
	expectedState.Members["foo/1"] = 0
	expectedState.ChangedPending = "foo/1"

	runWriteHookTest(c, st, expectedState, hiJoined)
}

func (s *stateSuite) TestWriteMultiHookDepartedJoinedChanged(c *tc.C) {
	c.Logf("relation-departed foo/1 and relation-joined foo/1 and relation-changed foo/1")

	// Setup initial state
	st := s.setupTestState()

	// Setup Departed Hook mocks
	hiDeparted := hook.Info{
		Kind:              hooks.RelationDeparted,
		RelationId:        123,
		RemoteUnit:        "foo/1",
		RemoteApplication: "foo",
	}
	expectedState := s.setupTestState()
	delete(expectedState.Members, "foo/1")

	runWriteHookTest(c, st, expectedState, hiDeparted)

	// Setup Joined Hook mocks
	hiJoined := hook.Info{
		Kind:       hooks.RelationJoined,
		RelationId: 123,
		RemoteUnit: "foo/1",
	}
	expectedState.Members["foo/1"] = 0
	expectedState.ChangedPending = "foo/1"

	runWriteHookTest(c, st, expectedState, hiJoined)

	// Setup Changed Hook
	hiChanged := hook.Info{
		Kind:              hooks.RelationChanged,
		RelationId:        123,
		RemoteUnit:        "foo/1",
		RemoteApplication: "foo",
	}
	expectedState.ChangedPending = ""

	runWriteHookTest(c, st, expectedState, hiChanged)
}

func (s *stateSuite) TestWriteMultiHookDepartedDeparted(c *tc.C) {
	c.Logf("relation-departed foo/1 and relation-departed foo/2")

	// Setup initial state
	st := s.setupTestState()

	// Setup Departed Hook mocks
	hiDeparted := hook.Info{
		Kind:              hooks.RelationDeparted,
		RelationId:        123,
		RemoteUnit:        "foo/1",
		RemoteApplication: "foo",
	}
	expectedState := s.setupTestState()
	delete(expectedState.Members, "foo/1")

	runWriteHookTest(c, st, expectedState, hiDeparted)

	// Setup 2nd Departed Hook mocks
	hiDeparted2 := hook.Info{
		Kind:              hooks.RelationDeparted,
		RelationId:        123,
		RemoteUnit:        "foo/2",
		RemoteApplication: "foo",
	}
	delete(expectedState.Members, "foo/2")

	runWriteHookTest(c, st, expectedState, hiDeparted2)
}

func (s *stateSuite) setupTestState() *relation.State {
	return &relation.State{
		RelationId: 123,
		Members: map[string]int64{
			"foo/1": 0,
			"foo/2": 0,
		},
		ApplicationMembers: map[string]int64{
			"foo": 0,
		},
	}
}

func runWriteHookTest(c *tc.C, st, expectedState *relation.State, hi hook.Info) {
	err := st.Validate(hi)
	c.Assert(err, tc.ErrorIsNil)
	logger := loggertesting.WrapCheckLog(c)
	st.UpdateStateForHook(hi, logger)
	c.Assert(*expectedState, tc.DeepEquals, *st)
	// Check that writing the same change again is OK.
	st.UpdateStateForHook(hi, logger)
	c.Assert(*expectedState, tc.DeepEquals, *st)
}

func (s *stateSuite) TestStateValidateErrorJoinedJoined(c *tc.C) {
	c.Logf("relation-changed foo/3 must follow relation-joined foo/3 (before relation-joined of another unit)")
	// Setup 2nd Joined Hook
	hiInfo := hook.Info{
		Kind:              hooks.RelationJoined,
		RelationId:        123,
		RemoteUnit:        "foo/4",
		RemoteApplication: "foo",
	}
	s.testStateValidateErrorAfterJoined(c, hiInfo)
}

func (s *stateSuite) TestStateValidateErrorJoined3Joined1(c *tc.C) {
	c.Logf("relation-changed foo/3 must follow relation-joined foo/3 (not relation-changed for another unit)")
	// Setup relation changed for a different unit.
	hiInfo := hook.Info{
		Kind:              hooks.RelationChanged,
		RelationId:        123,
		RemoteUnit:        "foo/1",
		RemoteApplication: "foo",
	}
	s.testStateValidateErrorAfterJoined(c, hiInfo)
}

func (s *stateSuite) testStateValidateErrorAfterJoined(c *tc.C, hiInfo hook.Info) {
	// Setup state post relation-changed foo/3
	st := &relation.State{
		RelationId: 123,
		Members: map[string]int64{
			"foo/1": 0,
			"foo/2": 0,
			"foo/3": 0,
		},
		ApplicationMembers: map[string]int64{
			"foo": 0,
		},
		ChangedPending: "foo/3",
	}

	// Run hook
	err := st.Validate(hiInfo)
	expect := fmt.Sprintf(`inappropriate %q for %q: expected "relation-changed" for "foo/3"`, hiInfo.Kind, hiInfo.RemoteUnit)
	c.Assert(err, tc.ErrorMatches, expect)
}

func (s *stateSuite) TestStateValidateErrorBrokenJoined(c *tc.C) {
	c.Logf("relation-joined after relation has been broken")

	// Setup state post broken relation app foo
	st := &relation.State{
		RelationId: 123,
	}

	// Setup relation joined on same app
	hiInfo := hook.Info{
		Kind:              hooks.RelationJoined,
		RelationId:        123,
		RemoteUnit:        "foo/1",
		RemoteApplication: "foo",
	}

	err := st.Validate(hiInfo)
	expect := fmt.Sprintf(`inappropriate %q for %q: relation is broken and cannot be changed further`, hiInfo.Kind, hiInfo.RemoteUnit)
	c.Assert(err, tc.ErrorMatches, expect)
}
