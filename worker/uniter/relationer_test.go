// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm/hooks"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	apiuniter "launchpad.net/juju-core/state/api/uniter"
	coretesting "launchpad.net/juju-core/testing"
	ft "launchpad.net/juju-core/testing/filetesting"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/worker/uniter"
	"launchpad.net/juju-core/worker/uniter/hook"
	"launchpad.net/juju-core/worker/uniter/relation"
)

type RelationerSuite struct {
	jujutesting.JujuConnSuite
	hooks   chan hook.Info
	svc     *state.Service
	rel     *state.Relation
	dir     *relation.StateDir
	dirPath string

	st         *api.State
	uniter     *apiuniter.State
	apiRelUnit *apiuniter.RelationUnit
}

var _ = gc.Suite(&RelationerSuite{})

func (s *RelationerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var err error
	s.svc = s.AddTestingService(c, "u", s.AddTestingCharm(c, "riak"))
	c.Assert(err, gc.IsNil)
	rels, err := s.svc.Relations()
	c.Assert(err, gc.IsNil)
	c.Assert(rels, gc.HasLen, 1)
	s.rel = rels[0]
	_, unit := s.AddRelationUnit(c, "u/0")
	s.dirPath = c.MkDir()
	s.dir, err = relation.ReadStateDir(s.dirPath, s.rel.Id())
	c.Assert(err, gc.IsNil)
	s.hooks = make(chan hook.Info)

	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = unit.SetPassword(password)
	c.Assert(err, gc.IsNil)
	s.st = s.OpenAPIAs(c, unit.Tag(), password)
	s.uniter = s.st.Uniter()
	c.Assert(s.uniter, gc.NotNil)

	apiUnit, err := s.uniter.Unit(unit.Tag())
	c.Assert(err, gc.IsNil)
	apiRel, err := s.uniter.Relation(s.rel.Tag())
	c.Assert(err, gc.IsNil)
	s.apiRelUnit, err = apiRel.Unit(apiUnit)
	c.Assert(err, gc.IsNil)
}

func (s *RelationerSuite) AddRelationUnit(c *gc.C, name string) (*state.RelationUnit, *state.Unit) {
	u, err := s.svc.AddUnit()
	c.Assert(err, gc.IsNil)
	c.Assert(u.Name(), gc.Equals, name)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = u.AssignToMachine(machine)
	c.Assert(err, gc.IsNil)
	privateAddr := instance.NewAddress(
		strings.Replace(name, "/", "-", 1)+".testing.invalid", instance.NetworkCloudLocal)
	err = machine.SetAddresses(privateAddr)
	c.Assert(err, gc.IsNil)
	ru, err := s.rel.Unit(u)
	c.Assert(err, gc.IsNil)
	return ru, u
}

func (s *RelationerSuite) TestStateDir(c *gc.C) {
	// Create the relationer; check its state dir is not created.
	r := uniter.NewRelationer(s.apiRelUnit, s.dir, s.hooks)
	path := strconv.Itoa(s.rel.Id())
	ft.Removed{path}.Check(c, s.dirPath)

	// Join the relation; check the dir was created.
	err := r.Join()
	c.Assert(err, gc.IsNil)
	ft.Dir{path, 0755}.Check(c, s.dirPath)

	// Prepare to depart the relation; check the dir is still there.
	hi := hook.Info{Kind: hooks.RelationBroken}
	_, err = r.PrepareHook(hi)
	c.Assert(err, gc.IsNil)
	ft.Dir{path, 0755}.Check(c, s.dirPath)

	// Actually depart it; check the dir is removed.
	err = r.CommitHook(hi)
	c.Assert(err, gc.IsNil)
	ft.Removed{path}.Check(c, s.dirPath)
}

