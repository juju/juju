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
	"gopkg.in/juju/charm.v5"
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

	st     api.Connection
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

// notifyAsserterC creates a coretesting.NotifyAsserterC that will sync the
// state before running our assertions.
func (s *FilterSuite) notifyAsserterC(c *gc.C, ch <-chan struct{}) coretesting.NotifyAsserterC {
	return coretesting.NotifyAsserterC{
		Precond: s.BackingState.StartSync,
		C:       c,
		Chan:    ch,
	}
}

// contentAsserterC creates a coretesting.ContentAsserterC that will sync the
// state before running our assertions.
func (s *FilterSuite) contentAsserterC(c *gc.C, ch interface{}) coretesting.ContentAsserterC {
	return coretesting.ContentAsserterC{
		Precond: s.BackingState.StartSync,
		C:       c,
		Chan:    ch,
	}
}

// EvilSync starts a state sync (ensuring that any changes will be delivered to
// the internal watchers "soon") -- and then waits "a while" so that we can be
// reasonably certain that the events have made it through the api server and
// then delivered from the api-level watcher to the filter itself.
//
// It's important to be clear that this *is* evil, and we should be testing
// with a mocked-out watcher we can control directly; the only reason this
// method exists is because we already perpetrated this crime -- but not
// consistently -- and we're concentrating the evil in one place.
func (s *FilterSuite) EvilSync() {
	s.BackingState.StartSync()
	time.Sleep(250 * time.Millisecond)
}

func (s *FilterSuite) TestUnitDeath(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer f.Stop() // no AssertStop, we test for an error below
	dyingC := s.notifyAsserterC(c, f.UnitDying())
	dyingC.AssertNoReceive()

	// Irrelevant change.
	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, jc.ErrorIsNil)
	dyingC.AssertNoReceive()

	// Set dying.
	err = s.unit.SetAgentStatus(state.StatusIdle, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	dyingC.AssertClosed()

	// Another irrelevant change.
	err = s.unit.ClearResolved()
	c.Assert(err, jc.ErrorIsNil)
	dyingC.AssertClosed()

	// Set dead.
	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	s.assertAgentTerminates(c, f)
}

func (s *FilterSuite) TestUnitRemoval(c *gc.C) {
	coretesting.SkipIfI386(c, "lp:1425569")

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
	deadC := s.notifyAsserterC(c, f.Dead())
	deadC.AssertClosed()
}

func (s *FilterSuite) assertAgentTerminates(c *gc.C, f filter.Filter) {
	s.assertFilterDies(c, f)
	c.Assert(f.Wait(), gc.Equals, worker.ErrTerminateAgent)
}

func (s *FilterSuite) TestServiceDeath(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)
	dyingC := s.notifyAsserterC(c, f.UnitDying())
	dyingC.AssertNoReceive()

	err = s.unit.SetAgentStatus(state.StatusIdle, "", nil)
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

	resolvedC := s.contentAsserterC(c, f.ResolvedEvents())
	resolvedC.AssertNoReceive()

	// Request an event; no interesting event is available.
	f.WantResolvedEvent()
	resolvedC.AssertNoReceive()

	// Change the unit in an irrelevant way; no events.
	err = s.unit.SetAgentStatus(state.StatusError, "blarg", nil)
	c.Assert(err, jc.ErrorIsNil)
	resolvedC.AssertNoReceive()

	// Change the unit's resolved to an interesting value; new event received.
	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, jc.ErrorIsNil)
	resolvedC.AssertOneValue(params.ResolvedRetryHooks)

	// Ask for the event again, and check it's resent.
	f.WantResolvedEvent()
	resolvedC.AssertOneValue(params.ResolvedRetryHooks)

	// Clear the resolved status *via the filter*; check not resent...
	err = f.ClearResolved()
	c.Assert(err, jc.ErrorIsNil)
	resolvedC.AssertNoReceive()

	// ...even when requested.
	f.WantResolvedEvent()
	resolvedC.AssertNoReceive()

	// Induce several events; only latest state is reported.
	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, jc.ErrorIsNil)
	err = f.ClearResolved()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, jc.ErrorIsNil)
	resolvedC.AssertOneValue(params.ResolvedNoHooks)
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
	upgradeC := s.contentAsserterC(c, f.UpgradeEvents())
	upgradeC.AssertNoReceive()

	// Setting a charm generates no new events if it already matches.
	err = f.SetCharm(oldCharm.URL())
	c.Assert(err, jc.ErrorIsNil)
	upgradeC.AssertNoReceive()

	// Explicitly request an event relative to the existing state; nothing.
	f.WantUpgradeEvent(false)
	upgradeC.AssertNoReceive()

	// Change the service in an irrelevant way; no events.
	err = svc.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	upgradeC.AssertNoReceive()

	// Change the service's charm; new event received.
	newCharm := s.AddTestingCharm(c, "upgrade2")
	err = svc.SetCharm(newCharm, false)
	c.Assert(err, jc.ErrorIsNil)
	upgradeC.AssertOneValue(newCharm.URL())

	// Request a new upgrade *unforced* upgrade event, we should see one.
	f.WantUpgradeEvent(false)
	upgradeC.AssertOneValue(newCharm.URL())

	// Request only *forced* upgrade events; nothing.
	f.WantUpgradeEvent(true)
	upgradeC.AssertNoReceive()

	// But when we have a forced upgrade to the same URL, no new event.
	err = svc.SetCharm(oldCharm, true)
	c.Assert(err, jc.ErrorIsNil)
	upgradeC.AssertNoReceive()

	// ...but a *forced* change to a different URL should generate an event.
	err = svc.SetCharm(newCharm, true)
	c.Assert(err, jc.ErrorIsNil)
	upgradeC.AssertOneValue(newCharm.URL())
}

