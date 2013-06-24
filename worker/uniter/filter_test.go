// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/worker"
	"launchpad.net/tomb"
	"time"
)

type FilterSuite struct {
	testing.JujuConnSuite
	wordpress  *state.Service
	unit       *state.Unit
	mysqlcharm *state.Charm
	wpcharm    *state.Charm
}

var _ = Suite(&FilterSuite{})

func (s *FilterSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	var err error
	s.wpcharm = s.AddTestingCharm(c, "wordpress")
	s.wordpress, err = s.State.AddService("wordpress", s.wpcharm)
	c.Assert(err, IsNil)
	s.unit, err = s.wordpress.AddUnit()
	c.Assert(err, IsNil)
	err = s.unit.AssignToNewMachine()
	c.Assert(err, IsNil)
	mid, err := s.unit.AssignedMachineId()
	c.Assert(err, IsNil)
	machine, err := s.State.Machine(mid)
	c.Assert(err, IsNil)
	err = machine.SetProvisioned("i-exist", "fake_nonce", nil)
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
	err = s.unit.SetStatus(params.StatusStarted, "")
	c.Assert(err, IsNil)
	err = s.unit.Destroy()
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
	s.assertTerminateAgent(c, f)
}

func (s *FilterSuite) TestUnitRemoval(c *C) {
	f, err := newFilter(s.State, s.unit.Name())
	c.Assert(err, IsNil)
	defer f.Stop()

	// short-circuit to remove because no status set.
	err = s.unit.Destroy()
	c.Assert(err, IsNil)
	s.assertTerminateAgent(c, f)
}

func (s *FilterSuite) assertTerminateAgent(c *C, f *filter) {
	s.State.StartSync()
	select {
	case <-f.Dead():
	case <-time.After(50 * time.Millisecond):
		c.Fatalf("dead not detected")
	}
	c.Assert(f.Wait(), Equals, worker.ErrTerminateAgent)
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

	err = s.unit.SetStatus(params.StatusStarted, "")
	c.Assert(err, IsNil)
	err = s.wordpress.Destroy()
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

	// Can't set s.wordpress to Dead while it still has units.
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
	err = s.unit.SetStatus(params.StatusError, "blarg")
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
		assertNoChange()
	}
	assertChange(state.ResolvedRetryHooks)

	// Ask for the event again, and check it's resent.
	f.WantResolvedEvent()
	assertChange(state.ResolvedRetryHooks)

	// Clear the resolved status *via the filter*; check not resent...
	err = f.ClearResolved()
	c.Assert(err, IsNil)
	assertNoChange()

	// ...even when requested.
	f.WantResolvedEvent()
	assertNoChange()

	// Induce several events; only latest state is reported.
	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, IsNil)
	err = f.ClearResolved()
	c.Assert(err, IsNil)
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, IsNil)
	assertChange(state.ResolvedNoHooks)
}

func (s *FilterSuite) TestCharmUpgradeEvents(c *C) {
	oldCharm := s.AddTestingCharm(c, "upgrade1")
	svc, err := s.State.AddService("upgradetest", oldCharm)
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)

	f, err := newFilter(s.State, unit.Name())
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

	// Setting a charm generates no new events if it already matches.
	err = f.SetCharm(oldCharm.URL())
	c.Assert(err, IsNil)
	assertNoChange()

	// Explicitly request an event relative to the existing state; nothing.
	f.WantUpgradeEvent(false)
	assertNoChange()

	// Change the service in an irrelevant way; no events.
	err = svc.SetExposed()
	c.Assert(err, IsNil)
	assertNoChange()

	// Change the service's charm; new event received.
	newCharm := s.AddTestingCharm(c, "upgrade2")
	err = svc.SetCharm(newCharm, false)
	c.Assert(err, IsNil)
	assertChange := func(url *charm.URL) {
		s.State.Sync()
		select {
		case upgradeCharm := <-f.UpgradeEvents():
			c.Assert(upgradeCharm, DeepEquals, url)
		case <-time.After(50 * time.Millisecond):
			c.Fatalf("timed out")
		}
	}
	assertChange(newCharm.URL())
	assertNoChange()

	// Request a new upgrade *unforced* upgrade event, we should see one.
	f.WantUpgradeEvent(false)
	assertChange(newCharm.URL())
	assertNoChange()

	// Request only *forced* upgrade events; nothing.
	f.WantUpgradeEvent(true)
	assertNoChange()

	// But when we have a forced upgrade to the same URL, no new event.
	err = svc.SetCharm(oldCharm, true)
	c.Assert(err, IsNil)
	assertNoChange()

	// ...but a *forced* change to a different URL should generate an event.
	err = svc.SetCharm(newCharm, true)
	assertChange(newCharm.URL())
	assertNoChange()
}

