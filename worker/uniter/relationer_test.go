// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm/hooks"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/worker/uniter"
	"launchpad.net/juju-core/worker/uniter/hook"
	"launchpad.net/juju-core/worker/uniter/relation"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type RelationerSuite struct {
	testing.JujuConnSuite
	hooks   chan hook.Info
	svc     *state.Service
	rel     *state.Relation
	ru      *state.RelationUnit
	dir     *relation.StateDir
	dirPath string
}

var _ = Suite(&RelationerSuite{})

func (s *RelationerSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	var err error
	s.svc, err = s.State.AddService("u", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)
	rels, err := s.svc.Relations()
	c.Assert(err, IsNil)
	c.Assert(rels, HasLen, 1)
	s.rel = rels[0]
	s.ru = s.AddRelationUnit(c, "u/0")
	s.dirPath = c.MkDir()
	s.dir, err = relation.ReadStateDir(s.dirPath, s.rel.Id())
	c.Assert(err, IsNil)
	s.hooks = make(chan hook.Info)
}

func (s *RelationerSuite) AddRelationUnit(c *C, name string) *state.RelationUnit {
	u, err := s.svc.AddUnit()
	c.Assert(err, IsNil)
	c.Assert(u.Name(), Equals, name)
	err = u.SetPrivateAddress(strings.Replace(name, "/", "-", 1) + ".example.com")
	c.Assert(err, IsNil)
	ru, err := s.rel.Unit(u)
	c.Assert(err, IsNil)
	return ru
}

func (s *RelationerSuite) TestEnterLeaveScope(c *C) {
	ru1 := s.AddRelationUnit(c, "u/1")
	r := uniter.NewRelationer(s.ru, s.dir, s.hooks)

	// u/1 does not consider u/0 to be alive.
	w := ru1.Watch()
	defer stop(c, w)
	s.State.StartSync()
	ch, ok := <-w.Changes()
	c.Assert(ok, Equals, true)
	c.Assert(ch.Joined, HasLen, 0)
	c.Assert(ch.Changed, HasLen, 0)
	c.Assert(ch.Departed, HasLen, 0)

	// u/0 enters scope; u/1 observes it.
	err := r.Join()
	c.Assert(err, IsNil)
	s.State.StartSync()
	select {
	case ch, ok := <-w.Changes():
		c.Assert(ok, Equals, true)
		c.Assert(ch.Joined, DeepEquals, []string{"u/0"})
		c.Assert(ch.Changed, HasLen, 1)
		_, found := ch.Changed["u/0"]
		c.Assert(found, Equals, true)
		c.Assert(ch.Departed, HasLen, 0)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("timed out waiting for presence detection")
	}

	// re-Join is no-op.
	err = r.Join()
	c.Assert(err, IsNil)
	s.State.StartSync()
	select {
	case ch, ok := <-w.Changes():
		c.Fatalf("got unexpected change: %#v, %#v", ch, ok)
	case <-time.After(50 * time.Millisecond):
	}

	// u/0 leaves scope; u/1 observes it.
	hi := hook.Info{Kind: hooks.RelationBroken}
	_, err = r.PrepareHook(hi)
	c.Assert(err, IsNil)

	// Verify PrepareHook created the dir.
	fi, err := os.Stat(filepath.Join(s.dirPath, strconv.Itoa(s.rel.Id())))
	c.Assert(err, IsNil)
	c.Assert(fi, checkers.Satisfies, os.FileInfo.IsDir)

	err = r.CommitHook(hi)
	c.Assert(err, IsNil)
	s.State.StartSync()
	select {
	case ch, ok := <-w.Changes():
		c.Assert(ok, Equals, true)
		c.Assert(ch.Joined, HasLen, 0)
		c.Assert(ch.Changed, HasLen, 0)
		c.Assert(ch.Departed, DeepEquals, []string{"u/0"})
	case <-time.After(worstCase):
		c.Fatalf("timed out waiting for absence detection")
	}
}

