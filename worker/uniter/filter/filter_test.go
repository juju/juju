// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filter_test

import (
	"fmt"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"
	"launchpad.net/tomb"

	"github.com/juju/juju/api"
	apiuniter "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/uniter/filter"
)

type FilterSuite struct {
	jujutesting.JujuConnSuite
	wordpress  *state.Service
	unit       *state.Unit
	mysqlcharm *state.Charm
	wpcharm    *state.Charm
	machine    *state.Machine

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
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	mid, err := s.unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	s.machine, err = s.State.Machine(mid)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetProvisioned("i-exist", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.APILogin(c, s.unit)
}

func (s *FilterSuite) APILogin(c *gc.C, unit *state.Unit) {
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAs(c, unit.Tag(), password)
	s.uniter, err = s.st.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.uniter, gc.NotNil)
}

func (s *FilterSuite) TestUnitDeath(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer f.Stop() // no AssertStop, we test for an error below
	asserter := coretesting.NotifyAsserterC{
		Precond: func() { s.BackingState.StartSync() },
		C:       c,
		Chan:    f.UnitDying(),
	}
	asserter.AssertNoReceive()

	// Irrelevant change.
	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, jc.ErrorIsNil)
	asserter.AssertNoReceive()

	// Set dying.
	err = s.unit.SetStatus(state.StatusStarted, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	asserter.AssertClosed()

	// Another irrelevant change.
	err = s.unit.ClearResolved()
	c.Assert(err, jc.ErrorIsNil)
	asserter.AssertClosed()

	// Set dead.
	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	s.assertAgentTerminates(c, f)
}

func (s *FilterSuite) TestUnitRemoval(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer f.Stop() // no AssertStop, we test for an error below

	// short-circuit to remove because no status set.
	err = s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertAgentTerminates(c, f)
}

// Ensure we get a signal on f.Dead()
func (s *FilterSuite) assertFilterDies(c *gc.C, f filter.Filter) {
	asserter := coretesting.NotifyAsserterC{
		Precond: func() { s.BackingState.StartSync() },
		C:       c,
		Chan:    f.Dead(),
	}
	asserter.AssertClosed()
}

func (s *FilterSuite) assertAgentTerminates(c *gc.C, f filter.Filter) {
	s.assertFilterDies(c, f)
	c.Assert(f.Wait(), gc.Equals, worker.ErrTerminateAgent)
}

func (s *FilterSuite) TestServiceDeath(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)
	dyingAsserter := coretesting.NotifyAsserterC{
		C:       c,
		Precond: func() { s.BackingState.StartSync() },
		Chan:    f.UnitDying(),
	}
	dyingAsserter.AssertNoReceive()

	err = s.unit.SetStatus(state.StatusStarted, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.wordpress.Destroy()
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.unit.Life(), gc.Equals, state.Dying)

	// Can't set s.wordpress to Dead while it still has units.
}

func (s *FilterSuite) TestResolvedEvents(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
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
	err = s.unit.SetStatus(state.StatusError, "blarg", nil)
	c.Assert(err, jc.ErrorIsNil)
	resolvedAsserter.AssertNoReceive()

	// Change the unit's resolved to an interesting value; new event received.
	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	resolvedAsserter.AssertNoReceive()

	// ...even when requested.
	f.WantResolvedEvent()
	resolvedAsserter.AssertNoReceive()

	// Induce several events; only latest state is reported.
	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, jc.ErrorIsNil)
	err = f.ClearResolved()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, jc.ErrorIsNil)
	assertChange(params.ResolvedNoHooks)
}

func (s *FilterSuite) TestCharmUpgradeEvents(c *gc.C) {
	oldCharm := s.AddTestingCharm(c, "upgrade1")
	svc := s.AddTestingService(c, "upgradetest", oldCharm)
	unit, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)

	s.APILogin(c, unit)

	f, err := filter.NewFilter(s.uniter, unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	assertNoChange()

	// Explicitly request an event relative to the existing state; nothing.
	f.WantUpgradeEvent(false)
	assertNoChange()

	// Change the service in an irrelevant way; no events.
	err = svc.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	assertNoChange()

	// Change the service's charm; new event received.
	newCharm := s.AddTestingCharm(c, "upgrade2")
	err = svc.SetCharm(newCharm, false)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	assertNoChange()

	// ...but a *forced* change to a different URL should generate an event.
	err = svc.SetCharm(newCharm, true)
	assertChange(newCharm.URL())
	assertNoChange()
}

