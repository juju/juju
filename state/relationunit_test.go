// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

type RUs []*state.RelationUnit

type RelationUnitSuite struct {
	ConnSuite
}

var _ = gc.Suite(&RelationUnitSuite{})

func assertInScope(c *gc.C, ru *state.RelationUnit) {
	ok, err := ru.InScope()
	c.Assert(err, gc.IsNil)
	c.Assert(ok, jc.IsTrue)
}

func assertNotInScope(c *gc.C, ru *state.RelationUnit) {
	ok, err := ru.InScope()
	c.Assert(err, gc.IsNil)
	c.Assert(ok, jc.IsFalse)
}

func (s *RelationUnitSuite) TestReadSettingsErrors(c *gc.C) {
	riak := s.AddTestingService(c, "riak", s.AddTestingCharm(c, "riak"))
	u0, err := riak.AddUnit()
	c.Assert(err, gc.IsNil)
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, gc.IsNil)
	rel, err := s.State.EndpointsRelation(riakEP)
	c.Assert(err, gc.IsNil)
	ru0, err := rel.Unit(u0)
	c.Assert(err, gc.IsNil)

	_, err = ru0.ReadSettings("nonsense")
	c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "nonsense" in relation "riak:ring": "nonsense" is not a valid unit name`)
	_, err = ru0.ReadSettings("unknown/0")
	c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "unknown/0" in relation "riak:ring": service "unknown" is not a member of "riak:ring"`)
	_, err = ru0.ReadSettings("riak/pressure")
	c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "riak/pressure" in relation "riak:ring": "riak/pressure" is not a valid unit name`)
	_, err = ru0.ReadSettings("riak/1")
	c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "riak/1" in relation "riak:ring": settings not found`)
}

func (s *RelationUnitSuite) TestPeerSettings(c *gc.C) {
	pr := NewPeerRelation(c, s.State)
	rus := RUs{pr.ru0, pr.ru1}

	// Check missing settings cannot be read by any RU.
	for _, ru := range rus {
		_, err := ru.ReadSettings("riak/0")
		c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "riak/0" in relation "riak:ring": settings not found`)
	}

	// Add settings for one RU.
	assertNotInScope(c, pr.ru0)
	err := pr.ru0.EnterScope(map[string]interface{}{"gene": "kelly"})
	c.Assert(err, gc.IsNil)
	node, err := pr.ru0.Settings()
	c.Assert(err, gc.IsNil)
	node.Set("meme", "socially-awkward-penguin")
	_, err = node.Write()
	c.Assert(err, gc.IsNil)
	normal := map[string]interface{}{
		"gene": "kelly",
		"meme": "socially-awkward-penguin",
	}

	// Check settings can be read by every RU.
	assertSettings := func(u *state.Unit, expect map[string]interface{}) {
		for _, ru := range rus {
			m, err := ru.ReadSettings(u.Name())
			c.Assert(err, gc.IsNil)
			c.Assert(m, gc.DeepEquals, expect)
		}
	}
	assertSettings(pr.u0, normal)
	assertInScope(c, pr.ru0)

	// Check that EnterScope when scope already entered does not touch
	// settings at all.
	changed := map[string]interface{}{"foo": "bar"}
	err = pr.ru0.EnterScope(changed)
	c.Assert(err, gc.IsNil)
	assertSettings(pr.u0, normal)
	assertInScope(c, pr.ru0)

	// Leave scope, check settings are still as accessible as before.
	err = pr.ru0.LeaveScope()
	c.Assert(err, gc.IsNil)
	assertSettings(pr.u0, normal)
	assertNotInScope(c, pr.ru0)

	// Re-enter scope wih changed settings, and check they completely overwrite
	// the old ones.
	err = pr.ru0.EnterScope(changed)
	c.Assert(err, gc.IsNil)
	assertSettings(pr.u0, changed)
	assertInScope(c, pr.ru0)

	// Leave and re-enter with nil nettings, and check they overwrite to become
	// an empty map.
	err = pr.ru0.LeaveScope()
	c.Assert(err, gc.IsNil)
	assertNotInScope(c, pr.ru0)
	err = pr.ru0.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	assertSettings(pr.u0, map[string]interface{}{})
	assertInScope(c, pr.ru0)

	// Check that entering scope for the first time with nil settings works correctly.
	assertNotInScope(c, pr.ru1)
	err = pr.ru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	assertSettings(pr.u1, map[string]interface{}{})
	assertInScope(c, pr.ru1)
}

func (s *RelationUnitSuite) TestProReqSettings(c *gc.C) {
	prr := NewProReqRelation(c, &s.ConnSuite, charm.ScopeGlobal)
	rus := RUs{prr.pru0, prr.pru1, prr.rru0, prr.rru1}

	// Check missing settings cannot be read by any RU.
	for _, ru := range rus {
		_, err := ru.ReadSettings("mysql/0")
		c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "mysql/0" in relation "wordpress:db mysql:server": settings not found`)
	}

	// Add settings for one RU.
	assertNotInScope(c, prr.pru0)
	err := prr.pru0.EnterScope(map[string]interface{}{"gene": "simmons"})
	c.Assert(err, gc.IsNil)
	node, err := prr.pru0.Settings()
	c.Assert(err, gc.IsNil)
	node.Set("meme", "foul-bachelor-frog")
	_, err = node.Write()
	c.Assert(err, gc.IsNil)
	assertInScope(c, prr.pru0)

	// Check settings can be read by every RU.
	for _, ru := range rus {
		m, err := ru.ReadSettings("mysql/0")
		c.Assert(err, gc.IsNil)
		c.Assert(m["gene"], gc.Equals, "simmons")
		c.Assert(m["meme"], gc.Equals, "foul-bachelor-frog")
	}
}