func (s *FilterSuite) TestConfigEvents(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)

	err = s.machine.SetProviderAddresses(network.NewAddress("0.1.2.3"))
	c.Assert(err, jc.ErrorIsNil)

	// Test no changes before the charm URL is set.
	configC := s.notifyAsserterC(c, f.ConfigEvents())
	configC.AssertNoReceive()

	// Set the charm URL to trigger config events.
	err = f.SetCharm(s.wpcharm.URL())
	c.Assert(err, jc.ErrorIsNil)
	s.EvilSync()
	configC.AssertOneReceive()

	// Change the config; new event received.
	changeConfig := func(title interface{}) {
		err := s.wordpress.UpdateConfigSettings(charm.Settings{
			"blog-title": title,
		})
		c.Assert(err, jc.ErrorIsNil)
	}
	changeConfig("20,000 leagues in the cloud")
	configC.AssertOneReceive()

	// Change the config a few more times, then reset the events. We sync to
	// make sure the events have arrived in the watcher -- and then wait a
	// little longer, to allow for the delay while the events are coalesced
	// -- before we tell it to discard all received events. This would be
	// much better tested by controlling a mocked-out watcher directly, but
	// that's a bit inconvenient for this change.
	changeConfig(nil)
	changeConfig("the curious incident of the dog in the cloud")
	s.EvilSync()
	f.DiscardConfigEvent()
	configC.AssertNoReceive()

	// Change the addresses of the unit's assigned machine; new event received.
	err = s.machine.SetProviderAddresses(network.NewAddress("0.1.2.4"))
	c.Assert(err, jc.ErrorIsNil)
	s.BackingState.StartSync()
	configC.AssertOneReceive()

	// Check that a filter's initial event works with DiscardConfigEvent
	// as expected.
	f, err = filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)
	s.BackingState.StartSync()
	f.DiscardConfigEvent()
	configC.AssertNoReceive()

	// Further changes are still collapsed as appropriate.
	changeConfig("forsooth")
	changeConfig("imagination failure")
	configC.AssertOneReceive()
}

func (s *FilterSuite) TestInitialAddressEventIgnored(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)

	err = s.machine.SetProviderAddresses(network.NewAddress("0.1.2.3"))
	c.Assert(err, jc.ErrorIsNil)

	// We should not get any config-change events until
	// setting the charm URL.
	configC := s.notifyAsserterC(c, f.ConfigEvents())
	configC.AssertNoReceive()

	// Set the charm URL to trigger config events.
	err = f.SetCharm(s.wpcharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	// We should get one config-change event only.
	s.EvilSync()
	configC.AssertOneReceive()
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
	err = s.machine.SetProviderAddresses(
		network.NewAddress("0.1.2.3"),
	)
	c.Assert(err, jc.ErrorIsNil)

	configC := s.notifyAsserterC(c, f.ConfigEvents())

	// Config and address events should be coalesced. Start
	// the synchronisation and sleep a bit to give the filter
	// a chance to pick them both up.
	s.EvilSync()
	configC.AssertOneReceive()
}