func (s *FilterSuite) TestConfigEvents(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)

	err = s.machine.SetAddresses(network.NewAddress("0.1.2.3", network.ScopeUnknown))
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)
	assertChange := func() {
		s.BackingState.StartSync()
		select {
		case _, ok := <-f.ConfigEvents():
			c.Assert(ok, jc.IsTrue)
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
		c.Assert(err, jc.ErrorIsNil)
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

	// Change the addresses of the unit's assigned machine; new event received.
	err = s.machine.SetAddresses(network.NewAddress("0.1.2.4", network.ScopeUnknown))
	c.Assert(err, jc.ErrorIsNil)
	s.BackingState.StartSync()
	assertChange()

	// Check that a filter's initial event works with DiscardConfigEvent
	// as expected.
	f, err = filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)
	s.BackingState.StartSync()
	f.DiscardConfigEvent()
	assertNoChange()

	// Further changes are still collapsed as appropriate.
	changeConfig("forsooth")
	changeConfig("imagination failure")
	assertChange()
}

func (s *FilterSuite) TestInitialAddressEventIgnored(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)

	// Note: we don't set addresses here because that would
	// race with the filter starting its address watcher.
	// We will always receive a config-changed event when
	// addresses change *after* the filter starts watching
	// addresses.

	// We should not get any config-change events until
	// setting the charm URL.
	s.BackingState.StartSync()
	select {
	case <-f.ConfigEvents():
		c.Fatalf("unexpected config event")
	case <-time.After(coretesting.ShortWait):
	}

	// Set the charm URL to trigger config events.
	err = f.SetCharm(s.wpcharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	// We should get one config-change event only.
	s.BackingState.StartSync()
	select {
	case _, ok := <-f.ConfigEvents():
		c.Assert(ok, jc.IsTrue)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out")
	}
	select {
	case <-f.ConfigEvents():
		c.Fatalf("unexpected config event")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *FilterSuite) TestConfigAndAddressEvents(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)

	// Set the charm URL to trigger config events.
	err = f.SetCharm(s.wpcharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	// Changing the machine addresses should also result in
	// a config-change event.
	err = s.machine.SetAddresses(
		network.NewAddress("0.1.2.3", network.ScopeUnknown),
	)
	c.Assert(err, jc.ErrorIsNil)

	assertChange := func() {
		select {
		case _, ok := <-f.ConfigEvents():
			c.Assert(ok, jc.IsTrue)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out")
		}
	}

	// Config and address events should be coalesced. Start
	// the synchronisation and sleep a bit to give the filter
	// a chance to pick them both up.
	s.BackingState.StartSync()
	time.Sleep(250 * time.Millisecond)
	assertChange()
	select {
	case <-f.ConfigEvents():
		c.Fatalf("unexpected config event")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *FilterSuite) TestConfigAndAddressEventsDiscarded(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)

	// Change the machine addresses.
	err = s.machine.SetAddresses(
		network.NewAddress("0.1.2.3", network.ScopeUnknown),
	)
	c.Assert(err, jc.ErrorIsNil)

	// Set the charm URL to trigger config events.
	err = f.SetCharm(s.wpcharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	// We should not receive any config-change events.
	s.BackingState.StartSync()
	f.DiscardConfigEvent()
	select {
	case <-f.ConfigEvents():
		c.Fatalf("unexpected config event")
	case <-time.After(coretesting.ShortWait):
	}
}

// TestActionEvent helper functions
func getAssertNoActionChange(s *FilterSuite, f filter.Filter, c *gc.C) func() {
	return func() {
		s.BackingState.StartSync()
		select {
		case <-f.ActionEvents():
			c.Fatalf("unexpected action event")
		case <-time.After(coretesting.ShortWait):
		}
	}
}

func getAssertActionChange(s *FilterSuite, f filter.Filter, c *gc.C) func(ids []string) {
	return func(ids []string) {
		s.BackingState.StartSync()
		expected := make(map[string]int)
		seen := make(map[string]int)
		for _, id := range ids {
			expected[id] += 1
			select {
			case event, ok := <-f.ActionEvents():
				c.Assert(ok, jc.IsTrue)
				seen[event.ActionId] += 1
			case <-time.After(coretesting.LongWait):
				c.Fatalf("timed out")
			}
		}
		c.Assert(seen, jc.DeepEquals, expected)

		getAssertNoActionChange(s, f, c)()
	}
}

func getAddAction(s *FilterSuite, c *gc.C) func(name string) string {
	return func(name string) string {
		newAction, err := s.unit.AddAction(name, nil)
		c.Assert(err, jc.ErrorIsNil)
		newId := newAction.Id()
		return newId
	}
}

func (s *FilterSuite) TestActionEvents(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)

	// Get helper functions
	assertNoChange := getAssertNoActionChange(s, f, c)
	assertChange := getAssertActionChange(s, f, c)
	addAction := getAddAction(s, c)

	// Test no changes before Actions are added for the Unit.
	assertNoChange()

	// Add a new action; event occurs
	testId := addAction("snapshot")
	assertChange([]string{testId})

	// Make sure bundled events arrive properly.
	testIds := make([]string, 5)
	for i := 0; i < 5; i++ {
		testIds[i] = addAction("name" + string(i))
	}

	assertChange(testIds)
}

func (s *FilterSuite) TestPreexistingActions(c *gc.C) {
	addAction := getAddAction(s, c)

	// Add an Action before the Filter has been created and see if
	// it arrives properly.

	testId := addAction("snapshot")

	// Now create the Filter and see whether the Action comes in as expected.
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)

	assertNoChange := getAssertNoActionChange(s, f, c)
	assertChange := getAssertActionChange(s, f, c)

	assertChange([]string{testId})

	// Let's make sure there were no duplicates.
	assertNoChange()
}