func (s *RelationUnitSuite) TestContainerSettings(c *gc.C) {
	prr := NewProReqRelation(c, &s.ConnSuite, charm.ScopeContainer)
	rus := RUs{prr.pru0, prr.pru1, prr.rru0, prr.rru1}

	// Check missing settings cannot be read by any RU.
	for _, ru := range rus {
		_, err := ru.ReadSettings("logging/0")
		c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "logging/0" in relation "logging:info mysql:juju-info": settings not found`)
	}

	// Add settings for one RU.
	assertNotInScope(c, prr.pru0)
	err := prr.pru0.EnterScope(map[string]interface{}{"gene": "hackman"})
	c.Assert(err, gc.IsNil)
	node, err := prr.pru0.Settings()
	c.Assert(err, gc.IsNil)
	node.Set("meme", "foul-bachelor-frog")
	_, err = node.Write()
	c.Assert(err, gc.IsNil)
	assertInScope(c, prr.pru0)

	// Check settings can be read by RUs in the same container.
	rus0 := RUs{prr.pru0, prr.rru0}
	for _, ru := range rus0 {
		m, err := ru.ReadSettings("mysql/0")
		c.Assert(err, gc.IsNil)
		c.Assert(m["gene"], gc.Equals, "hackman")
		c.Assert(m["meme"], gc.Equals, "foul-bachelor-frog")
	}

	// Check settings are still inaccessible to RUs outside that container
	rus1 := RUs{prr.pru1, prr.rru1}
	for _, ru := range rus1 {
		_, err := ru.ReadSettings("mysql/0")
		c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "mysql/0" in relation "logging:info mysql:juju-info": settings not found`)
	}
}

