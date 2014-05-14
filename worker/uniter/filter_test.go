// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"time"

	gc "launchpad.net/gocheck"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/charm"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	apiuniter "launchpad.net/juju-core/state/api/uniter"
	statetesting "launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/worker"
)

type FilterSuite struct {
	jujutesting.JujuConnSuite
	wordpress  *state.Service
	unit       *state.Unit
	mysqlcharm *state.Charm
	wpcharm    *state.Charm

	st     *api.State
	uniter *apiuniter.State
}

var _ = gc.Suite(&FilterSuite{})

func (s *FilterSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.wpcharm = s.AddTestingCharm(c, "wordpress")
	s.wordpress = s.AddTestingService(c, "wordpress", s.wpcharm)
	var err error
	s.unit, err = s.wordpress.AddUnit()
	c.Assert(err, gc.IsNil)
	err = s.unit.AssignToNewMachine()
	c.Assert(err, gc.IsNil)
	mid, err := s.unit.AssignedMachineId()
	c.Assert(err, gc.IsNil)
	machine, err := s.State.Machine(mid)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("i-exist", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	s.APILogin(c, s.unit)
}

func (s *FilterSuite) APILogin(c *gc.C, unit *state.Unit) {
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = unit.SetPassword(password)
	c.Assert(err, gc.IsNil)
	s.st = s.OpenAPIAs(c, unit.Tag(), password)
	s.uniter = s.st.Uniter()
	c.Assert(s.uniter, gc.NotNil)
}

func (s *FilterSuite) TestUnitDeath(c *gc.C) {
	f, err := newFilter(s.uniter, s.unit.Tag())
	c.Assert(err, gc.IsNil)
	defer f.Stop() // no AssertStop, we test for an error below
	asserter := coretesting.NotifyAsserterC{
		Precond: func() { s.BackingState.StartSync() },
		C:       c,
		Chan:    f.UnitDying(),
	}
	asserter.AssertNoReceive()

	// Irrelevant change.
	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, gc.IsNil)
	asserter.AssertNoReceive()

	// Set dying.
	err = s.unit.SetStatus(params.StatusStarted, "", nil)
	c.Assert(err, gc.IsNil)
	err = s.unit.Destroy()
	c.Assert(err, gc.IsNil)
	asserter.AssertClosed()

	// Another irrelevant change.
	err = s.unit.ClearResolved()
	c.Assert(err, gc.IsNil)
	asserter.AssertClosed()

	// Set dead.
	err = s.unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	s.assertAgentTerminates(c, f)
}

func (s *FilterSuite) TestUnitRemoval(c *gc.C) {
	f, err := newFilter(s.uniter, s.unit.Tag())
	c.Assert(err, gc.IsNil)
	defer f.Stop() // no AssertStop, we test for an error below

	// short-circuit to remove because no status set.
	err = s.unit.Destroy()
	c.Assert(err, gc.IsNil)
	s.assertAgentTerminates(c, f)
}

// Ensure we get a signal on f.Dead()
func (s *FilterSuite) assertFilterDies(c *gc.C, f *filter) {
	asserter := coretesting.NotifyAsserterC{
		Precond: func() { s.BackingState.StartSync() },
		C:       c,
		Chan:    f.Dead(),
	}
	asserter.AssertClosed()
}

func (s *FilterSuite) assertAgentTerminates(c *gc.C, f *filter) {
	s.assertFilterDies(c, f)
	c.Assert(f.Wait(), gc.Equals, worker.ErrTerminateAgent)
}

func (s *FilterSuite) TestServiceDeath(c *gc.C) {
	f, err := newFilter(s.uniter, s.unit.Tag())
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, f)
	dyingAsserter := coretesting.NotifyAsserterC{
		C:       c,
		Precond: func() { s.BackingState.StartSync() },
		Chan:    f.UnitDying(),
	}
	dyingAsserter.AssertNoReceive()

	err = s.unit.SetStatus(params.StatusStarted, "", nil)
	c.Assert(err, gc.IsNil)
	err = s.wordpress.Destroy()
	c.Assert(err, gc.IsNil)

	timeout := time.After(coretesting.LongWait)
loop:
	for {
		select {
		case <-f.UnitDying():
			break loop
		case <-time.After(coretesting.ShortWait):
			s.BackingState.StartSync()
		case <-timeout:
			c.Fatalf("dead not detected")
		}
	}
	err = s.unit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.unit.Life(), gc.Equals, state.Dying)

	// Can't set s.wordpress to Dead while it still has units.
}

