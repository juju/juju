package uniter_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/presence"
	"launchpad.net/juju-core/worker/uniter"
	"launchpad.net/juju-core/worker/uniter/hook"
	"launchpad.net/juju-core/worker/uniter/relation"
	"strings"
	"time"
)

type RelationerSuite struct {
	testing.JujuConnSuite
	hooks chan hook.Info
	svc   *state.Service
	rel   *state.Relation
	ru    *state.RelationUnit
	dir   *relation.StateDir
}

var _ = Suite(&RelationerSuite{})

func (s *RelationerSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	ch := s.AddTestingCharm(c, "dummy")
	var err error
	s.svc, err = s.State.AddService("u", ch)
	c.Assert(err, IsNil)
	s.rel, err = s.State.AddRelation(state.RelationEndpoint{
		"u", "ifce", "my-relation", state.RolePeer, charm.ScopeGlobal,
	})
	c.Assert(err, IsNil)
	s.ru = s.AddRelationUnit(c, "u/0")
	s.dir, err = relation.ReadStateDir(c.MkDir(), s.rel.Id())
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

func (s *RelationerSuite) TestStartStopPresence(c *C) {
	ru1 := s.AddRelationUnit(c, "u/1")
	r := uniter.NewRelationer(s.ru, s.dir, s.hooks)

	// Check that a watcher does not consider u/0 to be alive.
	w := ru1.Watch()
	defer stop(c, w)
	ch, ok := <-w.Changes()
	c.Assert(ok, Equals, true)
	c.Assert(ch.Changed, HasLen, 0)
	c.Assert(ch.Departed, HasLen, 0)

	// Start u/0's pinger and check it is observed by u/1.
	err := r.Join()
	c.Assert(err, IsNil)
	defer abandon(c, r)
	assertChanged(c, w)

	// Check that we can't start u/0's pinger again while it's running.
	f := func() { r.Join() }
	c.Assert(f, PanicMatches, "pinger is already started")

	// Stop the pinger and check the change is observed.
	err = r.Abandon()
	c.Assert(err, IsNil)
	assertDeparted(c, w)

	// Stop it again to check that multiple stops are safe.
	err = r.Abandon()
	c.Assert(err, IsNil)

	// Check we can start it again, and it works...
	err = r.Join()
	c.Assert(err, IsNil)
	assertChanged(c, w)

	// ..and stop it again, and that works too.
	err = r.Abandon()
	c.Assert(err, IsNil)
	assertDeparted(c, w)
}

func (s *RelationerSuite) TestStartStopHooks(c *C) {
	ru1 := s.AddRelationUnit(c, "u/1")
	ru2 := s.AddRelationUnit(c, "u/2")
	r := uniter.NewRelationer(s.ru, s.dir, s.hooks)
	err := r.Join()
	c.Assert(err, IsNil)
	defer r.Abandon()

	// Check no hooks are being sent.
	s.assertNoHook(c)

	// Start hooks, and check that still no changes are sent.
	r.StartHooks()
	s.assertNoHook(c)

	// Check we can't start hooks again.
	f := func() { r.StartHooks() }
	c.Assert(f, PanicMatches, "hooks already started!")

	// Join u/1 to the relation, and check that we receive the expected hooks.
	err = ru1.Init()
	c.Assert(err, IsNil)
	err = ru1.Pinger().Start()
	c.Assert(err, IsNil)
	defer kill(c, ru1.Pinger())
	s.assertHook(c, hook.Info{
		Kind:       hook.RelationJoined,
		RemoteUnit: "u/1",
		Members: map[string]map[string]interface{}{
			"u/1": {"private-address": "u-1.example.com"},
		},
	})
	s.assertHook(c, hook.Info{
		Kind:       hook.RelationChanged,
		RemoteUnit: "u/1",
		Members: map[string]map[string]interface{}{
			"u/1": {"private-address": "u-1.example.com"},
		},
	})
	s.assertNoHook(c)

	// Stop hooks, make more changes, check no events.
	err = r.StopHooks()
	c.Assert(err, IsNil)
	kill(c, ru1.Pinger())
	err = ru2.Init()
	c.Assert(err, IsNil)
	err = ru2.Pinger().Start()
	c.Assert(err, IsNil)
	defer kill(c, ru2.Pinger())
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
	s.assertHook(c, hook.Info{
		Kind:       hook.RelationDeparted,
		RemoteUnit: "u/1",
		Members:    map[string]map[string]interface{}{},
	})
	s.assertHook(c, hook.Info{
		Kind:          hook.RelationJoined,
		ChangeVersion: 1,
		RemoteUnit:    "u/2",
		Members: map[string]map[string]interface{}{
			"u/2": {"private-address": "roehampton"},
		},
	})
	s.assertHook(c, hook.Info{
		Kind:          hook.RelationChanged,
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
	defer r.Abandon()
	ctx := r.Context()
	c.Assert(ctx.Units(), HasLen, 0)

	// Check preparing an invalid hook changes nothing.
	changed := hook.Info{
		Kind:          hook.RelationChanged,
		RemoteUnit:    "u/1",
		ChangeVersion: 7,
		Members: map[string]map[string]interface{}{
			"u/1": {"private-address": "glastonbury"},
		},
	}
	_, err = r.PrepareHook(changed)
	c.Assert(err, ErrorMatches, `inappropriate "relation-changed" for "u/1": unit has not joined`)
	c.Assert(ctx.Units(), HasLen, 0)
	c.Assert(s.dir.State().Members, HasLen, 0)

	// Check preparing a valid hook updates the context, but not persistent
	// relation state.
	joined := hook.Info{
		Kind:       hook.RelationJoined,
		RemoteUnit: "u/1",
		Members: map[string]map[string]interface{}{
			"u/1": {"private-address": "u-1.example.com"},
		},
	}
	name, err := r.PrepareHook(joined)
	c.Assert(err, IsNil)
	c.Assert(s.dir.State().Members, HasLen, 0)
	c.Assert(name, Equals, "my-relation-relation-joined")
	c.Assert(ctx.Units(), DeepEquals, []string{"u/1"})
	s1, err := ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(s1, DeepEquals, joined.Members["u/1"])

	// Clear the changed hook's Members, as though it had been deserialized.
	changed.Members = nil

	// Check that preparing the following hook fails as before...
	_, err = r.PrepareHook(changed)
	c.Assert(err, ErrorMatches, `inappropriate "relation-changed" for "u/1": unit has not joined`)
	c.Assert(s.dir.State().Members, HasLen, 0)
	c.Assert(ctx.Units(), DeepEquals, []string{"u/1"})
	s1, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(s1, DeepEquals, joined.Members["u/1"])

	// ...but that committing the previous hook updates the persistent
	// relation state...
	err = r.CommitHook(joined)
	c.Assert(err, IsNil)
	c.Assert(s.dir.State().Members, DeepEquals, map[string]int{"u/1": 0})
	c.Assert(ctx.Units(), DeepEquals, []string{"u/1"})
	s1, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(s1, DeepEquals, joined.Members["u/1"])

	// ...and allows us to prepare the next hook...
	name, err = r.PrepareHook(changed)
	c.Assert(err, IsNil)
	c.Assert(name, Equals, "my-relation-relation-changed")
	c.Assert(s.dir.State().Members, DeepEquals, map[string]int{"u/1": 0})
	c.Assert(ctx.Units(), DeepEquals, []string{"u/1"})
	s1, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(s1, DeepEquals, map[string]interface{}{"private-address": "u-1.example.com"})

	// ...and commit it.
	err = r.CommitHook(changed)
	c.Assert(err, IsNil)
	c.Assert(s.dir.State().Members, DeepEquals, map[string]int{"u/1": 7})
	c.Assert(ctx.Units(), DeepEquals, []string{"u/1"})

	// To verify implied behaviour above, prepare a new joined hook with
	// missing membership information, and check relation context
	// membership is updated appropriately...
	joined.RemoteUnit = "u/2"
	joined.ChangeVersion = 3
	joined.Members = nil
	name, err = r.PrepareHook(joined)
	c.Assert(err, IsNil)
	c.Assert(s.dir.State().Members, HasLen, 1)
	c.Assert(name, Equals, "my-relation-relation-joined")
	c.Assert(ctx.Units(), DeepEquals, []string{"u/1", "u/2"})

	// ...and so is relation state on commit.
	err = r.CommitHook(joined)
	c.Assert(err, IsNil)
	c.Assert(s.dir.State().Members, DeepEquals, map[string]int{"u/1": 7, "u/2": 3})
	c.Assert(ctx.Units(), DeepEquals, []string{"u/1", "u/2"})
}

func (s *RelationerSuite) TestSetDying(c *C) {
	ru1 := s.AddRelationUnit(c, "u/1")
	err := ru1.Init()
	c.Assert(err, IsNil)
	err = ru1.Pinger().Start()
	c.Assert(err, IsNil)
	defer kill(c, ru1.Pinger())
	r := uniter.NewRelationer(s.ru, s.dir, s.hooks)
	err = r.Join()
	c.Assert(err, IsNil)
	defer r.Abandon()
	r.StartHooks()
	s.assertHook(c, hook.Info{
		Kind:       hook.RelationJoined,
		RemoteUnit: "u/1",
		Members: map[string]map[string]interface{}{
			"u/1": {"private-address": "u-1.example.com"},
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
	s.assertHook(c, hook.Info{Kind: hook.RelationChanged, RemoteUnit: "u/1"})
	s.assertHook(c, hook.Info{Kind: hook.RelationDeparted, RemoteUnit: "u/1"})
	s.assertHook(c, hook.Info{Kind: hook.RelationBroken})

	// Check that the relation state has been broken.
	err = s.dir.State().Validate(hook.Info{Kind: hook.RelationBroken})
	c.Assert(err, ErrorMatches, ".*: relation is broken and cannot be changed further")

	// TODO: when we have lifecycle handling, verify that the relation can
	// be destroyed. Can't see a clean way to do so currently.
}

func assertChanged(c *C, w *state.RelationUnitsWatcher) {
	select {
	case ch, ok := <-w.Changes():
		c.Assert(ok, Equals, true)
		c.Assert(ch.Changed, HasLen, 1)
		_, found := ch.Changed["u/0"]
		c.Assert(found, Equals, true)
		c.Assert(ch.Departed, HasLen, 0)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("timed out waiting for presence detection")
	}
}

func assertDeparted(c *C, w *state.RelationUnitsWatcher) {
	select {
	case ch, ok := <-w.Changes():
		c.Assert(ok, Equals, true)
		c.Assert(ch.Changed, HasLen, 0)
		c.Assert(ch.Departed, DeepEquals, []string{"u/0"})
	case <-time.After(5000 * time.Millisecond):
		c.Fatalf("timed out waiting for absence detection")
	}
}

func (s *RelationerSuite) assertNoHook(c *C) {
	select {
	case hi, ok := <-s.hooks:
		c.Fatalf("got unexpected hook info %#v (%b)", hi, ok)
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *RelationerSuite) assertHook(c *C, expect hook.Info) {
	select {
	case hi, ok := <-s.hooks:
		c.Assert(ok, Equals, true)
		c.Assert(hi, DeepEquals, expect)
		c.Assert(s.dir.Write(hi), Equals, nil)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("timed out waiting for %#v", expect)
	}
}

func kill(c *C, p *presence.Pinger) {
	c.Assert(p.Kill(), Equals, nil)
}

type stopper interface {
	Stop() error
}

func stop(c *C, s stopper) {
	c.Assert(s.Stop(), Equals, nil)
}

func abandon(c *C, r *uniter.Relationer) {
	c.Assert(r.Abandon(), Equals, nil)
}