func (s *RelationUnitSuite) TestContainerCreateSubordinate(c *gc.C) {
	psvc := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	rsvc := s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints([]string{"mysql", "logging"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	punit, err := psvc.AddUnit()
	c.Assert(err, gc.IsNil)
	pru, err := rel.Unit(punit)
	c.Assert(err, gc.IsNil)

	// Check that no units of the subordinate service exist.
	assertSubCount := func(expect int) []*state.Unit {
		runits, err := rsvc.AllUnits()
		c.Assert(err, gc.IsNil)
		c.Assert(runits, gc.HasLen, expect)
		return runits
	}
	assertSubCount(0)

	// Enter principal's scope and check a subordinate was created.
	assertNotInScope(c, pru)
	err = pru.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	assertSubCount(1)
	assertInScope(c, pru)

	// Enter principal scope again and check no more subordinates created.
	err = pru.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	assertSubCount(1)
	assertInScope(c, pru)

	// Leave principal scope, then re-enter, and check that still no further
	// subordinates are created.
	err = pru.LeaveScope()
	c.Assert(err, gc.IsNil)
	assertNotInScope(c, pru)
	err = pru.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	runits := assertSubCount(1)
	assertInScope(c, pru)

	// Set the subordinate to Dying, and enter scope again; because the scope
	// is already entered, no error is returned.
	runit := runits[0]
	err = runit.Destroy()
	c.Assert(err, gc.IsNil)
	err = pru.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	assertInScope(c, pru)

	// Leave scope, then try to enter again with the Dying subordinate.
	err = pru.LeaveScope()
	c.Assert(err, gc.IsNil)
	assertNotInScope(c, pru)
	err = pru.EnterScope(nil)
	c.Assert(err, gc.Equals, state.ErrCannotEnterScopeYet)
	assertNotInScope(c, pru)

	// Remove the subordinate, and enter scope again; this should work, and
	// create a new subordinate.
	err = runit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = runit.Remove()
	c.Assert(err, gc.IsNil)
	assertSubCount(0)
	assertNotInScope(c, pru)
	err = pru.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	assertSubCount(1)
	assertInScope(c, pru)
}

func (s *RelationUnitSuite) TestDestroyRelationWithUnitsInScope(c *gc.C) {
	pr := NewPeerRelation(c, s.State)
	rel := pr.ru0.Relation()

	// Enter two units, and check that Destroying the service sets the
	// relation to Dying (rather than removing it directly).
	assertNotInScope(c, pr.ru0)
	err := pr.ru0.EnterScope(map[string]interface{}{"some": "settings"})
	c.Assert(err, gc.IsNil)
	assertInScope(c, pr.ru0)
	assertNotInScope(c, pr.ru1)
	err = pr.ru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	assertInScope(c, pr.ru1)
	err = pr.svc.Destroy()
	c.Assert(err, gc.IsNil)
	err = rel.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(rel.Life(), gc.Equals, state.Dying)

	// Check that we can't add a new unit now.
	assertNotInScope(c, pr.ru2)
	err = pr.ru2.EnterScope(nil)
	c.Assert(err, gc.Equals, state.ErrCannotEnterScope)
	assertNotInScope(c, pr.ru2)

	// Check that we created no settings for the unit we failed to add.
	_, err = pr.ru0.ReadSettings("riak/2")
	c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "riak/2" in relation "riak:ring": settings not found`)

	// ru0 leaves the scope; check that service Destroy is still a no-op.
	assertInScope(c, pr.ru0)
	err = pr.ru0.LeaveScope()
	c.Assert(err, gc.IsNil)
	assertNotInScope(c, pr.ru0)
	err = pr.svc.Destroy()
	c.Assert(err, gc.IsNil)

	// Check that unit settings for the original unit still exist, and have
	// not yet been marked for deletion.
	err = s.State.Cleanup()
	c.Assert(err, gc.IsNil)
	assertSettings := func() {
		settings, err := pr.ru1.ReadSettings("riak/0")
		c.Assert(err, gc.IsNil)
		c.Assert(settings, gc.DeepEquals, map[string]interface{}{"some": "settings"})
	}
	assertSettings()

	// The final unit leaves the scope, and cleans up after itself.
	assertInScope(c, pr.ru1)
	err = pr.ru1.LeaveScope()
	c.Assert(err, gc.IsNil)
	assertNotInScope(c, pr.ru1)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	// The settings were not themselves actually deleted yet...
	assertSettings()

	// ...but they were scheduled for deletion.
	err = s.State.Cleanup()
	c.Assert(err, gc.IsNil)
	_, err = pr.ru1.ReadSettings("riak/0")
	c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "riak/0" in relation "riak:ring": settings not found`)
}

func (s *RelationUnitSuite) TestAliveRelationScope(c *gc.C) {
	pr := NewPeerRelation(c, s.State)
	rel := pr.ru0.Relation()

	// Two units enter...
	assertNotInScope(c, pr.ru0)
	err := pr.ru0.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	assertInScope(c, pr.ru0)
	assertNotInScope(c, pr.ru1)
	err = pr.ru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	assertInScope(c, pr.ru1)

	// One unit becomes Dying, then re-enters the scope; this is not an error,
	// because the state is already as requested.
	err = pr.u0.Destroy()
	c.Assert(err, gc.IsNil)
	err = pr.ru0.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	assertInScope(c, pr.ru0)

	// Two units leave...
	err = pr.ru0.LeaveScope()
	c.Assert(err, gc.IsNil)
	assertNotInScope(c, pr.ru0)
	err = pr.ru1.LeaveScope()
	c.Assert(err, gc.IsNil)
	assertNotInScope(c, pr.ru1)

	// The relation scope is empty, but the relation is still alive...
	err = rel.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(rel.Life(), gc.Equals, state.Alive)

	// ...and new units can still join it...
	assertNotInScope(c, pr.ru2)
	err = pr.ru2.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	assertInScope(c, pr.ru2)

	// ...but Dying units cannot.
	err = pr.u3.Destroy()
	c.Assert(err, gc.IsNil)
	assertNotInScope(c, pr.ru3)
	err = pr.ru3.EnterScope(nil)
	c.Assert(err, gc.Equals, state.ErrCannotEnterScope)
	assertNotInScope(c, pr.ru3)
}

func (s *StateSuite) TestWatchWatchScopeDiesOnStateClose(c *gc.C) {
	testWatcherDiesWhenStateCloses(c, func(c *gc.C, st *state.State) waiter {
		pr := NewPeerRelation(c, st)
		w := pr.ru0.WatchScope()
		<-w.Changes()
		return w
	})
}

func (s *RelationUnitSuite) TestPeerWatchScope(c *gc.C) {
	pr := NewPeerRelation(c, s.State)

	// Test empty initial event.
	w0 := pr.ru0.WatchScope()
	defer testing.AssertStop(c, w0)
	s.assertScopeChange(c, w0, nil, nil)
	s.assertNoScopeChange(c, w0)

	// ru0 enters; check no change, but settings written.
	assertNotInScope(c, pr.ru0)
	err := pr.ru0.EnterScope(map[string]interface{}{"foo": "bar"})
	c.Assert(err, gc.IsNil)
	s.assertNoScopeChange(c, w0)
	node, err := pr.ru0.Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node.Map(), gc.DeepEquals, map[string]interface{}{"foo": "bar"})
	assertInScope(c, pr.ru0)

	// ru1 enters; check change is observed.
	assertNotInScope(c, pr.ru1)
	err = pr.ru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	s.assertScopeChange(c, w0, []string{"riak/1"}, nil)
	s.assertNoScopeChange(c, w0)
	assertInScope(c, pr.ru1)

	// ru1 enters again, check no problems and no changes.
	err = pr.ru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	s.assertNoScopeChange(c, w0)
	assertInScope(c, pr.ru1)

	// Stop watching; ru2 enters.
	testing.AssertStop(c, w0)
	assertNotInScope(c, pr.ru2)
	err = pr.ru2.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	assertInScope(c, pr.ru2)

	// Start watch again, check initial event.
	w0 = pr.ru0.WatchScope()
	defer testing.AssertStop(c, w0)
	s.assertScopeChange(c, w0, []string{"riak/1", "riak/2"}, nil)
	s.assertNoScopeChange(c, w0)

	// ru1 leaves; check event.
	assertInScope(c, pr.ru1)
	err = pr.ru1.LeaveScope()
	c.Assert(err, gc.IsNil)
	s.assertScopeChange(c, w0, nil, []string{"riak/1"})
	s.assertNoScopeChange(c, w0)
	assertNotInScope(c, pr.ru1)

	// ru1 leaves again; check no problems and no changes.
	err = pr.ru1.LeaveScope()
	c.Assert(err, gc.IsNil)
	s.assertNoScopeChange(c, w0)
	assertNotInScope(c, pr.ru1)
}