func (s *RelationerSuite) TestEnterLeaveScope(c *gc.C) {
	ru1, _ := s.AddRelationUnit(c, "u/1")
	r := uniter.NewRelationer(s.apiRelUnit, s.dir, s.hooks)

	// u/1 does not consider u/0 to be alive.
	w := ru1.Watch()
	defer stop(c, w)
	s.State.StartSync()
	ch, ok := <-w.Changes()
	c.Assert(ok, gc.Equals, true)
	c.Assert(ch.Changed, gc.HasLen, 0)
	c.Assert(ch.Departed, gc.HasLen, 0)

	// u/0 enters scope; u/1 observes it.
	err := r.Join()
	c.Assert(err, gc.IsNil)
	s.State.StartSync()
	select {
	case ch, ok := <-w.Changes():
		c.Assert(ok, gc.Equals, true)
		c.Assert(ch.Changed, gc.HasLen, 1)
		_, found := ch.Changed["u/0"]
		c.Assert(found, gc.Equals, true)
		c.Assert(ch.Departed, gc.HasLen, 0)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for presence detection")
	}

	// re-Join is no-op.
	err = r.Join()
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)

	err = r.CommitHook(hi)
	c.Assert(err, gc.IsNil)
	s.State.StartSync()
	select {
	case ch, ok := <-w.Changes():
		c.Assert(ok, gc.Equals, true)
		c.Assert(ch.Changed, gc.HasLen, 0)
		c.Assert(ch.Departed, gc.DeepEquals, []string{"u/0"})
	case <-time.After(worstCase):
		c.Fatalf("timed out waiting for absence detection")
	}
}

func (s *RelationerSuite) TestStartStopHooks(c *gc.C) {
	ru1, _ := s.AddRelationUnit(c, "u/1")
	ru2, _ := s.AddRelationUnit(c, "u/2")
	r := uniter.NewRelationer(s.apiRelUnit, s.dir, s.hooks)
	c.Assert(r.IsImplicit(), gc.Equals, false)
	err := r.Join()
	c.Assert(err, gc.IsNil)

	// Check no hooks are being sent.
	s.assertNoHook(c)

	// Start hooks, and check that still no changes are sent.
	r.StartHooks()
	defer stopHooks(c, r)
	s.assertNoHook(c)

	// Check we can't start hooks again.
	f := func() { r.StartHooks() }
	c.Assert(f, gc.PanicMatches, "hooks already started!")

	// Join u/1 to the relation, and check that we receive the expected hooks.
	settings := map[string]interface{}{"unit": "settings"}
	err = ru1.EnterScope(settings)
	c.Assert(err, gc.IsNil)
	s.assertHook(c, hook.Info{
		Kind:       hooks.RelationJoined,
		RemoteUnit: "u/1",
	})
	s.assertHook(c, hook.Info{
		Kind:       hooks.RelationChanged,
		RemoteUnit: "u/1",
	})
	s.assertNoHook(c)

	// Stop hooks, make more changes, check no events.
	err = r.StopHooks()
	c.Assert(err, gc.IsNil)
	err = ru1.LeaveScope()
	c.Assert(err, gc.IsNil)
	err = ru2.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	node, err := ru2.Settings()
	c.Assert(err, gc.IsNil)
	node.Set("private-address", "roehampton")
	_, err = node.Write()
	c.Assert(err, gc.IsNil)
	s.assertNoHook(c)

	// Stop hooks again to verify safety.
	err = r.StopHooks()
	c.Assert(err, gc.IsNil)
	s.assertNoHook(c)

	// Start them again, and check we get the expected events sent.
	r.StartHooks()
	defer stopHooks(c, r)
	s.assertHook(c, hook.Info{
		Kind:       hooks.RelationDeparted,
		RemoteUnit: "u/1",
	})
	s.assertHook(c, hook.Info{
		Kind:          hooks.RelationJoined,
		ChangeVersion: 1,
		RemoteUnit:    "u/2",
	})
	s.assertHook(c, hook.Info{
		Kind:          hooks.RelationChanged,
		ChangeVersion: 1,
		RemoteUnit:    "u/2",
	})
	s.assertNoHook(c)

	// Stop them again, just to be sure.
	err = r.StopHooks()
	c.Assert(err, gc.IsNil)
	s.assertNoHook(c)
}

