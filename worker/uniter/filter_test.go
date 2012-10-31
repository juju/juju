package uniter

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker"
	"time"
)

type FilterSuite struct {
	testing.JujuConnSuite
	ch   *state.Charm
	svc  *state.Service
	unit *state.Unit
}

var _ = Suite(&FilterSuite{})

func (s *FilterSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	s.ch = s.AddTestingCharm(c, "dummy")
	var err error
	s.svc, err = s.State.AddService("dummy", s.ch)
	c.Assert(err, IsNil)
	s.unit, err = s.svc.AddUnit()
	c.Assert(err, IsNil)
}

func (s *FilterSuite) TestUnitDeath(c *C) {
	f, err := newFilter(s.State, s.unit.Name())
	c.Assert(err, IsNil)
	defer f.Stop()
	assertNotClosed := func() {
		s.State.StartSync()
		select {
		case <-time.After(50 * time.Millisecond):
		case <-f.UnitDying():
			c.Fatalf("unexpected receive")
		}
	}
	assertNotClosed()

	// Irrelevant change.
	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, IsNil)
	assertNotClosed()

	// Set dying.
	err = s.unit.EnsureDying()
	c.Assert(err, IsNil)
	assertClosed := func() {
		s.State.StartSync()
		select {
		case <-time.After(50 * time.Millisecond):
			c.Fatalf("dying not detected")
		case _, ok := <-f.UnitDying():
			c.Assert(ok, Equals, false)
		}
	}
	assertClosed()

	// Another irrelevant change.
	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)
	assertClosed()

	// Set dead.
	err = s.unit.EnsureDead()
	c.Assert(err, IsNil)
	s.State.StartSync()
	select {
	case <-f.Dying():
	case <-time.After(50 * time.Millisecond):
		c.Fatalf("dead not detected")
	}
	c.Assert(f.Wait(), Equals, worker.ErrDead)
}

func (s *FilterSuite) TestServiceDeath(c *C) {
	f, err := newFilter(s.State, s.unit.Name())
	c.Assert(err, IsNil)
	defer f.Stop()
	s.State.StartSync()
	select {
	case <-time.After(50 * time.Millisecond):
	case <-f.UnitDying():
		c.Fatalf("unexpected receive")
	}

	err = s.svc.EnsureDying()
	c.Assert(err, IsNil)
	timeout := time.After(500 * time.Millisecond)
loop:
	for {
		select {
		case <-f.UnitDying():
			break loop
		case <-time.After(50 * time.Millisecond):
			s.State.StartSync()
		case <-timeout:
			c.Fatalf("dead not detected")
		}
	}
	err = s.unit.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.unit.Life(), Equals, state.Dying)

	// Can't set s.svc to Dead while it still has units.
}

func (s *FilterSuite) TestResolvedEvents(c *C) {
	f, err := newFilter(s.State, s.unit.Name())
	c.Assert(err, IsNil)
	defer f.Stop()

	// No initial event; not worth mentioning ResolvedNone.
	assertNoChange := func() {
		s.State.StartSync()
		select {
		case rm := <-f.ResolvedEvents():
			c.Fatalf("unexpected %#v", rm)
		case <-time.After(50 * time.Millisecond):
		}
	}
	assertNoChange()

	// Request an event; no interesting event is available.
	f.WantResolvedEvent()
	assertNoChange()

	// Change the unit in an irrelevant way; no events.
	err = s.unit.SetCharm(s.ch)
	c.Assert(err, IsNil)
	assertNoChange()

	// Change the unit's resolved to an interesting value; new event received.
	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, IsNil)
	assertChange := func(expect state.ResolvedMode) {
		s.State.Sync()
		select {
		case rm := <-f.ResolvedEvents():
			c.Assert(rm, Equals, expect)
		case <-time.After(50 * time.Millisecond):
			c.Fatalf("timed out")
		}
	}
	assertChange(state.ResolvedRetryHooks)
	assertNoChange()

	// Request a few events, and change the unit a few times; when
	// we finally receive, only the most recent state is sent.
	f.WantResolvedEvent()
	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)
	f.WantResolvedEvent()
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, IsNil)
	f.WantResolvedEvent()
	assertChange(state.ResolvedNoHooks)
	assertNoChange()
}