func (s *FilterSuite) TestConfigEvents(c *C) {
	f, err := newFilter(s.State, s.unit.Name())
	c.Assert(err, IsNil)
	defer f.Stop()

	// Test no changes before the charm URL is set.
	assertNoChange := func() {
		s.State.StartSync()
		select {
		case <-f.ConfigEvents():
			c.Fatalf("unexpected config event")
		case <-time.After(50 * time.Millisecond):
		}
	}
	assertNoChange()

	// Set the charm URL to trigger config events.
	err = f.SetCharm(s.wpcharm.URL())
	c.Assert(err, IsNil)
	assertChange := func() {
		s.State.Sync()
		select {
		case _, ok := <-f.ConfigEvents():
			c.Assert(ok, Equals, true)
		case <-time.After(50 * time.Millisecond):
			c.Fatalf("timed out")
		}
		assertNoChange()
	}
	assertChange()

	// Change the config; new event received.
	changeConfig := func(title interface{}) {
		err := s.wordpress.UpdateConfigSettings(charm.Settings{
			"blog-title": title,
		})
		c.Assert(err, IsNil)
	}
	changeConfig("20,000 leagues in the cloud")
	assertChange()

	// Change the config a few more times, then reset the events. We sync to
	// make sure the event has come into the filter before we tell it to discard
	// all received events.
	changeConfig(nil)
	changeConfig("the curious incident of the dog in the cloud")
	s.State.Sync()
	f.DiscardConfigEvent()
	assertNoChange()

	// Check that a filter's initial event works with DiscardConfigEvent
	// as expected.
	f, err = newFilter(s.State, s.unit.Name())
	c.Assert(err, IsNil)
	defer f.Stop()
	s.State.Sync()
	f.DiscardConfigEvent()
	assertNoChange()

	// Further changes are still collapsed as appropriate.
	changeConfig("forsooth")
	changeConfig("imagination failure")
	assertChange()
}

func (s *FilterSuite) TestCharmErrorEvents(c *C) {
	f, err := newFilter(s.State, s.unit.Name())
	c.Assert(err, IsNil)
	defer f.Stop()

	assertNoChange := func() {
		s.State.StartSync()
		select {
		case <-f.ConfigEvents():
			c.Fatalf("unexpected config event")
		case <-time.After(50 * time.Millisecond):
		}
	}
	assertDead := func(f *filter) {
		select {
		case <-f.Dead():
		case <-time.After(50 * time.Millisecond):
			c.Fatalf("filter did not die")
		}
	}

	// Check setting an invalid charm URL does not send events.
	err = f.SetCharm(charm.MustParseURL("cs:missing/one-1"))
	c.Assert(err, Equals, tomb.ErrDying)
	assertNoChange()
	assertDead(f)

	// Filter died after the error, so restart it.
	f, err = newFilter(s.State, s.unit.Name())
	c.Assert(err, IsNil)
	defer f.Stop()

	// Check with a nil charm URL, again no changes.
	err = f.SetCharm(nil)
	c.Assert(err, Equals, tomb.ErrDying)
	assertNoChange()
	assertDead(f)
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
		assertNoChange()
	}
	assertChange([]int{0, 1})

	// Add another relation, and change another's Life (by entering scope before
	// Destroy, thereby setting the relation to Dying); check event.
	s.addRelation(c)
	ru0, err := rel0.Unit(s.unit)
	c.Assert(err, IsNil)
	err = ru0.EnterScope(nil)
	c.Assert(err, IsNil)
	err = rel0.Destroy()
	c.Assert(err, IsNil)
	assertChange([]int{0, 2})

	// Remove a relation completely; check event.
	err = rel1.Destroy()
	c.Assert(err, IsNil)
	assertChange([]int{1})

	// Start a new filter, check initial event.
	f, err = newFilter(s.State, s.unit.Name())
	c.Assert(err, IsNil)
	defer f.Stop()
	assertChange([]int{0, 2})

	// Check setting the charm URL generates all new relation events.
	err = f.SetCharm(s.wpcharm.URL())
	c.Assert(err, IsNil)
	assertChange([]int{0, 2})
}

func (s *FilterSuite) addRelation(c *C) *state.Relation {
	if s.mysqlcharm == nil {
		s.mysqlcharm = s.AddTestingCharm(c, "mysql")
	}
	rels, err := s.wordpress.Relations()
	c.Assert(err, IsNil)
	svcName := fmt.Sprintf("mysql%d", len(rels))
	_, err = s.State.AddService(svcName, s.mysqlcharm)
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{svcName, "wordpress"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	return rel
}