func (s *RelationUnitSuite) TestProReqWatchScope(c *gc.C) {
	prr := NewProReqRelation(c, &s.ConnSuite, charm.ScopeGlobal)

	// Test empty initial events for all RUs.
	ws := prr.watches()
	for _, w := range ws {
		defer testing.AssertStop(c, w)
	}
	for _, w := range ws {
		s.assertScopeChange(c, w, nil, nil)
	}
	s.assertNoScopeChange(c, ws...)

	// pru0 enters; check detected only by req RUs.
	assertNotInScope(c, prr.pru0)
	err := prr.pru0.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	rws := func() []*state.RelationScopeWatcher {
		return []*state.RelationScopeWatcher{ws[2], ws[3]}
	}
	for _, w := range rws() {
		s.assertScopeChange(c, w, []string{"mysql/0"}, nil)
	}
	s.assertNoScopeChange(c, ws...)
	assertInScope(c, prr.pru0)

	// req0 enters; check detected only by pro RUs.
	assertNotInScope(c, prr.rru0)
	err = prr.rru0.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	pws := func() []*state.RelationScopeWatcher {
		return []*state.RelationScopeWatcher{ws[0], ws[1]}
	}
	for _, w := range pws() {
		s.assertScopeChange(c, w, []string{"wordpress/0"}, nil)
	}
	s.assertNoScopeChange(c, ws...)
	assertInScope(c, prr.rru0)

	// Stop watches; remaining RUs enter.
	for _, w := range ws {
		testing.AssertStop(c, w)
	}
	assertNotInScope(c, prr.pru1)
	err = prr.pru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	assertInScope(c, prr.pru1)
	assertNotInScope(c, prr.rru1)
	err = prr.rru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	assertInScope(c, prr.rru0)

	// Start new watches, check initial events.
	ws = prr.watches()
	for _, w := range ws {
		defer testing.AssertStop(c, w)
	}
	for _, w := range pws() {
		s.assertScopeChange(c, w, []string{"wordpress/0", "wordpress/1"}, nil)
	}
	for _, w := range rws() {
		s.assertScopeChange(c, w, []string{"mysql/0", "mysql/1"}, nil)
	}
	s.assertNoScopeChange(c, ws...)

	// pru0 leaves; check detected only by req RUs.
	assertInScope(c, prr.pru0)
	err = prr.pru0.LeaveScope()
	c.Assert(err, gc.IsNil)
	for _, w := range rws() {
		s.assertScopeChange(c, w, nil, []string{"mysql/0"})
	}
	s.assertNoScopeChange(c, ws...)
	assertNotInScope(c, prr.pru0)

	// rru0 leaves; check detected only by pro RUs.
	assertInScope(c, prr.rru0)
	err = prr.rru0.LeaveScope()
	c.Assert(err, gc.IsNil)
	for _, w := range pws() {
		s.assertScopeChange(c, w, nil, []string{"wordpress/0"})
	}
	s.assertNoScopeChange(c, ws...)
	assertNotInScope(c, prr.rru0)
}

func (s *RelationUnitSuite) TestContainerWatchScope(c *gc.C) {
	prr := NewProReqRelation(c, &s.ConnSuite, charm.ScopeContainer)

	// Test empty initial events for all RUs.
	ws := prr.watches()
	for _, w := range ws {
		defer testing.AssertStop(c, w)
	}
	for _, w := range ws {
		s.assertScopeChange(c, w, nil, nil)
	}
	s.assertNoScopeChange(c, ws...)

	// pru0 enters; check detected only by same-container req.
	assertNotInScope(c, prr.pru0)
	err := prr.pru0.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	s.assertScopeChange(c, ws[2], []string{"mysql/0"}, nil)
	s.assertNoScopeChange(c, ws...)
	assertInScope(c, prr.pru0)

	// req1 enters; check detected only by same-container pro.
	assertNotInScope(c, prr.rru1)
	err = prr.rru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	s.assertScopeChange(c, ws[1], []string{"logging/1"}, nil)
	s.assertNoScopeChange(c, ws...)
	assertInScope(c, prr.rru1)

	// Stop watches; remaining RUs enter scope.
	for _, w := range ws {
		testing.AssertStop(c, w)
	}
	assertNotInScope(c, prr.pru1)
	err = prr.pru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	assertNotInScope(c, prr.rru0)
	err = prr.rru0.EnterScope(nil)
	c.Assert(err, gc.IsNil)

	// Start new watches, check initial events.
	ws = prr.watches()
	for _, w := range ws {
		defer testing.AssertStop(c, w)
	}
	s.assertScopeChange(c, ws[0], []string{"logging/0"}, nil)
	s.assertScopeChange(c, ws[1], []string{"logging/1"}, nil)
	s.assertScopeChange(c, ws[2], []string{"mysql/0"}, nil)
	s.assertScopeChange(c, ws[3], []string{"mysql/1"}, nil)
	s.assertNoScopeChange(c, ws...)
	assertInScope(c, prr.pru1)
	assertInScope(c, prr.rru0)

	// pru0 leaves; check detected only by same-container req.
	assertInScope(c, prr.pru0)
	err = prr.pru0.LeaveScope()
	c.Assert(err, gc.IsNil)
	s.assertScopeChange(c, ws[2], nil, []string{"mysql/0"})
	s.assertNoScopeChange(c, ws...)
	assertNotInScope(c, prr.pru0)

	// rru0 leaves; check detected only by same-container pro.
	assertInScope(c, prr.rru0)
	err = prr.rru0.LeaveScope()
	c.Assert(err, gc.IsNil)
	s.assertScopeChange(c, ws[0], nil, []string{"logging/0"})
	s.assertNoScopeChange(c, ws...)
	assertNotInScope(c, prr.rru0)
}

