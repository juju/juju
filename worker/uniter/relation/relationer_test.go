// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v7/hooks"
	"github.com/juju/errors"
	"github.com/juju/juju/api"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	apiuniter "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation/mocks"
	"github.com/juju/juju/worker/uniter/relation"
)

type relationerSuite struct {
	jujutesting.JujuConnSuite
	hooks chan hook.Info
	app   *state.Application
	rel   *state.Relation
	mgr   relation.StateManager

	st      api.Connection
	uniter  *apiuniter.State
	relUnit relation.RelationUnit
}

var _ = gc.Suite(&relationerSuite{})

func (s *relationerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var err error
	s.app = s.AddTestingApplication(c, "u", s.AddTestingCharm(c, "riak"))
	c.Assert(err, jc.ErrorIsNil)
	rels, err := s.app.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rels, gc.HasLen, 1)
	s.rel = rels[0]
	_, unit := s.AddRelationUnit(c, "u/0")
	s.hooks = make(chan hook.Info)

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAs(c, unit.Tag(), password)
	s.uniter, err = s.st.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.uniter, gc.NotNil)

	apiUnit, err := s.uniter.Unit(unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	apiRel, err := s.uniter.Relation(s.rel.Tag().(names.RelationTag))
	c.Assert(err, jc.ErrorIsNil)
	apiRelUnit, err := apiRel.Unit(apiUnit.Tag())
	c.Assert(err, jc.ErrorIsNil)
	s.relUnit = &relation.RelationUnitShim{apiRelUnit}
	s.mgr, err = relation.NewStateManager(apiUnit)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationerSuite) AddRelationUnit(c *gc.C, name string) (*state.RelationUnit, *state.Unit) {
	u, err := s.app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.Name(), gc.Equals, name)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	privateAddr := network.NewScopedSpaceAddress(
		strings.Replace(name, "/", "-", 1)+".testing.invalid", network.ScopeCloudLocal,
	)
	err = machine.SetProviderAddresses(privateAddr)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := s.rel.Unit(u)
	c.Assert(err, jc.ErrorIsNil)
	return ru, u
}

func (s *relationerSuite) TestEnterLeaveScope(c *gc.C) {
	ru1, _ := s.AddRelationUnit(c, "u/1")
	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())
	r := relation.NewRelationer(s.relUnit, s.mgr)

	w := ru1.Watch()
	// u/1 does not consider u/0 to be alive.
	defer stop(c, w)
	rc := statetesting.NewRelationUnitsWatcherC(c, s.State, w)
	rc.AssertChange(nil, []string{"u"}, nil)

	// u/0 enters scope; u/1 observes it.
	err := r.Join()
	c.Assert(err, jc.ErrorIsNil)
	rc.AssertChange([]string{"u/0"}, nil, nil)

	// re-Join is no-op.
	err = r.Join()
	c.Assert(err, jc.ErrorIsNil)
	rc.AssertNoChange()

	// u/0 leaves scope; u/1 observes it.
	hi := hook.Info{Kind: hooks.RelationBroken}
	_, err = r.PrepareHook(hi)
	c.Assert(err, jc.ErrorIsNil)

	err = r.CommitHook(hi)
	c.Assert(err, jc.ErrorIsNil)
	rc.AssertChange(nil, nil, []string{"u/0"})
}

func (s *relationerSuite) TestPrepareCommitHooks(c *gc.C) {
	r := relation.NewRelationer(s.relUnit, s.mgr)
	err := r.Join()
	c.Assert(err, jc.ErrorIsNil)

	assertMembers := func(expect map[string]int64) {
		st, err := s.mgr.Relation(s.relUnit.Relation().Id())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(st.Members, jc.DeepEquals, expect)
		expectNames := make([]string, 0, len(expect))
		for name := range expect {
			expectNames = append(expectNames, name)
		}
		c.Assert(r.ContextInfo().MemberNames, jc.SameContents, expectNames)
	}
	assertMembers(map[string]int64{})

	// Check preparing an invalid hook changes nothing.
	changed := hook.Info{
		Kind:              hooks.RelationChanged,
		RemoteUnit:        "u/1",
		RemoteApplication: "u",
		ChangeVersion:     7,
	}
	_, err = r.PrepareHook(changed)
	c.Assert(err, gc.ErrorMatches, `inappropriate "relation-changed" for "u/1": unit has not joined`)
	assertMembers(map[string]int64{})

	// Check preparing a valid hook updates neither the context nor persistent
	// relation state.
	joined := hook.Info{
		Kind:       hooks.RelationJoined,
		RemoteUnit: "u/1",
	}
	name, err := r.PrepareHook(joined)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "ring-relation-joined")
	assertMembers(map[string]int64{})

	// Check that preparing the following hook fails as before...
	_, err = r.PrepareHook(changed)
	c.Assert(err, gc.ErrorMatches, `inappropriate "relation-changed" for "u/1": unit has not joined`)
	assertMembers(map[string]int64{})

	// ...but that committing the previous hook updates the persistent
	// relation state...
	err = r.CommitHook(joined)
	c.Assert(err, jc.ErrorIsNil)
	assertMembers(map[string]int64{"u/1": 0})

	// ...and allows us to prepare the next hook...
	name, err = r.PrepareHook(changed)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "ring-relation-changed")
	assertMembers(map[string]int64{"u/1": 0})

	// ...and commit it.
	err = r.CommitHook(changed)
	c.Assert(err, jc.ErrorIsNil)
	assertMembers(map[string]int64{"u/1": 7})

	// To verify implied behaviour above, prepare a new joined hook with
	// missing membership information, and check relation context
	// membership is stil not updated...
	joined.RemoteUnit = "u/2"
	joined.ChangeVersion = 3
	name, err = r.PrepareHook(joined)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "ring-relation-joined")
	assertMembers(map[string]int64{"u/1": 7})

	// ...until commit, at which point so is relation state.
	err = r.CommitHook(joined)
	c.Assert(err, jc.ErrorIsNil)
	assertMembers(map[string]int64{"u/1": 7, "u/2": 3})
}

