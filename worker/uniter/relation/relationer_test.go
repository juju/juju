// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	ft "github.com/juju/testing/filetesting"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable/hooks"

	"github.com/juju/juju/api"
	apiuniter "github.com/juju/juju/api/uniter"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/relation"
)

type RelationerSuite struct {
	jujutesting.JujuConnSuite
	hooks   chan hook.Info
	svc     *state.Service
	rel     *state.Relation
	dir     *relation.StateDir
	dirPath string

	st         api.Connection
	uniter     *apiuniter.State
	apiRelUnit *apiuniter.RelationUnit
}

var _ = gc.Suite(&RelationerSuite{})

func (s *RelationerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var err error
	s.svc = s.AddTestingService(c, "u", s.AddTestingCharm(c, "riak"))
	c.Assert(err, jc.ErrorIsNil)
	rels, err := s.svc.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rels, gc.HasLen, 1)
	s.rel = rels[0]
	_, unit := s.AddRelationUnit(c, "u/0")
	s.dirPath = c.MkDir()
	s.dir, err = relation.ReadStateDir(s.dirPath, s.rel.Id())
	c.Assert(err, jc.ErrorIsNil)
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
	s.apiRelUnit, err = apiRel.Unit(apiUnit)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RelationerSuite) AddRelationUnit(c *gc.C, name string) (*state.RelationUnit, *state.Unit) {
	u, err := s.svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.Name(), gc.Equals, name)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	privateAddr := network.NewScopedAddress(
		strings.Replace(name, "/", "-", 1)+".testing.invalid", network.ScopeCloudLocal,
	)
	err = machine.SetProviderAddresses(privateAddr)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := s.rel.Unit(u)
	c.Assert(err, jc.ErrorIsNil)
	return ru, u
}

func (s *RelationerSuite) TestStateDir(c *gc.C) {
	// Create the relationer; check its state dir is not created.
	r := relation.NewRelationer(s.apiRelUnit, s.dir)
	path := strconv.Itoa(s.rel.Id())
	ft.Removed{path}.Check(c, s.dirPath)

	// Join the relation; check the dir was created.
	err := r.Join()
	c.Assert(err, jc.ErrorIsNil)
	ft.Dir{path, 0755}.Check(c, s.dirPath)

	// Prepare to depart the relation; check the dir is still there.
	hi := hook.Info{Kind: hooks.RelationBroken}
	_, err = r.PrepareHook(hi)
	c.Assert(err, jc.ErrorIsNil)
	ft.Dir{path, 0755}.Check(c, s.dirPath)

	// Actually depart it; check the dir is removed.
	err = r.CommitHook(hi)
	c.Assert(err, jc.ErrorIsNil)
	ft.Removed{path}.Check(c, s.dirPath)
}

func (s *RelationerSuite) TestEnterLeaveScope(c *gc.C) {
	ru1, _ := s.AddRelationUnit(c, "u/1")
	r := relation.NewRelationer(s.apiRelUnit, s.dir)

	// u/1 does not consider u/0 to be alive.
	w := ru1.Watch()
	defer stop(c, w)
	s.State.StartSync()
	ch, ok := <-w.Changes()
	c.Assert(ok, jc.IsTrue)
	c.Assert(ch.Changed, gc.HasLen, 0)
	c.Assert(ch.Departed, gc.HasLen, 0)

	// u/0 enters scope; u/1 observes it.
	err := r.Join()
	c.Assert(err, jc.ErrorIsNil)
	s.State.StartSync()
	select {
	case ch, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(ch.Changed, gc.HasLen, 1)
		_, found := ch.Changed["u/0"]
		c.Assert(found, jc.IsTrue)
		c.Assert(ch.Departed, gc.HasLen, 0)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for presence detection")
	}

	// re-Join is no-op.
	err = r.Join()
	c.Assert(err, jc.ErrorIsNil)
	// TODO(jam): This would be a great to replace with statetesting.NotifyWatcherC
	s.State.StartSync()
	select {
	case ch, ok := <-w.Changes():
		c.Fatalf("got unexpected change: %#v, %#v", ch, ok)
	case <-time.After(coretesting.ShortWait):
	}

	// u/0 leaves scope; u/1 observes it.
	hi := hook.Info{Kind: hooks.RelationBroken}
	_, err = r.PrepareHook(hi)
	c.Assert(err, jc.ErrorIsNil)

	err = r.CommitHook(hi)
	c.Assert(err, jc.ErrorIsNil)
	s.State.StartSync()
	select {
	case ch, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(ch.Changed, gc.HasLen, 0)
		c.Assert(ch.Departed, gc.DeepEquals, []string{"u/0"})
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for absence detection")
	}
}