func (s *FilterSuite) TestCharmEvents(c *C) {
	f, err := newFilter(s.State, s.unit.Name())
	c.Assert(err, IsNil)
	defer f.Stop()

	// No initial event is sent.
	assertNoChange := func() {
		s.State.StartSync()
		select {
		case sch := <-f.UpgradeEvents():
			c.Fatalf("unexpected %#v", sch)
		case <-time.After(50 * time.Millisecond):
		}
	}
	assertNoChange()

	// Request an event relative to the existing state; nothing.
	f.WantUpgradeEvent(s.ch.URL(), false)
	assertNoChange()

	// Change the service in an irrelevant way; no events.
	err = s.svc.SetExposed()
	c.Assert(err, IsNil)
	assertNoChange()

	// Change the service's charm; new event received.
	ch := s.AddTestingCharm(c, "dummy-v2")
	err = s.svc.SetCharm(ch, false)
	c.Assert(err, IsNil)
	assertChange := func(url *charm.URL) {
		s.State.Sync()
		select {
		case sch := <-f.UpgradeEvents():
			c.Assert(sch.URL(), DeepEquals, url)
		case <-time.After(50 * time.Millisecond):
			c.Fatalf("timed out")
		}
	}
	assertChange(ch.URL())
	assertNoChange()

	// Request a change relative to the original state, unforced;
	// same event is sent.
	f.WantUpgradeEvent(s.ch.URL(), false)
	assertChange(ch.URL())
	assertNoChange()

	// Request a forced change relative to the initial state; no change...
	f.WantUpgradeEvent(s.ch.URL(), true)
	assertNoChange()

	// ...and still no change when we have a forced upgrade to that state...
	err = s.svc.SetCharm(s.ch, true)
	c.Assert(err, IsNil)
	assertNoChange()

	// ...but a *forced* change to a different charm does generate an event.
	err = s.svc.SetCharm(ch, true)
	assertChange(ch.URL())
	assertNoChange()
}

func (s *FilterSuite) TestConfigEvents(c *C) {
	f, err := newFilter(s.State, s.unit.Name())
	c.Assert(err, IsNil)
	defer f.Stop()

	// Initial event.
	assertChange := func() {
		s.State.Sync()
		select {
		case _, ok := <-f.ConfigEvents():
			c.Assert(ok, Equals, true)
		case <-time.After(50 * time.Millisecond):
			c.Fatalf("timed out")
		}
	}
	assertChange()
	assertNoChange := func() {
		s.State.StartSync()
		select {
		case <-f.ConfigEvents():
			c.Fatalf("unexpected config event")
		case <-time.After(50 * time.Millisecond):
		}
	}
	assertNoChange()

	// Change the config; new event received.
	node, err := s.svc.Config()
	c.Assert(err, IsNil)
	node.Set("skill-level", 9001)
	_, err = node.Write()
	c.Assert(err, IsNil)
	assertChange()
	assertNoChange()

	// Change the config a couple of times, then reset the events.
	node.Set("title", "20,000 leagues in the cloud")
	_, err = node.Write()
	c.Assert(err, IsNil)
	node.Set("outlook", "precipitous")
	_, err = node.Write()
	c.Assert(err, IsNil)
	s.State.Sync()
	f.DiscardConfigEvent()
	assertNoChange()

	// Check that a filter's initial event works with DiscardConfigEvent
	// as expected.
	f, err = newFilter(s.State, s.unit.Name())
	c.Assert(err, IsNil)
	defer f.Stop()
	f.DiscardConfigEvent()
	s.State.Sync()
	assertNoChange()

	// Further changes are still collapsed as appropriate.
	node.Set("skill-level", 123)
	_, err = node.Write()
	c.Assert(err, IsNil)
	node.Set("outlook", "expressive")
	_, err = node.Write()
	c.Assert(err, IsNil)
	assertChange()
	assertNoChange()
}

func (s *FilterSuite) TestRelationsEvents(c *C) {
	f, err := newFilter(s.State, s.unit.Name())
	c.Assert(err, IsNil)
	defer f.Stop()

	assertNoChange := func() {
		s.State.Sync()
		select {
		case ids := <-f.RelationsEvents():
			c.Fatalf("unexpected relations event %#v", ids)
		case <-time.After(50 * time.Millisecond):
		}
	}
	assertNoChange()

	// Add a couple of relations; check the event.
	rel0 := s.addRelation(c)
	rel1 := s.addRelation(c)
	assertChange := func(expect []int) {
		s.State.Sync()
		select {
		case got := <-f.RelationsEvents():
			c.Assert(got, DeepEquals, expect)
		case <-time.After(50 * time.Millisecond):
			c.Fatalf("timed out")
		}
	}
	assertChange([]int{0, 1})
	assertNoChange()

	// Add another relation, and change another's Life (by entering scope before
	// Destroy, thereby setting the relation to Dying); check event.
	s.addRelation(c)
	ru0, err := rel0.Unit(s.unit)
	c.Assert(err, IsNil)
	err = s.unit.SetPrivateAddress("x.example.com")
	c.Assert(err, IsNil)
	err = ru0.EnterScope()
	c.Assert(err, IsNil)
	err = rel0.Destroy()
	c.Assert(err, IsNil)
	assertChange([]int{0, 2})
	assertNoChange()

	// Remove a relation completely; check event.
	err = rel1.Destroy()
	c.Assert(err, IsNil)
	assertChange([]int{1})
	assertNoChange()

	// Start a new filter, check initial event.
	f, err = newFilter(s.State, s.unit.Name())
	c.Assert(err, IsNil)
	defer f.Stop()
	assertChange([]int{0, 2})
	assertNoChange()
}

func (s *FilterSuite) addRelation(c *C) *state.Relation {
	rels, err := s.svc.Relations()
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(state.Endpoint{
		"dummy", "ifce", fmt.Sprintf("rel%d", len(rels)), state.RolePeer, charm.ScopeGlobal,
	})
	c.Assert(err, IsNil)
	return rel
}
