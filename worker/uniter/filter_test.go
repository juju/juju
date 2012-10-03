package uniter

import (
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
	f := newFilter(s.unit)
	defer f.Stop()
	assertNotClosed := func() {
		s.State.StartSync()
		select {
		case <-time.After(50 * time.Millisecond):
		case <-f.unitDying():
			c.Fatalf("unexpected receive")
		}
	}
	assertNotClosed()

	// Irrelevant change.
	err := s.unit.SetResolved(state.ResolvedRetryHooks)
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
		case _, ok := <-f.unitDying():
			c.Assert(ok, Equals, false)
		}
	}
	assertClosed()

	// Another irrelevant change.
	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)
	assertClosed()

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
	f := newFilter(s.unit)
	defer f.Stop()
	s.State.StartSync()
	select {
	case <-time.After(50 * time.Millisecond):
	case <-f.unitDying():
		c.Fatalf("unexpected receive")
	}

	err := s.svc.EnsureDying()
	c.Assert(err, IsNil)
	timeout := time.After(500 * time.Millisecond)
loop:
	for {
		select {
		case <-f.unitDying():
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
	f := newFilter(s.unit)
	defer f.Stop()

	// Initial event.
	assertChange := func(expect state.ResolvedMode) {
		s.State.Sync()
		select {
		case rm := <-f.resolvedEvents():
			c.Assert(*rm, Equals, expect)
		case <-time.After(50 * time.Millisecond):
			c.Fatalf("timed out")
		}
	}
	assertChange(state.ResolvedNone)
	assertNoChange := func() {
		s.State.StartSync()
		select {
		case rm := <-f.resolvedEvents():
			c.Fatalf("unexpected %#v", rm)
		case <-time.After(50 * time.Millisecond):
		}
	}
	assertNoChange()

	// Request an event; it matches the previous one.
	f.wantResolvedEvent()
	assertChange(state.ResolvedNone)
	assertNoChange()

	// Change the unit in an irrelevant way; no events.
	err := s.unit.SetCharm(s.ch)
	c.Assert(err, IsNil)
	assertNoChange()

	// Change the unit's resolved; new event received.
	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, IsNil)
	assertChange(state.ResolvedRetryHooks)
	assertNoChange()

	// Request a few events, and change the unit a few times; when
	// we finally receive, only the most recent state is sent.
	f.wantResolvedEvent()
	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)
	f.wantResolvedEvent()
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, IsNil)
	f.wantResolvedEvent()
	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)
	f.wantResolvedEvent()
	assertChange(state.ResolvedNone)
	assertNoChange()
}

func (s *FilterSuite) TestCharmEvents(c *C) {
	f := newFilter(s.unit)
	defer f.Stop()

	// Initial event.
	assertChange := func(url *charm.URL, force bool) {
		s.State.Sync()
		select {
		case ch := <-f.charmEvents():
			c.Assert(ch, DeepEquals, &charmChange{url, force})
		case <-time.After(50 * time.Millisecond):
			c.Fatalf("timed out")
		}
	}
	assertChange(s.ch.URL(), false)
	assertNoChange := func() {
		s.State.StartSync()
		select {
		case ch := <-f.charmEvents():
			c.Fatalf("unexpected %#v", ch)
		case <-time.After(50 * time.Millisecond):
		}
	}
	assertNoChange()

	// Request an event; it matches the previous one.
	f.wantCharmEvent()
	assertChange(s.ch.URL(), false)
	assertNoChange()

	// Change the service in an irrelevant way; no events.
	err := s.svc.SetExposed()
	c.Assert(err, IsNil)
	assertNoChange()

	// Change the service's charm; new event received.
	err = s.svc.SetCharm(s.ch, true)
	c.Assert(err, IsNil)
	assertChange(s.ch.URL(), true)
	assertNoChange()

	// Request a few events, and change the unit a few times; when
	// we finally receive, only the most recent state is sent.
	f.wantCharmEvent()
	ch := s.AddTestingCharm(c, "dummy-v2")
	err = s.svc.SetCharm(ch, true)
	c.Assert(err, IsNil)
	f.wantCharmEvent()
	err = s.svc.SetCharm(ch, false)
	c.Assert(err, IsNil)
	f.wantCharmEvent()
	assertChange(ch.URL(), false)
	assertNoChange()
}

func (s *FilterSuite) TestConfigEvents(c *C) {
	f := newFilter(s.unit)
	defer f.Stop()

	// Initial event.
	assertChange := func() {
		s.State.Sync()
		select {
		case _, ok := <-f.configEvents():
			c.Assert(ok, Equals, true)
		case <-time.After(50 * time.Millisecond):
			c.Fatalf("timed out")
		}
	}
	assertChange()
	assertNoChange := func() {
		s.State.StartSync()
		select {
		case <-f.configEvents():
			c.Fatalf("unexpected config event")
		case <-time.After(50 * time.Millisecond):
		}
	}
	assertNoChange()

	// Request an event; it matches the previous one.
	f.wantConfigEvent()
	assertChange()
	assertNoChange()

	// Change the config; new event received.
	node, err := s.svc.Config()
	c.Assert(err, IsNil)
	node.Set("skill-level", 9001)
	_, err = node.Write()
	c.Assert(err, IsNil)
	assertChange()
	assertNoChange()

	// Request a few events, and change the unit a few times; when
	// we finally receive, only a single event is sent.
	f.wantConfigEvent()
	node.Set("title", "20,000 leagues in the cloud")
	_, err = node.Write()
	c.Assert(err, IsNil)
	f.wantConfigEvent()
	node.Set("outlook", "precipitous")
	_, err = node.Write()
	c.Assert(err, IsNil)
	f.wantConfigEvent()
	assertChange()
	assertNoChange()
}