func (s *RelationerSuite) TestPrepareCommitHooks(c *gc.C) {
	r := uniter.NewRelationer(s.apiRelUnit, s.dir, s.hooks)
	err := r.Join()
	c.Assert(err, gc.IsNil)
	ctx := r.Context()
	c.Assert(ctx.UnitNames(), gc.HasLen, 0)

	// Check preparing an invalid hook changes nothing.
	changed := hook.Info{
		Kind:          hooks.RelationChanged,
		RemoteUnit:    "u/1",
		ChangeVersion: 7,
	}
	_, err = r.PrepareHook(changed)
	c.Assert(err, gc.ErrorMatches, `inappropriate "relation-changed" for "u/1": unit has not joined`)
	c.Assert(ctx.UnitNames(), gc.HasLen, 0)
	c.Assert(s.dir.State().Members, gc.HasLen, 0)

	// Check preparing a valid hook updates the context, but not persistent
	// relation state.
	joined := hook.Info{
		Kind:       hooks.RelationJoined,
		RemoteUnit: "u/1",
	}
	name, err := r.PrepareHook(joined)
	c.Assert(err, gc.IsNil)
	c.Assert(s.dir.State().Members, gc.HasLen, 0)
	c.Assert(name, gc.Equals, "ring-relation-joined")
	c.Assert(ctx.UnitNames(), gc.DeepEquals, []string{"u/1"})

	// Check that preparing the following hook fails as before...
	_, err = r.PrepareHook(changed)
	c.Assert(err, gc.ErrorMatches, `inappropriate "relation-changed" for "u/1": unit has not joined`)
	c.Assert(s.dir.State().Members, gc.HasLen, 0)
	c.Assert(ctx.UnitNames(), gc.DeepEquals, []string{"u/1"})

	// ...but that committing the previous hook updates the persistent
	// relation state...
	err = r.CommitHook(joined)
	c.Assert(err, gc.IsNil)
	c.Assert(s.dir.State().Members, gc.DeepEquals, map[string]int64{"u/1": 0})
	c.Assert(ctx.UnitNames(), gc.DeepEquals, []string{"u/1"})

	// ...and allows us to prepare the next hook...
	name, err = r.PrepareHook(changed)
	c.Assert(err, gc.IsNil)
	c.Assert(name, gc.Equals, "ring-relation-changed")
	c.Assert(s.dir.State().Members, gc.DeepEquals, map[string]int64{"u/1": 0})
	c.Assert(ctx.UnitNames(), gc.DeepEquals, []string{"u/1"})

	// ...and commit it.
	err = r.CommitHook(changed)
	c.Assert(err, gc.IsNil)
	c.Assert(s.dir.State().Members, gc.DeepEquals, map[string]int64{"u/1": 7})
	c.Assert(ctx.UnitNames(), gc.DeepEquals, []string{"u/1"})

	// To verify implied behaviour above, prepare a new joined hook with
	// missing membership information, and check relation context
	// membership is updated appropriately...
	joined.RemoteUnit = "u/2"
	joined.ChangeVersion = 3
	name, err = r.PrepareHook(joined)
	c.Assert(err, gc.IsNil)
	c.Assert(s.dir.State().Members, gc.HasLen, 1)
	c.Assert(name, gc.Equals, "ring-relation-joined")
	c.Assert(ctx.UnitNames(), gc.DeepEquals, []string{"u/1", "u/2"})

	// ...and so is relation state on commit.
	err = r.CommitHook(joined)
	c.Assert(err, gc.IsNil)
	c.Assert(s.dir.State().Members, gc.DeepEquals, map[string]int64{"u/1": 7, "u/2": 3})
	c.Assert(ctx.UnitNames(), gc.DeepEquals, []string{"u/1", "u/2"})
}

func (s *RelationerSuite) TestSetDying(c *gc.C) {
	ru1, _ := s.AddRelationUnit(c, "u/1")
	settings := map[string]interface{}{"unit": "settings"}
	err := ru1.EnterScope(settings)
	c.Assert(err, gc.IsNil)
	r := uniter.NewRelationer(s.apiRelUnit, s.dir, s.hooks)
	err = r.Join()
	c.Assert(err, gc.IsNil)
	r.StartHooks()
	defer stopHooks(c, r)
	s.assertHook(c, hook.Info{
		Kind:       hooks.RelationJoined,
		RemoteUnit: "u/1",
	})

	// While a changed hook is still pending, the relation (or possibly the unit,
	// pending lifecycle work), changes Life to Dying, and the relationer is
	// informed.
	err = r.SetDying()
	c.Assert(err, gc.IsNil)

	// Check that we cannot rejoin the relation.
	f := func() { r.Join() }
	c.Assert(f, gc.PanicMatches, "dying relationer must not join!")

	// ...but the hook stream continues, sending the required changed hook for
	// u/1 before moving on to a departed, despite the fact that its pinger is
	// still running, and closing with a broken.
	s.assertHook(c, hook.Info{Kind: hooks.RelationChanged, RemoteUnit: "u/1"})
	s.assertHook(c, hook.Info{Kind: hooks.RelationDeparted, RemoteUnit: "u/1"})
	s.assertHook(c, hook.Info{Kind: hooks.RelationBroken})

	// Check that the relation state has been broken.
	err = s.dir.State().Validate(hook.Info{Kind: hooks.RelationBroken})
	c.Assert(err, gc.ErrorMatches, ".*: relation is broken and cannot be changed further")
}