func (s *RelationerSuite) TestStartStopHooks(c *C) {
	ru1 := s.AddRelationUnit(c, "u/1")
	ru2 := s.AddRelationUnit(c, "u/2")
	r := uniter.NewRelationer(s.ru, s.dir, s.hooks)
	c.Assert(r.IsImplicit(), Equals, false)
	err := r.Join()
	c.Assert(err, IsNil)

	// Check no hooks are being sent.
	s.assertNoHook(c)

	// Start hooks, and check that still no changes are sent.
	r.StartHooks()
	defer stopHooks(c, r)
	s.assertNoHook(c)

	// Check we can't start hooks again.
	f := func() { r.StartHooks() }
	c.Assert(f, PanicMatches, "hooks already started!")

	// Join u/1 to the relation, and check that we receive the expected hooks.
	settings := map[string]interface{}{"unit": "settings"}
	err = ru1.EnterScope(settings)
	c.Assert(err, IsNil)
	s.assertHook(c, hook.Info{
		Kind:       hooks.RelationJoined,
		RemoteUnit: "u/1",
		Members:    map[string]map[string]interface{}{"u/1": settings},
	})
	s.assertHook(c, hook.Info{
		Kind:       hooks.RelationChanged,
		RemoteUnit: "u/1",
		Members:    map[string]map[string]interface{}{"u/1": settings},
	})
	s.assertNoHook(c)

	// Stop hooks, make more changes, check no events.
	err = r.StopHooks()
	c.Assert(err, IsNil)
	err = ru1.LeaveScope()
	c.Assert(err, IsNil)
	err = ru2.EnterScope(nil)
	c.Assert(err, IsNil)
	node, err := ru2.Settings()
	c.Assert(err, IsNil)
	node.Set("private-address", "roehampton")
	_, err = node.Write()
	c.Assert(err, IsNil)
	s.assertNoHook(c)

	// Stop hooks again to verify safety.
	err = r.StopHooks()
	c.Assert(err, IsNil)
	s.assertNoHook(c)

	// Start them again, and check we get the expected events sent.
	r.StartHooks()
	defer stopHooks(c, r)
	s.assertHook(c, hook.Info{
		Kind:       hooks.RelationDeparted,
		RemoteUnit: "u/1",
		Members:    map[string]map[string]interface{}{},
	})
	s.assertHook(c, hook.Info{
		Kind:          hooks.RelationJoined,
		ChangeVersion: 1,
		RemoteUnit:    "u/2",
		Members: map[string]map[string]interface{}{
			"u/2": {"private-address": "roehampton"},
		},
	})
	s.assertHook(c, hook.Info{
		Kind:          hooks.RelationChanged,
		ChangeVersion: 1,
		RemoteUnit:    "u/2",
		Members: map[string]map[string]interface{}{
			"u/2": {"private-address": "roehampton"},
		},
	})
	s.assertNoHook(c)

	// Stop them again, just to be sure.
	err = r.StopHooks()
	c.Assert(err, IsNil)
	s.assertNoHook(c)
}