func (s *FilterSuite) TestResolvedEvents(c *gc.C) {
	f, err := newFilter(s.uniter, s.unit.Tag())
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, f)

	resolvedAsserter := coretesting.ContentAsserterC{
		C:       c,
		Precond: func() { s.BackingState.StartSync() },
		Chan:    f.ResolvedEvents(),
	}
	resolvedAsserter.AssertNoReceive()

	// Request an event; no interesting event is available.
	f.WantResolvedEvent()
	resolvedAsserter.AssertNoReceive()

	// Change the unit in an irrelevant way; no events.
	err = s.unit.SetStatus(params.StatusError, "blarg", nil)
	c.Assert(err, gc.IsNil)
	resolvedAsserter.AssertNoReceive()

	// Change the unit's resolved to an interesting value; new event received.
	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, gc.IsNil)
	assertChange := func(expect params.ResolvedMode) {
		rm := resolvedAsserter.AssertOneReceive().(params.ResolvedMode)
		c.Assert(rm, gc.Equals, expect)
	}
	assertChange(params.ResolvedRetryHooks)

	// Ask for the event again, and check it's resent.
	f.WantResolvedEvent()
	assertChange(params.ResolvedRetryHooks)

	// Clear the resolved status *via the filter*; check not resent...
	err = f.ClearResolved()
	c.Assert(err, gc.IsNil)
	resolvedAsserter.AssertNoReceive()

	// ...even when requested.
	f.WantResolvedEvent()
	resolvedAsserter.AssertNoReceive()

	// Induce several events; only latest state is reported.
	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, gc.IsNil)
	err = f.ClearResolved()
	c.Assert(err, gc.IsNil)
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, gc.IsNil)
	assertChange(params.ResolvedNoHooks)
}

func (s *FilterSuite) TestCharmUpgradeEvents(c *gc.C) {
	oldCharm := s.AddTestingCharm(c, "upgrade1")
	svc := s.AddTestingService(c, "upgradetest", oldCharm)
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)

	s.APILogin(c, unit)

	f, err := newFilter(s.uniter, unit.Tag())
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, f)

	// No initial event is sent.
	assertNoChange := func() {
		s.BackingState.StartSync()
		select {
		case sch := <-f.UpgradeEvents():
			c.Fatalf("unexpected %#v", sch)
		case <-time.After(coretesting.ShortWait):
		}
	}
	assertNoChange()

	// Setting a charm generates no new events if it already matches.
	err = f.SetCharm(oldCharm.URL())
	c.Assert(err, gc.IsNil)
	assertNoChange()

	// Explicitly request an event relative to the existing state; nothing.
	f.WantUpgradeEvent(false)
	assertNoChange()

	// Change the service in an irrelevant way; no events.
	err = svc.SetExposed()
	c.Assert(err, gc.IsNil)
	assertNoChange()

	// Change the service's charm; new event received.
	newCharm := s.AddTestingCharm(c, "upgrade2")
	err = svc.SetCharm(newCharm, false)
	c.Assert(err, gc.IsNil)
	assertChange := func(url *charm.URL) {
		s.BackingState.StartSync()
		select {
		case upgradeCharm := <-f.UpgradeEvents():
			c.Assert(upgradeCharm, gc.DeepEquals, url)
		case <-time.After(coretesting.LongWait):
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
	c.Assert(err, gc.IsNil)
	assertNoChange()

	// ...but a *forced* change to a different URL should generate an event.
	err = svc.SetCharm(newCharm, true)
	assertChange(newCharm.URL())
	assertNoChange()
}

func (s *FilterSuite) TestConfigEvents(c *gc.C) {
	f, err := newFilter(s.uniter, s.unit.Tag())
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, f)

	// Test no changes before the charm URL is set.
	assertNoChange := func() {
		s.BackingState.StartSync()
		select {
		case <-f.ConfigEvents():
			c.Fatalf("unexpected config event")
		case <-time.After(coretesting.ShortWait):
		}
	}
	assertNoChange()

	// Set the charm URL to trigger config events.
	err = f.SetCharm(s.wpcharm.URL())
	c.Assert(err, gc.IsNil)
	assertChange := func() {
		s.BackingState.StartSync()
		select {
		case _, ok := <-f.ConfigEvents():
			c.Assert(ok, gc.Equals, true)
		case <-time.After(coretesting.LongWait):
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
		c.Assert(err, gc.IsNil)
	}
	changeConfig("20,000 leagues in the cloud")
	assertChange()

	// Change the config a few more times, then reset the events. We sync to
	// make sure the events have arrived in the watcher -- and then wait a
	// little longer, to allow for the delay while the events are coalesced
	// -- before we tell it to discard all received events. This would be
	// much better tested by controlling a mocked-out watcher directly, but
	// that's a bit inconvenient for this change.
	changeConfig(nil)
	changeConfig("the curious incident of the dog in the cloud")
	s.BackingState.StartSync()
	time.Sleep(250 * time.Millisecond)
	f.DiscardConfigEvent()
	assertNoChange()

	// Check that a filter's initial event works with DiscardConfigEvent
	// as expected.
	f, err = newFilter(s.uniter, s.unit.Tag())
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, f)
	s.BackingState.StartSync()
	f.DiscardConfigEvent()
	assertNoChange()

	// Further changes are still collapsed as appropriate.
	changeConfig("forsooth")
	changeConfig("imagination failure")
	assertChange()
}