func (s *RelationerSuite) TestPrepareCommitHooks(c *gc.C) {
	r := relation.NewRelationer(s.apiRelUnit, s.dir)
	err := r.Join()
	c.Assert(err, jc.ErrorIsNil)

	assertMembers := func(expect map[string]int64) {
		c.Assert(s.dir.State().Members, jc.DeepEquals, expect)
		expectNames := make([]string, 0, len(expect))
		for name := range expect {
			expectNames = append(expectNames, name)
		}
		c.Assert(r.ContextInfo().MemberNames, jc.SameContents, expectNames)
	}
	assertMembers(map[string]int64{})

	// Check preparing an invalid hook changes nothing.
	changed := hook.Info{
		Kind:          hooks.RelationChanged,
		RemoteUnit:    "u/1",
		ChangeVersion: 7,
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

func (s *RelationerSuite) TestSetDying(c *gc.C) {
	ru1, u := s.AddRelationUnit(c, "u/1")
	settings := map[string]interface{}{"unit": "settings"}
	err := ru1.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	r := relation.NewRelationer(s.apiRelUnit, s.dir)
	err = r.Join()
	c.Assert(err, jc.ErrorIsNil)

	// Change Life to Dying check the results.
	err = r.SetDying()
	c.Assert(err, jc.ErrorIsNil)

	// Check that we cannot rejoin the relation.
	f := func() { r.Join() }
	c.Assert(f, gc.PanicMatches, "dying relationer must not join!")

	// Simulate a RelationBroken hook.
	err = r.CommitHook(hook.Info{Kind: hooks.RelationBroken})
	c.Assert(err, jc.ErrorIsNil)

	// Check that the relation state has been broken.
	err = s.dir.State().Validate(hook.Info{Kind: hooks.RelationBroken})
	c.Assert(err, gc.ErrorMatches, ".*: relation is broken and cannot be changed further")

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

type RelationerImplicitSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&RelationerImplicitSuite{})

func (s *RelationerImplicitSuite) TestImplicitRelationer(c *gc.C) {
	// Create a relationer for an implicit endpoint (mysql:juju-info).
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	u, err := mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProviderAddresses(network.NewScopedAddress("blah", network.ScopeCloudLocal))
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	relsDir := c.MkDir()
	dir, err := relation.ReadStateDir(relsDir, rel.Id())
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
	apiRelUnit, err := apiRel.Unit(apiUnit)
	c.Assert(err, jc.ErrorIsNil)

	r := relation.NewRelationer(apiRelUnit, dir)
	c.Assert(r, jc.Satisfies, (*relation.Relationer).IsImplicit)

	// Hooks are not allowed.
	f := func() { r.PrepareHook(hook.Info{}) }
	c.Assert(f, gc.PanicMatches, "implicit relations must not run hooks")
	f = func() { r.CommitHook(hook.Info{}) }
	c.Assert(f, gc.PanicMatches, "implicit relations must not run hooks")

	// Set it to Dying; check that the dir is removed immediately.
	err = r.SetDying()
	c.Assert(err, jc.ErrorIsNil)
	path := strconv.Itoa(rel.Id())
	ft.Removed{path}.Check(c, relsDir)

	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