func (s *RelationerSuite) TestPrepareCommitHooks(c *C) {
	r := uniter.NewRelationer(s.ru, s.dir, s.hooks)
	err := r.Join()
	c.Assert(err, IsNil)
	ctx := r.Context()
	c.Assert(ctx.UnitNames(), HasLen, 0)

	// Check preparing an invalid hook changes nothing.
	changed := hook.Info{
		Kind:          hooks.RelationChanged,
		RemoteUnit:    "u/1",
		ChangeVersion: 7,
		Members: map[string]map[string]interface{}{
			"u/1": {"private-address": "glastonbury"},
		},
	}
	_, err = r.PrepareHook(changed)
	c.Assert(err, ErrorMatches, `inappropriate "relation-changed" for "u/1": unit has not joined`)
	c.Assert(ctx.UnitNames(), HasLen, 0)
	c.Assert(s.dir.State().Members, HasLen, 0)

	// Check preparing a valid hook updates the context, but not persistent
	// relation state.
	joined := hook.Info{
		Kind:       hooks.RelationJoined,
		RemoteUnit: "u/1",
		Members: map[string]map[string]interface{}{
			"u/1": {"private-address": "u-1.example.com"},
		},
	}
	name, err := r.PrepareHook(joined)
	c.Assert(err, IsNil)
	c.Assert(s.dir.State().Members, HasLen, 0)
	c.Assert(name, Equals, "ring-relation-joined")
	c.Assert(ctx.UnitNames(), DeepEquals, []string{"u/1"})
	s1, err := ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(s1, DeepEquals, joined.Members["u/1"])

	// Clear the changed hook's Members, as though it had been deserialized.
	changed.Members = nil

	// Check that preparing the following hook fails as before...
	_, err = r.PrepareHook(changed)
	c.Assert(err, ErrorMatches, `inappropriate "relation-changed" for "u/1": unit has not joined`)
	c.Assert(s.dir.State().Members, HasLen, 0)
	c.Assert(ctx.UnitNames(), DeepEquals, []string{"u/1"})
	s1, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(s1, DeepEquals, joined.Members["u/1"])

	// ...but that committing the previous hook updates the persistent
	// relation state...
	err = r.CommitHook(joined)
	c.Assert(err, IsNil)
	c.Assert(s.dir.State().Members, DeepEquals, map[string]int64{"u/1": 0})
	c.Assert(ctx.UnitNames(), DeepEquals, []string{"u/1"})
	s1, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(s1, DeepEquals, joined.Members["u/1"])

	// ...and allows us to prepare the next hook...
	name, err = r.PrepareHook(changed)
	c.Assert(err, IsNil)
	c.Assert(name, Equals, "ring-relation-changed")
	c.Assert(s.dir.State().Members, DeepEquals, map[string]int64{"u/1": 0})
	c.Assert(ctx.UnitNames(), DeepEquals, []string{"u/1"})
	s1, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(s1, DeepEquals, map[string]interface{}{"private-address": "u-1.example.com"})

	// ...and commit it.
	err = r.CommitHook(changed)
	c.Assert(err, IsNil)
	c.Assert(s.dir.State().Members, DeepEquals, map[string]int64{"u/1": 7})
	c.Assert(ctx.UnitNames(), DeepEquals, []string{"u/1"})

	// To verify implied behaviour above, prepare a new joined hook with
	// missing membership information, and check relation context
	// membership is updated appropriately...
	joined.RemoteUnit = "u/2"
	joined.ChangeVersion = 3
	joined.Members = nil
	name, err = r.PrepareHook(joined)
	c.Assert(err, IsNil)
	c.Assert(s.dir.State().Members, HasLen, 1)
	c.Assert(name, Equals, "ring-relation-joined")
	c.Assert(ctx.UnitNames(), DeepEquals, []string{"u/1", "u/2"})

	// ...and so is relation state on commit.
	err = r.CommitHook(joined)
	c.Assert(err, IsNil)
	c.Assert(s.dir.State().Members, DeepEquals, map[string]int64{"u/1": 7, "u/2": 3})
	c.Assert(ctx.UnitNames(), DeepEquals, []string{"u/1", "u/2"})
}

func (s *RelationerSuite) TestSetDying(c *C) {
	ru1 := s.AddRelationUnit(c, "u/1")
	settings := map[string]interface{}{"unit": "settings"}
	err := ru1.EnterScope(settings)
	c.Assert(err, IsNil)
	r := uniter.NewRelationer(s.ru, s.dir, s.hooks)
	err = r.Join()
	c.Assert(err, IsNil)
	r.StartHooks()
	defer stopHooks(c, r)
	s.assertHook(c, hook.Info{
		Kind:       hooks.RelationJoined,
		RemoteUnit: "u/1",
		Members: map[string]map[string]interface{}{
			"u/1": settings,
		},
	})

	// While a changed hook is still pending, the relation (or possibly the unit,
	// pending lifecycle work), changes Life to Dying, and the relationer is
	// informed.
	err = r.SetDying()
	c.Assert(err, IsNil)

	// Check that we cannot rejoin the relation.
	f := func() { r.Join() }
	c.Assert(f, PanicMatches, "dying relationer must not join!")

	// ...but the hook stream continues, sending the required changed hook for
	// u/1 before moving on to a departed, despite the fact that its pinger is
	// still running, and closing with a broken.
	s.assertHook(c, hook.Info{Kind: hooks.RelationChanged, RemoteUnit: "u/1"})
	s.assertHook(c, hook.Info{Kind: hooks.RelationDeparted, RemoteUnit: "u/1"})
	s.assertHook(c, hook.Info{Kind: hooks.RelationBroken})

	// Check that the relation state has been broken.
	err = s.dir.State().Validate(hook.Info{Kind: hooks.RelationBroken})
	c.Assert(err, ErrorMatches, ".*: relation is broken and cannot be changed further")
}