func (s *FilterSuite) TestCharmErrorEvents(c *gc.C) {
	f, err := newFilter(s.uniter, s.unit.Tag())
	c.Assert(err, gc.IsNil)
	defer f.Stop() // no AssertStop, we test for an error below

	assertNoChange := func() {
		s.BackingState.StartSync()
		select {
		case <-f.ConfigEvents():
			c.Fatalf("unexpected config event")
		case <-time.After(coretesting.ShortWait):
		}
	}

	// Check setting an invalid charm URL does not send events.
	err = f.SetCharm(charm.MustParseURL("cs:missing/one-1"))
	c.Assert(err, gc.Equals, tomb.ErrDying)
	assertNoChange()
	s.assertFilterDies(c, f)

	// Filter died after the error, so restart it.
	f, err = newFilter(s.uniter, s.unit.Tag())
	c.Assert(err, gc.IsNil)
	defer f.Stop() // no AssertStop, we test for an error below

	// Check with a nil charm URL, again no changes.
	err = f.SetCharm(nil)
	c.Assert(err, gc.Equals, tomb.ErrDying)
	assertNoChange()
	s.assertFilterDies(c, f)
}

func (s *FilterSuite) TestRelationsEvents(c *gc.C) {
	f, err := newFilter(s.uniter, s.unit.Tag())
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, f)

	assertNoChange := func() {
		s.BackingState.StartSync()
		select {
		case ids := <-f.RelationsEvents():
			c.Fatalf("unexpected relations event %#v", ids)
		case <-time.After(coretesting.ShortWait):
		}
	}
	assertNoChange()

	// Add a couple of relations; check the event.
	rel0 := s.addRelation(c)
	rel1 := s.addRelation(c)
	assertChange := func(expect []int) {
		s.BackingState.StartSync()
		select {
		case got := <-f.RelationsEvents():
			c.Assert(got, gc.DeepEquals, expect)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out")
		}
		assertNoChange()
	}
	assertChange([]int{0, 1})

	// Add another relation, and change another's Life (by entering scope before
	// Destroy, thereby setting the relation to Dying); check event.
	s.addRelation(c)
	ru0, err := rel0.Unit(s.unit)
	c.Assert(err, gc.IsNil)
	err = ru0.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	err = rel0.Destroy()
	c.Assert(err, gc.IsNil)
	assertChange([]int{0, 2})

	// Remove a relation completely; check no event, because the relation
	// could not have been removed if the unit was in scope, and therefore
	// the uniter never needs to hear about it.
	err = rel1.Destroy()
	c.Assert(err, gc.IsNil)
	assertNoChange()
	err = f.Stop()
	c.Assert(err, gc.IsNil)

	// Start a new filter, check initial event.
	f, err = newFilter(s.uniter, s.unit.Tag())
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, f)
	assertChange([]int{0, 2})

	// Check setting the charm URL generates all new relation events.
	err = f.SetCharm(s.wpcharm.URL())
	c.Assert(err, gc.IsNil)
	assertChange([]int{0, 2})
}

func (s *FilterSuite) addRelation(c *gc.C) *state.Relation {
	if s.mysqlcharm == nil {
		s.mysqlcharm = s.AddTestingCharm(c, "mysql")
	}
	rels, err := s.wordpress.Relations()
	c.Assert(err, gc.IsNil)
	svcName := fmt.Sprintf("mysql%d", len(rels))
	s.AddTestingService(c, svcName, s.mysqlcharm)
	eps, err := s.State.InferEndpoints([]string{svcName, "wordpress"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	return rel
}
