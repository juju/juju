package uniter_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/presence"
	"launchpad.net/juju-core/state/testing"
	"launchpad.net/juju-core/worker/uniter"
	"strings"
	"time"
)

type RelationerSuite struct {
	testing.StateSuite
	hooks chan uniter.HookInfo
	svc   *state.Service
	rel   *state.Relation
	ru    *state.RelationUnit
	rs    *uniter.RelationState
}

var _ = Suite(&RelationerSuite{})

func (s *RelationerSuite) SetUpTest(c *C) {
	s.StateSuite.SetUpTest(c)
	ch := s.AddTestingCharm(c, "dummy")
	var err error
	s.svc, err = s.State.AddService("u", ch)
	c.Assert(err, IsNil)
	s.rel, err = s.State.AddRelation(state.RelationEndpoint{
		"u", "ifce", "my-relation", state.RolePeer, charm.ScopeGlobal,
	})
	s.ru = s.AddRelationUnit(c, "u/0")
	s.rs, err = uniter.NewRelationState(c.MkDir(), s.rel.Id())
	c.Assert(err, IsNil)
	s.hooks = make(chan uniter.HookInfo)
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
	r := uniter.NewRelationer(s.ru, s.rs, s.hooks)

	// Check that a watcher does not consider u/0 to be alive.
	w := ru1.Watch()
	defer stop(c, w)
	ch, ok := <-w.Changes()
	c.Assert(ok, Equals, true)
	c.Assert(ch.Changed, HasLen, 0)
	c.Assert(ch.Departed, HasLen, 0)

	// Start u/0's pinger and check it is observed by u/1.
	err := r.StartPresence()
	c.Assert(err, IsNil)
	defer stopPresence(c, r)
	assertChanged(c, w)

	// Check that we can't start u/0's pinger again while it's running.
	f := func() { r.StartPresence() }
	c.Assert(f, PanicMatches, "presence already started!")

	// Stop the pinger and check the change is observed.
	err = r.StopPresence()
	c.Assert(err, IsNil)
	assertDeparted(c, w)

	// Stop it again to check that multiple stops are safe.
	err = r.StopPresence()
	c.Assert(err, IsNil)

	// Check we can start it again, and it works...
	err = r.StartPresence()
	c.Assert(err, IsNil)
	assertChanged(c, w)

	// ..and stop it again, and that works too.
	err = r.StopPresence()
	c.Assert(err, IsNil)
	assertDeparted(c, w)
}