func (s *FilterSuite) TestConfigAndAddressEventsDiscarded(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)

	// There should be no pending changes yet
	configC := s.notifyAsserterC(c, f.ConfigEvents())
	configC.AssertNoReceive()

	// Change the machine addresses.
	err = s.machine.SetProviderAddresses(network.NewAddress("0.1.2.3"))
	c.Assert(err, jc.ErrorIsNil)

	// Set the charm URL to trigger config events.
	err = f.SetCharm(s.wpcharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	// We should not receive any config-change events.
	s.EvilSync()
	f.DiscardConfigEvent()
	configC.AssertNoReceive()
}

func getAssertActionChange(actionC coretesting.ContentAsserterC) func(ids []string) {
	// This calls AssertReceive N times for N ids, but allows the
	// ids to come back in any order.
	return func(ids []string) {
		expected := make(map[string]int)
		seen := make(map[string]int)
		for _, id := range ids {
			expected[id] += 1
			actionId := actionC.AssertReceive().(string)
			seen[actionId] += 1
		}
		actionC.C.Assert(seen, jc.DeepEquals, expected)

		// Ensure that there are no other items remaining
		actionC.AssertNoReceive()
	}
}

func getAddAction(s *FilterSuite, c *gc.C) func(name string) string {
	return func(name string) string {
		newAction, err := s.State.EnqueueAction(s.unit.Tag(), name, nil)
		// newAction, err := s.unit.AddAction(name, nil)
		c.Assert(err, jc.ErrorIsNil)
		newId := newAction.Id()
		return newId
	}
}

func (s *FilterSuite) TestActionEvents(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)

	actionC := s.contentAsserterC(c, f.ActionEvents())
	addAction := getAddAction(s, c)
	assertChange := getAssertActionChange(actionC)

	// Test no changes before Actions are added for the Unit.
	actionC.AssertNoReceive()

	// Add a new action; event occurs
	testId := addAction("fakeaction")
	assertChange([]string{testId})

	// Make sure bundled events arrive properly.
	testIds := make([]string, 5)
	for i := 0; i < 5; i++ {
		testIds[i] = addAction("fakeaction")
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

	actionC := s.contentAsserterC(c, f.ActionEvents())
	assertChange := getAssertActionChange(actionC)
	assertChange([]string{testId})

	// Let's make sure there were no duplicates.
	actionC.AssertNoReceive()
}

func (s *FilterSuite) TestCharmErrorEvents(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer f.Stop() // no AssertStop, we test for an error below

	configC := s.notifyAsserterC(c, f.ConfigEvents())

	// Check setting an invalid charm URL does not send events.
	err = f.SetCharm(charm.MustParseURL("cs:missing/one-1"))
	c.Assert(err, gc.Equals, tomb.ErrDying)
	configC.AssertNoReceive()
	s.assertFilterDies(c, f)

	// Filter died after the error, so restart it.
	f, err = filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer f.Stop() // no AssertStop, we test for an error below

	// Check with a nil charm URL, again no changes.
	err = f.SetCharm(nil)
	c.Assert(err, gc.Equals, tomb.ErrDying)
	configC.AssertNoReceive()
	s.assertFilterDies(c, f)
}

func (s *FilterSuite) TestRelationsEvents(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)

	relationsC := s.contentAsserterC(c, f.RelationsEvents())
	relationsC.AssertNoReceive()

	// Add a couple of relations; check the event.
	rel0 := s.addRelation(c)
	rel1 := s.addRelation(c)
	c.Assert(relationsC.AssertOneReceive(), gc.DeepEquals, []int{0, 1})

	// Add another relation, and change another's Life (by entering scope before
	// Destroy, thereby setting the relation to Dying); check event.
	s.addRelation(c)
	ru0, err := rel0.Unit(s.unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = rel0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relationsC.AssertOneReceive(), gc.DeepEquals, []int{0, 2})

	// Remove a relation completely; check no event, because the relation
	// could not have been removed if the unit was in scope, and therefore
	// the uniter never needs to hear about it.
	err = rel1.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	relationsC.AssertNoReceive()
	err = f.Stop()
	c.Assert(err, jc.ErrorIsNil)

	// Start a new filter, check initial event.
	f, err = filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)
	relationsC = s.contentAsserterC(c, f.RelationsEvents())
	c.Assert(relationsC.AssertOneReceive(), gc.DeepEquals, []int{0, 2})

	// Check setting the charm URL generates all new relation events.
	err = f.SetCharm(s.wpcharm.URL())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relationsC.AssertOneReceive(), gc.DeepEquals, []int{0, 2})
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
	meterC := s.notifyAsserterC(c, f.MeterStatusEvents())
	// Initial meter status does not trigger event.
	meterC.AssertNoReceive()

	// Set unit meter status to trigger event.
	err = s.unit.SetMeterStatus("GREEN", "Operating normally.")
	c.Assert(err, jc.ErrorIsNil)
	meterC.AssertOneReceive()

	// Make sure bundled events arrive properly.
	for i := 0; i < 5; i++ {
		err = s.unit.SetMeterStatus("RED", fmt.Sprintf("Update %d.", i))
		c.Assert(err, jc.ErrorIsNil)
	}
	meterC.AssertOneReceive()
}