func (s *RelationUnitSuite) assertScopeChange(c *gc.C, w *state.RelationScopeWatcher, entered, left []string) {
	s.State.StartSync()
	select {
	case ch, ok := <-w.Changes():
		c.Assert(ok, gc.Equals, true)
		sort.Strings(entered)
		sort.Strings(ch.Entered)
		c.Assert(ch.Entered, gc.DeepEquals, entered)
		sort.Strings(left)
		sort.Strings(ch.Left)
		c.Assert(ch.Left, gc.DeepEquals, left)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no change")
	}
}

func (s *RelationUnitSuite) assertNoScopeChange(c *gc.C, ws ...*state.RelationScopeWatcher) {
	s.State.StartSync()
	for _, w := range ws {
		select {
		case ch, ok := <-w.Changes():
			c.Fatalf("got unwanted change: %#v, %t", ch, ok)
		case <-time.After(coretesting.ShortWait):
		}
	}
}

type PeerRelation struct {
	rel                *state.Relation
	svc                *state.Service
	u0, u1, u2, u3     *state.Unit
	ru0, ru1, ru2, ru3 *state.RelationUnit
}

func NewPeerRelation(c *gc.C, st *state.State) *PeerRelation {
	svc := state.AddTestingService(c, st, "riak", state.AddTestingCharm(c, st, "riak"))
	ep, err := svc.Endpoint("ring")
	c.Assert(err, gc.IsNil)
	rel, err := st.EndpointsRelation(ep)
	c.Assert(err, gc.IsNil)
	pr := &PeerRelation{rel: rel, svc: svc}
	pr.u0, pr.ru0 = addRU(c, svc, rel, nil)
	pr.u1, pr.ru1 = addRU(c, svc, rel, nil)
	pr.u2, pr.ru2 = addRU(c, svc, rel, nil)
	pr.u3, pr.ru3 = addRU(c, svc, rel, nil)
	return pr
}

type ProReqRelation struct {
	rel                    *state.Relation
	psvc, rsvc             *state.Service
	pu0, pu1, ru0, ru1     *state.Unit
	pru0, pru1, rru0, rru1 *state.RelationUnit
}

func NewProReqRelation(c *gc.C, s *ConnSuite, scope charm.RelationScope) *ProReqRelation {
	psvc := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	var rsvc *state.Service
	if scope == charm.ScopeGlobal {
		rsvc = s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	} else {
		rsvc = s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	}
	eps, err := s.State.InferEndpoints([]string{"mysql", rsvc.Name()})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	prr := &ProReqRelation{rel: rel, psvc: psvc, rsvc: rsvc}
	prr.pu0, prr.pru0 = addRU(c, psvc, rel, nil)
	prr.pu1, prr.pru1 = addRU(c, psvc, rel, nil)
	if scope == charm.ScopeGlobal {
		prr.ru0, prr.rru0 = addRU(c, rsvc, rel, nil)
		prr.ru1, prr.rru1 = addRU(c, rsvc, rel, nil)
	} else {
		prr.ru0, prr.rru0 = addRU(c, rsvc, rel, prr.pu0)
		prr.ru1, prr.rru1 = addRU(c, rsvc, rel, prr.pu1)
	}
	return prr
}

func (prr *ProReqRelation) watches() []*state.RelationScopeWatcher {
	return []*state.RelationScopeWatcher{
		prr.pru0.WatchScope(), prr.pru1.WatchScope(),
		prr.rru0.WatchScope(), prr.rru1.WatchScope(),
	}
}