func (s *RelationerSuite) assertNoHook(c *C) {
	s.State.StartSync()
	select {
	case hi, ok := <-s.hooks:
		c.Fatalf("got unexpected hook info %#v (%t)", hi, ok)
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *RelationerSuite) assertHook(c *C, expect hook.Info) {
	s.State.StartSync()
	// We must ensure the local state dir exists first.
	c.Assert(s.dir.Ensure(), IsNil)
	select {
	case hi, ok := <-s.hooks:
		c.Assert(ok, Equals, true)
		expect.ChangeVersion = hi.ChangeVersion
		c.Assert(hi, DeepEquals, expect)
		c.Assert(s.dir.Write(hi), Equals, nil)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("timed out waiting for %#v", expect)
	}
}

type stopper interface {
	Stop() error
}

func stop(c *C, s stopper) {
	c.Assert(s.Stop(), IsNil)
}

func stopHooks(c *C, r *uniter.Relationer) {
	c.Assert(r.StopHooks(), IsNil)
}

type RelationerImplicitSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&RelationerImplicitSuite{})

func (s *RelationerImplicitSuite) TestImplicitRelationer(c *C) {
	// Create a relationer for an implicit endpoint (mysql:juju-info).
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	u, err := mysql.AddUnit()
	c.Assert(err, IsNil)
	err = u.SetPrivateAddress("blah")
	c.Assert(err, IsNil)
	logging, err := s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"logging", "mysql"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	ru, err := rel.Unit(u)
	c.Assert(err, IsNil)
	relsDir := c.MkDir()
	dir, err := relation.ReadStateDir(relsDir, rel.Id())
	c.Assert(err, IsNil)
	hooks := make(chan hook.Info)
	r := uniter.NewRelationer(ru, dir, hooks)
	c.Assert(r, checkers.Satisfies, (*uniter.Relationer).IsImplicit)

	// Join the relationer; the dir won't be created until necessary
	err = r.Join()
	c.Assert(err, IsNil)
	_, err = os.Stat(filepath.Join(relsDir, strconv.Itoa(rel.Id())))
	c.Assert(err, NotNil)
	sub, err := logging.Unit("logging/0")
	c.Assert(err, IsNil)
	err = sub.SetPrivateAddress("blah")
	c.Assert(err, IsNil)

	// Join the other side; check no hooks are sent.
	r.StartHooks()
	defer func() { c.Assert(r.StopHooks(), IsNil) }()
	subru, err := rel.Unit(sub)
	c.Assert(err, IsNil)
	err = subru.EnterScope(map[string]interface{}{"some": "data"})
	c.Assert(err, IsNil)
	s.State.StartSync()
	select {
	case <-time.After(50 * time.Millisecond):
	case <-hooks:
		c.Fatalf("unexpected hook generated")
	}

	// Set it to Dying; check that the dir is removed.
	err = r.SetDying()
	c.Assert(err, IsNil)
	_, err = os.Stat(filepath.Join(relsDir, strconv.Itoa(rel.Id())))
	c.Assert(err, checkers.Satisfies, os.IsNotExist)

	// Check that it left scope, by leaving scope on the other side and destroying
	// the relation.
	err = subru.LeaveScope()
	c.Assert(err, IsNil)
	err = rel.Destroy()
	c.Assert(err, IsNil)
	err = rel.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)

	// Verify that no other hooks were sent at any stage.
	select {
	case <-hooks:
		c.Fatalf("unexpected hook generated")
	default:
	}
}