func (s *relationerSuite) TestSetDying(c *gc.C) {
	ru1, u := s.AddRelationUnit(c, "u/1")
	settings := map[string]interface{}{"unit": "settings"}
	err := ru1.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	r := relation.NewRelationer(s.relUnit, s.mgr)
	err = r.Join()
	c.Assert(err, jc.ErrorIsNil)

	// Change Life to Dying check the results.
	err = r.SetDying()
	c.Assert(err, jc.ErrorIsNil)

	// Check that we cannot rejoin the relation.
	err = r.Join()
	c.Assert(err, gc.ErrorMatches, "dying relationer must not join!")

	// Simulate a RelationBroken hook.
	err = r.CommitHook(hook.Info{Kind: hooks.RelationBroken})
	c.Assert(err, jc.ErrorIsNil)

	// Check that the relation state has been removed by die.
	c.Assert(s.mgr.RelationFound(s.relUnit.Relation().Id()), jc.IsFalse)

	// Check that it left scope, by leaving scope on the other side and destroying
	// the relation.
	err = ru1.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

type stopper interface {
	Stop() error
}

func stop(c *gc.C, s stopper) {
	c.Assert(s.Stop(), gc.IsNil)
}

type relationerImplicitSuite struct {
	jujutesting.JujuConnSuite

	mockUnitRW *mocks.MockUnitStateReadWriter
}

var _ = gc.Suite(&relationerImplicitSuite{})

func (s *relationerImplicitSuite) TestImplicitRelationer(c *gc.C) {
	// Create a relationer for an implicit endpoint (mysql:juju-info).
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	u, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProviderAddresses(network.NewScopedSpaceAddress("blah", network.ScopeCloudLocal))
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	defer s.setupRWMock(c).Finish()
	mgr, err := relation.NewStateManager(s.mockUnitRW)
	c.Assert(err, jc.ErrorIsNil)

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = u.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	st := s.OpenAPIAs(c, u.Tag(), password)
	uniterState, err := st.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uniterState, gc.NotNil)

	apiUnit, err := uniterState.Unit(u.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	apiRel, err := uniterState.Relation(rel.Tag().(names.RelationTag))
	c.Assert(err, jc.ErrorIsNil)
	apiRelUnit, err := apiRel.Unit(apiUnit.Tag())
	c.Assert(err, jc.ErrorIsNil)
	relUnit := &relation.RelationUnitShim{apiRelUnit}

	r := relation.NewRelationer(relUnit, mgr)
	c.Assert(r, jc.Satisfies, (*relation.Relationer).IsImplicit)

	// Hooks are not allowed.
	_, err = r.PrepareHook(hook.Info{})
	c.Assert(err, gc.ErrorMatches, `restart immediately`)
	err = r.CommitHook(hook.Info{})
	c.Assert(err, gc.ErrorMatches, `restart immediately`)

	// Set it to Dying; check that the ops is removed immediately.
	err = r.SetDying()
	c.Assert(err, jc.ErrorIsNil)

	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *relationerImplicitSuite) setupRWMock(c *gc.C) *gomock.Controller {
	ctlr := gomock.NewController(c)
	s.mockUnitRW = mocks.NewMockUnitStateReadWriter(ctlr)
	exp := s.mockUnitRW.EXPECT()
	exp.State().Return(params.UnitStateResult{RelationState: map[int]string{0: ""}}, nil)
	exp.SetState(unitStateMatcher{c: c, expected: map[int]string{}}).Return(nil)
	return ctlr
}