func addRU(c *gc.C, svc *state.Service, rel *state.Relation, principal *state.Unit) (*state.Unit, *state.RelationUnit) {
	// Given the service svc in the relation rel, add a unit of svc and create
	// a RelationUnit with rel. If principal is supplied, svc is assumed to be
	// subordinate and the unit will be created by temporarily entering the
	// relation's scope as the principal.
	var u *state.Unit
	if principal == nil {
		unit, err := svc.AddUnit()
		c.Assert(err, gc.IsNil)
		u = unit
	} else {
		origUnits, err := svc.AllUnits()
		c.Assert(err, gc.IsNil)
		pru, err := rel.Unit(principal)
		c.Assert(err, gc.IsNil)
		err = pru.EnterScope(nil) // to create the subordinate
		c.Assert(err, gc.IsNil)
		err = pru.LeaveScope() // to reset to initial expected state
		c.Assert(err, gc.IsNil)
		newUnits, err := svc.AllUnits()
		c.Assert(err, gc.IsNil)
		for _, unit := range newUnits {
			found := false
			for _, old := range origUnits {
				if unit.Name() == old.Name() {
					found = true
					break
				}
			}
			if !found {
				u = unit
				break
			}
		}
		c.Assert(u, gc.NotNil)
	}
	preventUnitDestroyRemove(c, u)
	ru, err := rel.Unit(u)
	c.Assert(err, gc.IsNil)
	return u, ru
}

type WatchScopeSuite struct {
	ConnSuite
}

var _ = gc.Suite(&WatchScopeSuite{})

func (s *WatchScopeSuite) TestPeer(c *gc.C) {
	// Create a service and get a peer relation.
	riak := s.AddTestingService(c, "riak", s.AddTestingCharm(c, "riak"))
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, gc.IsNil)
	rels, err := riak.Relations()
	c.Assert(err, gc.IsNil)
	c.Assert(rels, gc.HasLen, 1)
	rel := rels[0]

	// Add some units to the service and set their private addresses; get
	// the relevant RelationUnits.
	// (Private addresses should be set by their unit agents on
	// startup; this test does not include that, but Join expects
	// the information to be available, and uses it to populate the
	// relation settings node.)
	addUnit := func(i int) *state.RelationUnit {
		unit, err := riak.AddUnit()
		c.Assert(err, gc.IsNil)
		err = unit.SetPrivateAddress(fmt.Sprintf("riak%d.example.com", i))
		c.Assert(err, gc.IsNil)
		ru, err := rel.Unit(unit)
		c.Assert(err, gc.IsNil)
		c.Assert(ru.Endpoint(), gc.Equals, riakEP)
		return ru
	}
	ru0 := addUnit(0)
	ru1 := addUnit(1)
	ru2 := addUnit(2)

	// ---------- Single unit ----------

	// Start watching the relation from the perspective of the first unit.
	w0 := ru0.Watch()
	defer testing.AssertStop(c, w0)
	w0c := testing.NewRelationUnitsWatcherC(c, s.State, w0)
	w0c.AssertChange(nil, nil)
	w0c.AssertNoChange()

	// Join the first unit to the relation, and change the settings, and
	// check that nothing apparently happens.
	err = ru0.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	changeSettings(c, ru0)
	w0c.AssertNoChange()

	// ---------- Two units ----------

	// Now join another unit to the relation...
	err = ru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)

	// ...and check that the first relation unit sees the change.
	expectChanged := []string{"riak/1"}
	w0c.AssertChange(expectChanged, nil)
	w0c.AssertNoChange()

	// Join again, check it's a no-op.
	err = ru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	w0c.AssertNoChange()

	// Start watching the relation from the perspective of the second unit,
	// and check that it sees the right state.
	w1 := ru1.Watch()
	defer testing.AssertStop(c, w1)
	w1c := testing.NewRelationUnitsWatcherC(c, s.State, w1)
	expectChanged = []string{"riak/0"}
	w1c.AssertChange(expectChanged, nil)
	w1c.AssertNoChange()

	// ---------- Three units ----------

	// Whoa, it works. Ok, check the third unit's opinion of the state.
	w2 := ru2.Watch()
	defer testing.AssertStop(c, w2)
	w2c := testing.NewRelationUnitsWatcherC(c, s.State, w2)
	expectChanged = []string{"riak/0", "riak/1"}
	w2c.AssertChange(expectChanged, nil)
	w2c.AssertNoChange()

	// Join the third unit, and check the first and second units see it.
	err = ru2.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	expectChanged = []string{"riak/2"}
	w0c.AssertChange(expectChanged, nil)
	w0c.AssertNoChange()
	w1c.AssertChange(expectChanged, nil)
	w1c.AssertNoChange()

	// Change the second unit's settings, and check that only
	// the first and third see changes.
	changeSettings(c, ru1)
	w1c.AssertNoChange()
	expectChanged = []string{"riak/1"}
	w0c.AssertChange(expectChanged, nil)
	w0c.AssertNoChange()
	w2c.AssertChange(expectChanged, nil)
	w2c.AssertNoChange()

	// ---------- Two units again ----------

	// Depart the second unit, and check that the first and third detect it.
	err = ru1.LeaveScope()
	c.Assert(err, gc.IsNil)
	expectDeparted := []string{"riak/1"}
	w0c.AssertChange(nil, expectDeparted)
	w0c.AssertNoChange()
	w2c.AssertChange(nil, expectDeparted)
	w2c.AssertNoChange()

	// Change its settings, and check the others don't observe anything.
	changeSettings(c, ru1)
	w0c.AssertNoChange()
	w2c.AssertNoChange()

	// Check no spurious events showed up on the second unit's watch, and check
	// it closes cleanly.
	w1c.AssertNoChange()
	testing.AssertStop(c, w1)

	// OK, we're done here. Cleanup, and error detection during same,
	// will be handled by the deferred kill/stop calls. Phew.
}