func (s *FilterSuite) TestStorageEvents(c *gc.C) {
	storageCharm := s.AddTestingCharm(c, "storage-block2")
	svc := s.AddTestingServiceWithStorage(c, "storage-block2", storageCharm, map[string]state.StorageConstraints{
		"multi1to10": state.StorageConstraints{Pool: "loop", Size: 1024, Count: 1},
		"multi2up":   state.StorageConstraints{Pool: "loop", Size: 2048, Count: 2},
	})
	unit, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	s.APILogin(c, unit)

	f, err := filter.NewFilter(s.uniter, unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)
	storageC := s.contentAsserterC(c, f.StorageEvents())
	c.Assert(storageC.AssertOneReceive(), gc.DeepEquals, []names.StorageTag{
		names.NewStorageTag("multi1to10/0"),
		names.NewStorageTag("multi2up/1"),
		names.NewStorageTag("multi2up/2"),
	})

	err = s.State.DestroyStorageInstance(names.NewStorageTag("multi2up/1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageC.AssertOneReceive(), gc.DeepEquals, []names.StorageTag{
		names.NewStorageTag("multi2up/1"),
	})
}

func (s *FilterSuite) setLeaderSetting(c *gc.C, key, value string) {
	err := s.wordpress.UpdateLeaderSettings(successToken{}, map[string]string{key: value})
	c.Assert(err, jc.ErrorIsNil)
}

type successToken struct{}

func (successToken) Check(interface{}) error {
	return nil
}

func (s *FilterSuite) TestLeaderSettingsEventsSendsChanges(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)

	leaderSettingsC := s.notifyAsserterC(c, f.LeaderSettingsEvents())
	// Assert that we get the initial event
	leaderSettingsC.AssertOneReceive()

	// And any time we make changes to the leader settings, we get an event
	s.setLeaderSetting(c, "foo", "bar-1")
	leaderSettingsC.AssertOneReceive()

	// And multiple changes to settings still get collapsed into a single event
	s.setLeaderSetting(c, "foo", "bar-2")
	s.setLeaderSetting(c, "foo", "bar-3")
	s.setLeaderSetting(c, "foo", "bar-4")
	s.EvilSync()
	leaderSettingsC.AssertOneReceive()
}

func (s *FilterSuite) TestWantLeaderSettingsEvents(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)

	leaderSettingsC := s.notifyAsserterC(c, f.LeaderSettingsEvents())

	// Supress the initial event
	f.WantLeaderSettingsEvents(false)
	leaderSettingsC.AssertNoReceive()

	// Also suppresses actual changes
	s.setLeaderSetting(c, "foo", "baz-1")
	s.EvilSync()
	leaderSettingsC.AssertNoReceive()

	// Reenabling the settings gives us an immediate change
	f.WantLeaderSettingsEvents(true)
	leaderSettingsC.AssertOneReceive()

	// And also gives changes when actual changes are made
	s.setLeaderSetting(c, "foo", "baz-2")
	s.EvilSync()
	leaderSettingsC.AssertOneReceive()

	// Setting a value to the same thing doesn't trigger a change
	s.setLeaderSetting(c, "foo", "baz-2")
	s.EvilSync()
	leaderSettingsC.AssertNoReceive()

}

func (s *FilterSuite) TestDiscardLeaderSettingsEvent(c *gc.C) {
	f, err := filter.NewFilter(s.uniter, s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, f)

	leaderSettingsC := s.notifyAsserterC(c, f.LeaderSettingsEvents())
	// Discard the initial event
	f.DiscardLeaderSettingsEvent()
	leaderSettingsC.AssertNoReceive()

	// However, it has not permanently disabled change events, another
	// change still shows up
	s.setLeaderSetting(c, "foo", "bing-1")
	s.EvilSync()
	leaderSettingsC.AssertOneReceive()

	// But at any point we can discard them
	s.setLeaderSetting(c, "foo", "bing-2")
	s.EvilSync()
	f.DiscardLeaderSettingsEvent()
	leaderSettingsC.AssertNoReceive()
}