func (s *RelationerSuite) TestStartStopHooks(c *C) {
	ru1 := s.AddRelationUnit(c, "u/1")
	ru2 := s.AddRelationUnit(c, "u/2")
	r := uniter.NewRelationer(s.ru, s.rs, s.hooks)

	// Check no hooks are being sent.
	s.assertNoHook(c)

	// Start hooks, and check that still no changes are sent.
	r.StartHooks()
	s.assertNoHook(c)

	// Check we can't start hooks again.
	f := func() { r.StartHooks() }
	c.Assert(f, PanicMatches, "hooks already started!")

	// Join u/1 to the relation, and check that we receive the expected hooks.
	p, err := ru1.Join()
	c.Assert(err, IsNil)
	defer kill(c, p)
	s.assertHook(c, uniter.HookInfo{
		HookKind:   "joined",
		RemoteUnit: "u/1",
		Members: map[string]map[string]interface{}{
			"u/1": {"private-address": "u-1.example.com"},
		},
	})
	s.assertHook(c, uniter.HookInfo{
		HookKind:   "changed",
		RemoteUnit: "u/1",
		Members: map[string]map[string]interface{}{
			"u/1": {"private-address": "u-1.example.com"},
		},
	})
	s.assertNoHook(c)

	// Stop hooks, make more changes, check no events.
	err = r.StopHooks()
	c.Assert(err, IsNil)
	kill(c, p)
	p, err = ru2.Join()
	c.Assert(err, IsNil)
	defer kill(c, p)
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
	s.assertHook(c, uniter.HookInfo{
		HookKind:   "departed",
		RemoteUnit: "u/1",
		Members:    map[string]map[string]interface{}{},
	})
	s.assertHook(c, uniter.HookInfo{
		HookKind:      "joined",
		ChangeVersion: 1,
		RemoteUnit:    "u/2",
		Members: map[string]map[string]interface{}{
			"u/2": {"private-address": "roehampton"},
		},
	})
	s.assertHook(c, uniter.HookInfo{
		HookKind:      "changed",
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
	r := uniter.NewRelationer(s.ru, s.rs, s.hooks)
	ctx := r.Context()
	c.Assert(ctx.Units(), HasLen, 0)

	// Check preparing an invalid hook changes nothing...
	changed := uniter.HookInfo{
		HookKind:      "changed",
		RemoteUnit:    "u/1",
		ChangeVersion: 7,
		Members: map[string]map[string]interface{}{
			"u/1": {"private-address": "glastonbury"},
		},
	}
	_, err := r.PrepareHook(changed)
	c.Assert(err, ErrorMatches, `inappropriate "changed" for "u/1": unit has not joined`)
	c.Assert(ctx.Units(), HasLen, 0)
	c.Assert(s.rs.Members, HasLen, 0)

	// ...as does committing an invalid hook.
	err = r.CommitHook(changed)
	c.Assert(err, ErrorMatches, `inappropriate "changed" for "u/1": unit has not joined`)
	c.Assert(ctx.Units(), HasLen, 0)
	c.Assert(s.rs.Members, HasLen, 0)

	// Check preparing a valid hook updates the context, but not persistent
	// relation state.
	joined := uniter.HookInfo{
		HookKind:   "joined",
		RemoteUnit: "u/1",
		Members: map[string]map[string]interface{}{
			"u/1": {"private-address": "u-1.example.com"},
		},
	}
	name, err := r.PrepareHook(joined)
	c.Assert(err, IsNil)
	c.Assert(s.rs.Members, HasLen, 0)
	c.Assert(name, Equals, "my-relation-relation-joined")
	c.Assert(ctx.Units(), DeepEquals, []string{"u/1"})
	s1, err := ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(s1, DeepEquals, joined.Members["u/1"])

	// Check that preparing the following hook fails as before...
	_, err = r.PrepareHook(changed)
	c.Assert(err, ErrorMatches, `inappropriate "changed" for "u/1": unit has not joined`)
	c.Assert(s.rs.Members, HasLen, 0)
	c.Assert(ctx.Units(), DeepEquals, []string{"u/1"})
	s1, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(s1, DeepEquals, joined.Members["u/1"])

	// ...but that committing the previous hook updates the persistent
	// relation state...
	err = r.CommitHook(joined)
	c.Assert(err, IsNil)
	c.Assert(msi(s.rs.Members), DeepEquals, msi{"u/1": 0})
	c.Assert(ctx.Units(), DeepEquals, []string{"u/1"})
	s1, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(s1, DeepEquals, joined.Members["u/1"])

	// ...and allows us to prepare the next hook.
	name, err = r.PrepareHook(changed)
	c.Assert(err, IsNil)
	c.Assert(name, Equals, "my-relation-relation-changed")
	c.Assert(msi(s.rs.Members), DeepEquals, msi{"u/1": 0})
	c.Assert(ctx.Units(), DeepEquals, []string{"u/1"})
	s1, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(s1, DeepEquals, changed.Members["u/1"])
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
	case <-time.After(500 * time.Millisecond):
	}
}

func (s *RelationerSuite) assertHook(c *C, expect uniter.HookInfo) {
	select {
	case hi, ok := <-s.hooks:
		c.Assert(ok, Equals, true)
		c.Assert(hi, DeepEquals, expect)
		c.Assert(s.rs.Commit(hi), Equals, nil)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("timed out waiting for %#v", expect)
	}
}

func kill(c *C, p *presence.Pinger) {
	c.Assert(p.Kill(), Equals, nil)
}

func stop(c *C, w *state.RelationUnitsWatcher) {
	c.Assert(w.Stop(), Equals, nil)
}

func stopPresence(c *C, r *uniter.Relationer) {
	c.Assert(r.StopPresence(), Equals, nil)
}