func (s *WatchScopeSuite) TestProviderRequirerGlobal(c *gc.C) {
	// Create a pair of services and a relation between them.
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, gc.IsNil)
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(mysqlEP, wordpressEP)
	c.Assert(err, gc.IsNil)

	// Add some units to the services and set their private addresses.
	addUnit := func(srv *state.Service, sub string, ep state.Endpoint) *state.RelationUnit {
		unit, err := srv.AddUnit()
		c.Assert(err, gc.IsNil)
		ru, err := rel.Unit(unit)
		c.Assert(err, gc.IsNil)
		c.Assert(ru.Endpoint(), gc.Equals, ep)
		return ru
	}
	msru0 := addUnit(mysql, "ms0", mysqlEP)
	msru1 := addUnit(mysql, "ms1", mysqlEP)
	wpru0 := addUnit(wordpress, "wp0", wordpressEP)
	wpru1 := addUnit(wordpress, "wp1", wordpressEP)

	// ---------- Single role active ----------

	// Watch the relation from the perspective of the first provider unit and
	// check initial event.
	msw0 := msru0.Watch()
	defer testing.AssertStop(c, msw0)
	msw0c := testing.NewRelationUnitsWatcherC(c, s.State, msw0)
	msw0c.AssertChange(nil, nil)
	msw0c.AssertNoChange()

	// Join the unit to the relation, change its settings, and check that
	// nothing apparently happens.
	err = msru0.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	changeSettings(c, msru0)
	msw0c.AssertNoChange()

	// Join the second provider unit, start its watch, and check what it thinks the
	// state of the relation is.
	err = msru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	msw1 := msru1.Watch()
	defer testing.AssertStop(c, msw1)
	msw1c := testing.NewRelationUnitsWatcherC(c, s.State, msw1)
	msw1c.AssertChange(nil, nil)
	msw1c.AssertNoChange()

	// Change the unit's settings, and check that neither provider unit
	// observes any change.
	changeSettings(c, msru1)
	msw1c.AssertNoChange()
	msw0c.AssertNoChange()

	// ---------- Two roles active ----------

	// Start watches from both requirer units' perspectives, and check that
	// they see the provider units.
	expectChanged := []string{"mysql/0", "mysql/1"}
	wpw0 := wpru0.Watch()
	defer testing.AssertStop(c, wpw0)
	wpw0c := testing.NewRelationUnitsWatcherC(c, s.State, wpw0)
	wpw0c.AssertChange(expectChanged, nil)
	wpw0c.AssertNoChange()
	wpw1 := wpru1.Watch()
	defer testing.AssertStop(c, wpw1)
	wpw1c := testing.NewRelationUnitsWatcherC(c, s.State, wpw1)
	wpw1c.AssertChange(expectChanged, nil)
	wpw1c.AssertNoChange()

	// Join the first requirer unit, and check the provider units see it.
	err = wpru0.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	expectChanged = []string{"wordpress/0"}
	msw0c.AssertChange(expectChanged, nil)
	msw0c.AssertNoChange()
	msw1c.AssertChange(expectChanged, nil)
	msw1c.AssertNoChange()

	// Join again, check no-op.
	err = wpru0.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	msw0c.AssertNoChange()
	msw1c.AssertNoChange()

	// Join the second requirer, and check the provider units see the change.
	err = wpru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	expectChanged = []string{"wordpress/1"}
	msw0c.AssertChange(expectChanged, nil)
	msw0c.AssertNoChange()
	msw1c.AssertChange(expectChanged, nil)
	msw1c.AssertNoChange()

	// Verify that neither requirer has observed any change to the relation.
	wpw0c.AssertNoChange()
	wpw1c.AssertNoChange()

	// Change settings for the first requirer, check providers see it...
	changeSettings(c, wpru0)
	expectChanged = []string{"wordpress/0"}
	msw0c.AssertChange(expectChanged, nil)
	msw0c.AssertNoChange()
	msw1c.AssertChange(expectChanged, nil)
	msw1c.AssertNoChange()

	// ...and requirers don't.
	wpw0c.AssertNoChange()
	wpw1c.AssertNoChange()

	// Depart the second requirer and check the providers see it...
	err = wpru1.LeaveScope()
	c.Assert(err, gc.IsNil)
	expectDeparted := []string{"wordpress/1"}
	msw0c.AssertChange(nil, expectDeparted)
	msw0c.AssertNoChange()
	msw1c.AssertChange(nil, expectDeparted)
	msw1c.AssertNoChange()

	// ...and the requirers don't.
	wpw0c.AssertNoChange()
	wpw1c.AssertNoChange()

	// Cleanup handled by defers as before.
}