func (s *FilterSuite) TestCharmErrorEvents(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
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
	f, err = filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer f.Stop() // no AssertStop, we test for an error below

	// Check with a nil charm URL, again no changes.
	err = f.SetCharm(nil)
	c.Assert(err, gc.Equals, tomb.ErrDying)
	assertNoChange()
	s.assertFilterDies(c, f)
}

func (s *FilterSuite) TestRelationsEvents(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	err = ru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = rel0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertChange([]int{0, 2})

	// Remove a relation completely; check no event, because the relation
	// could not have been removed if the unit was in scope, and therefore
	// the uniter never needs to hear about it.
	err = rel1.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertNoChange()
	err = f.Stop()
	c.Assert(err, jc.ErrorIsNil)

	// Start a new filter, check initial event.
	f, err = filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)
	assertChange([]int{0, 2})

	// Check setting the charm URL generates all new relation events.
	err = f.SetCharm(s.wpcharm.URL())
	c.Assert(err, jc.ErrorIsNil)
	assertChange([]int{0, 2})
}

func (s *FilterSuite) addRelation(c *gc.C) *state.Relation {
	if s.mysqlcharm == nil {
		s.mysqlcharm = s.AddTestingCharm(c, "mysql")
	}
	rels, err := s.wordpress.Relations()
	c.Assert(err, jc.ErrorIsNil)
	svcName := fmt.Sprintf("mysql%d", len(rels))
	s.AddTestingService(c, svcName, s.mysqlcharm)
	eps, err := s.State.InferEndpoints(svcName, "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	return rel
}

func (s *FilterSuite) TestMeterStatusEvents(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)
	assertNoChange := func() {
		s.BackingState.StartSync()
		select {
		case <-f.MeterStatusEvents():
			c.Fatalf("unexpected meter status event")
		case <-time.After(coretesting.ShortWait):
		}
	}
	assertChange := func() {
		s.BackingState.StartSync()
		select {
		case _, ok := <-f.MeterStatusEvents():
			c.Assert(ok, jc.IsTrue)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out")
		}
		assertNoChange()
	}
	// Initial meter status does not trigger event.
	assertNoChange()

	// Set unit meter status to trigger event.
	err = s.unit.SetMeterStatus("GREEN", "Operating normally.")
	c.Assert(err, jc.ErrorIsNil)
	assertChange()

	// Make sure bundled events arrive properly.
	for i := 0; i < 5; i++ {
		err = s.unit.SetMeterStatus("RED", fmt.Sprintf("Update %d.", i))
		c.Assert(err, jc.ErrorIsNil)
	}
	assertChange()
}