func (s *RelationerSuite) assertNoHook(c *gc.C) {
	s.BackingState.StartSync()
	select {
	case hi, ok := <-s.hooks:
		c.Fatalf("got unexpected hook info %#v (%t)", hi, ok)
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *RelationerSuite) assertHook(c *gc.C, expect hook.Info) {
	s.BackingState.StartSync()
	// We must ensure the local state dir exists first.
	c.Assert(s.dir.Ensure(), gc.IsNil)
	select {
	case hi, ok := <-s.hooks:
		c.Assert(ok, gc.Equals, true)
		expect.ChangeVersion = hi.ChangeVersion
		c.Assert(hi, gc.DeepEquals, expect)
		c.Assert(s.dir.Write(hi), gc.Equals, nil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for %#v", expect)
	}
}

type stopper interface {
	Stop() error
}

func stop(c *gc.C, s stopper) {
	c.Assert(s.Stop(), gc.IsNil)
}

func stopHooks(c *gc.C, r *uniter.Relationer) {
	c.Assert(r.StopHooks(), gc.IsNil)
}

type RelationerImplicitSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&RelationerImplicitSuite{})

func (s *RelationerImplicitSuite) TestImplicitRelationer(c *gc.C) {
	// Create a relationer for an implicit endpoint (mysql:juju-info).
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	u, err := mysql.AddUnit()
	c.Assert(err, gc.IsNil)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = u.AssignToMachine(machine)
	c.Assert(err, gc.IsNil)
	err = machine.SetAddresses(instance.NewAddress("blah", instance.NetworkCloudLocal))
	c.Assert(err, gc.IsNil)
	logging := s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints([]string{"logging", "mysql"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	relsDir := c.MkDir()
	dir, err := relation.ReadStateDir(relsDir, rel.Id())
	c.Assert(err, gc.IsNil)
	hooks := make(chan hook.Info)

	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = u.SetPassword(password)
	c.Assert(err, gc.IsNil)
	st := s.OpenAPIAs(c, u.Tag(), password)
	uniterState := st.Uniter()
	c.Assert(uniterState, gc.NotNil)

	apiUnit, err := uniterState.Unit(u.Tag())
	c.Assert(err, gc.IsNil)
	apiRel, err := uniterState.Relation(rel.Tag())
	c.Assert(err, gc.IsNil)
	apiRelUnit, err := apiRel.Unit(apiUnit)
	c.Assert(err, gc.IsNil)

	r := uniter.NewRelationer(apiRelUnit, dir, hooks)
	c.Assert(r, jc.Satisfies, (*uniter.Relationer).IsImplicit)

	// Join the relation.
	err = r.Join()
	c.Assert(err, gc.IsNil)
	sub, err := logging.Unit("logging/0")
	c.Assert(err, gc.IsNil)

	// Join the other side; check no hooks are sent.
	r.StartHooks()
	defer func() { c.Assert(r.StopHooks(), gc.IsNil) }()
	subru, err := rel.Unit(sub)
	c.Assert(err, gc.IsNil)
	err = subru.EnterScope(map[string]interface{}{"some": "data"})
	c.Assert(err, gc.IsNil)
	s.State.StartSync()
	select {
	case <-time.After(coretesting.ShortWait):
	case <-hooks:
		c.Fatalf("unexpected hook generated")
	}

	// Set it to Dying; check that the dir is removed immediately.
	err = r.SetDying()
	c.Assert(err, gc.IsNil)
	path := strconv.Itoa(rel.Id())
	ft.Removed{path}.Check(c, relsDir)

	// Check that it left scope, by leaving scope on the other side and destroying
	// the relation.
	err = subru.LeaveScope()
	c.Assert(err, gc.IsNil)
	err = rel.Destroy()
	c.Assert(err, gc.IsNil)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Verify that no other hooks were sent at any stage.
	select {
	case <-hooks:
		c.Fatalf("unexpected hook generated")
	default:
	}
}