func (s *WatchScopeSuite) TestProviderRequirerContainer(c *gc.C) {
	// Create a pair of services and a relation between them.
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("juju-info")
	c.Assert(err, gc.IsNil)
	logging := s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	loggingEP, err := logging.Endpoint("info")
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(mysqlEP, loggingEP)
	c.Assert(err, gc.IsNil)

	// Change mysqlEP to match the endpoint that will actually be used by the relation.
	mysqlEP.Scope = charm.ScopeContainer

	// Add some units to the services and set their private addresses.
	addUnits := func(i int) (*state.RelationUnit, *state.RelationUnit) {
		msu, err := mysql.AddUnit()
		c.Assert(err, gc.IsNil)
		msru, err := rel.Unit(msu)
		c.Assert(err, gc.IsNil)
		c.Assert(msru.Endpoint(), gc.Equals, mysqlEP)
		err = msru.EnterScope(nil)
		c.Assert(err, gc.IsNil)
		err = msru.LeaveScope()
		c.Assert(err, gc.IsNil)
		lgu, err := s.State.Unit("logging/" + strconv.Itoa(i))
		c.Assert(err, gc.IsNil)
		lgru, err := rel.Unit(lgu)
		c.Assert(err, gc.IsNil)
		c.Assert(lgru.Endpoint(), gc.Equals, loggingEP)
		return msru, lgru
	}
	msru0, lgru0 := addUnits(0)
	msru1, lgru1 := addUnits(1)

	// ---------- Single role active ----------

	// Start watching the relation from the perspective of the first unit, and
	// check the initial event.
	msw0 := msru0.Watch()
	defer testing.AssertStop(c, msw0)
	msw0c := testing.NewRelationUnitsWatcherC(c, s.State, msw0)
	msw0c.AssertChange(nil, nil)
	msw0c.AssertNoChange()

	// Join the unit to the relation, change its settings, and check that
	// nothing apparently happens.
	err = msru0.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	changeSettings(c, msru0)
	msw0c.AssertNoChange()

	// Watch the relation from the perspective of the second provider, and
	// check initial event.
	msw1 := msru1.Watch()
	defer testing.AssertStop(c, msw1)
	msw1c := testing.NewRelationUnitsWatcherC(c, s.State, msw1)
	msw1c.AssertChange(nil, nil)
	msw1c.AssertNoChange()

	// Join the second provider unit to the relation, and check that neither
	// watching unit observes any change.
	err = msru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	msw1c.AssertNoChange()
	msw0c.AssertNoChange()

	// Change the unit's settings, and check that nothing apparently happens.
	changeSettings(c, msru1)
	msw1c.AssertNoChange()
	msw0c.AssertNoChange()

	// ---------- Two roles active ----------

	// Start a watch from the first requirer unit's perspective, and check it
	// only sees the first provider (with which it shares a container).
	lgw0 := lgru0.Watch()
	defer testing.AssertStop(c, lgw0)
	lgw0c := testing.NewRelationUnitsWatcherC(c, s.State, lgw0)
	expectChanged := []string{"mysql/0"}
	lgw0c.AssertChange(expectChanged, nil)
	lgw0c.AssertNoChange()

	// Join the first requirer unit, and check that only the first provider
	// observes the change.
	err = lgru0.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	expectChanged = []string{"logging/0"}
	msw0c.AssertChange(expectChanged, nil)
	msw0c.AssertNoChange()
	msw1c.AssertNoChange()
	lgw0c.AssertNoChange()

	// Watch from the second requirer's perspective, and check it only sees the
	// second provider.
	lgw1 := lgru1.Watch()
	defer testing.AssertStop(c, lgw1)
	lgw1c := testing.NewRelationUnitsWatcherC(c, s.State, lgw1)
	expectChanged = []string{"mysql/1"}
	lgw1c.AssertChange(expectChanged, nil)
	lgw1c.AssertNoChange()

	// Join the second requirer, and check that the first provider observes it...
	err = lgru1.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	expectChanged = []string{"logging/1"}
	msw1c.AssertChange(expectChanged, nil)
	msw1c.AssertNoChange()

	// ...and that nothing else sees anything.
	msw0c.AssertNoChange()
	lgw0c.AssertNoChange()
	lgw1c.AssertNoChange()

	// Change the second provider's settings and check that the second
	// requirer notices...
	changeSettings(c, msru1)
	expectChanged = []string{"mysql/1"}
	lgw1c.AssertChange(expectChanged, nil)
	lgw1c.AssertNoChange()

	// ...but that nothing else does.
	msw0c.AssertNoChange()
	msw1c.AssertNoChange()
	msw0c.AssertNoChange()

	// Finally, depart the first provider, and check that only the first
	// requirer observes any change.
	err = msru0.LeaveScope()
	c.Assert(err, gc.IsNil)
	expectDeparted := []string{"mysql/0"}
	lgw0c.AssertChange(nil, expectDeparted)
	lgw0c.AssertNoChange()
	lgw1c.AssertNoChange()
	msw0c.AssertNoChange()
	msw1c.AssertNoChange()

	// Again, I think we're done, and can be comfortable that the appropriate
	// connections are in place.
}

func changeSettings(c *gc.C, ru *state.RelationUnit) {
	node, err := ru.Settings()
	c.Assert(err, gc.IsNil)
	value, _ := node.Get("value")
	v, _ := value.(int)
	node.Set("value", v+1)
	_, err = node.Write()
	c.Assert(err, gc.IsNil)
}
