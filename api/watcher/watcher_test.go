// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/migrationminion"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	corewatcher "github.com/juju/juju/watcher"
	"github.com/juju/juju/watcher/watchertest"
	"github.com/juju/juju/worker"
)

type watcherSuite struct {
	testing.JujuConnSuite

	stateAPI api.Connection

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.stateAPI, s.rawMachine = s.OpenAPIAsNewMachine(c, state.JobManageModel, state.JobHostUnits)
}

func (s *watcherSuite) TestWatchInitialEventConsumed(c *gc.C) {
	// Machiner.Watch should send the initial event as part of the Watch
	// call (for NotifyWatchers there is no state to be transmitted). So a
	// call to Next() should not have anything to return.
	var results params.NotifyWatchResults
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}}}
	err := s.stateAPI.APICall("Machiner", s.stateAPI.BestFacadeVersion("Machiner"), "", "Watch", args, &results)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)

	// We expect the Call() to "Next" to block, so run it in a goroutine.
	done := make(chan error)
	go func() {
		ignored := struct{}{}
		done <- s.stateAPI.APICall("NotifyWatcher", s.stateAPI.BestFacadeVersion("NotifyWatcher"), result.NotifyWatcherId, "Next", nil, &ignored)
	}()

	select {
	case err := <-done:
		c.Errorf("Call(Next) did not block immediately after Watch(): err %v", err)
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *watcherSuite) TestWatchMachine(c *gc.C) {
	var results params.NotifyWatchResults
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}}}
	err := s.stateAPI.APICall("Machiner", s.stateAPI.BestFacadeVersion("Machiner"), "", "Watch", args, &results)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)

	w := watcher.NewNotifyWatcher(s.stateAPI, result)
	wc := watchertest.NewNotifyWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()
	wc.AssertOneChange()
}

func (s *watcherSuite) TestNotifyWatcherStopsWithPendingSend(c *gc.C) {
	var results params.NotifyWatchResults
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}}}
	err := s.stateAPI.APICall("Machiner", s.stateAPI.BestFacadeVersion("Machiner"), "", "Watch", args, &results)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)

	// params.NotifyWatcher conforms to the watcher.NotifyWatcher interface
	w := watcher.NewNotifyWatcher(s.stateAPI, result)
	wc := watchertest.NewNotifyWatcherC(c, w, s.BackingState.StartSync)
	wc.AssertStops()
}

func (s *watcherSuite) TestWatchUnitsKeepsEvents(c *gc.C) {
	// Create two services, relate them, and add one unit to each - a
	// principal and a subordinate.
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("mysql", "logging")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	principal, err := mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = principal.AssignToMachine(s.rawMachine)
	c.Assert(err, jc.ErrorIsNil)
	relUnit, err := rel.Unit(principal)
	c.Assert(err, jc.ErrorIsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	subordinate, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)

	// Call the Deployer facade's WatchUnits for machine-0.
	var results params.StringsWatchResults
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}}}
	err = s.stateAPI.APICall("Deployer", s.stateAPI.BestFacadeVersion("Deployer"), "", "WatchUnits", args, &results)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)

	// Start a StringsWatcher and check the initial event.
	w := watcher.NewStringsWatcher(s.stateAPI, result)
	wc := watchertest.NewStringsWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	wc.AssertChange("mysql/0", "logging/0")
	wc.AssertNoChange()

	// Now, without reading any changes advance the lifecycle of both
	// units, inducing an update server-side after each two changes to
	// ensure they're reported as separate events over the API.
	err = subordinate.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	s.BackingState.StartSync()
	err = subordinate.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = principal.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	s.BackingState.StartSync()

	// Expect these changes as 2 separate events, so that
	// nothing gets lost.
	wc.AssertChange("logging/0")
	wc.AssertChange("mysql/0")
	wc.AssertNoChange()
}

func (s *watcherSuite) TestStringsWatcherStopsWithPendingSend(c *gc.C) {
	// Call the Deployer facade's WatchUnits for machine-0.
	var results params.StringsWatchResults
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}}}
	err := s.stateAPI.APICall("Deployer", s.stateAPI.BestFacadeVersion("Deployer"), "", "WatchUnits", args, &results)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)

	// Start a StringsWatcher and check the initial event.
	w := watcher.NewStringsWatcher(s.stateAPI, result)
	wc := watchertest.NewStringsWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	// Create a service, deploy a unit of it on the machine.
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	principal, err := mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = principal.AssignToMachine(s.rawMachine)
	c.Assert(err, jc.ErrorIsNil)
}

// TODO(fwereade): 2015-11-18 lp:1517391
func (s *watcherSuite) TestWatchMachineStorage(c *gc.C) {
	f := factory.NewFactory(s.BackingState)
	f.MakeMachine(c, &factory.MachineParams{
		Volumes: []state.MachineVolumeParams{{
			Volume: state.VolumeParams{
				Pool: "environscoped",
				Size: 1024,
			},
		}},
	})

	var results params.MachineStorageIdsWatchResults
	args := params.Entities{Entities: []params.Entity{{
		Tag: s.State.ModelTag().String(),
	}}}
	err := s.stateAPI.APICall(
		"StorageProvisioner",
		s.stateAPI.BestFacadeVersion("StorageProvisioner"),
		"", "WatchVolumeAttachments", args, &results)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)

	w := watcher.NewVolumeAttachmentsWatcher(s.stateAPI, result)
	defer func() {

		// Check we can stop the watcher...
		w.Kill()
		wait := make(chan error)
		go func() {
			wait <- w.Wait()
		}()
		select {
		case err := <-wait:
			c.Assert(err, jc.ErrorIsNil)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("watcher never stopped")
		}

		// ...and that its channel hasn't been closed.
		s.BackingState.StartSync()
		select {
		case change, ok := <-w.Changes():
			c.Fatalf("watcher sent unexpected change: (%#v, %v)", change, ok)
		default:
		}

	}()

	// Check initial event;
	s.BackingState.StartSync()
	select {
	case changes, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(changes, jc.SameContents, []corewatcher.MachineStorageId{{
			MachineTag:    "machine-1",
			AttachmentTag: "volume-0",
		}})
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	// check no subsequent event.
	s.BackingState.StartSync()
	select {
	case <-w.Changes():
		c.Fatalf("received unexpected change")
	case <-time.After(coretesting.ShortWait):
	}
}

type migrationSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&migrationSuite{})

func (s *migrationSuite) startSync(c *gc.C, st *state.State) {
	backingSt, err := s.BackingStatePool.Get(st.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	backingSt.StartSync()
}

func (s *migrationSuite) TestMigrationStatusWatcher(c *gc.C) {
	const nonce = "noncey"

	// Create a model to migrate.
	hostedState := s.Factory.MakeModel(c, &factory.ModelParams{})
	defer hostedState.Close()
	hostedFactory := factory.NewFactory(hostedState)

	// Create a machine in the hosted model to connect as.
	m, password := hostedFactory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: nonce,
	})

	// Connect as the machine to watch for migration status.
	apiInfo := s.APIInfo(c)
	apiInfo.Tag = m.Tag()
	apiInfo.Password = password
	apiInfo.ModelTag = hostedState.ModelTag()
	apiInfo.Nonce = nonce

	apiConn, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	defer apiConn.Close()

	// Start watching for a migration.
	client := migrationminion.NewClient(apiConn)
	w, err := client.Watch()
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		c.Assert(worker.Stop(w), jc.ErrorIsNil)
	}()

	assertNoChange := func() {
		s.startSync(c, hostedState)
		select {
		case _, ok := <-w.Changes():
			c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
		case <-time.After(coretesting.ShortWait):
		}
	}

	assertChange := func(id string, phase migration.Phase) {
		s.startSync(c, hostedState)
		select {
		case status, ok := <-w.Changes():
			c.Assert(ok, jc.IsTrue)
			c.Check(status.MigrationId, gc.Equals, id)
			c.Check(status.Phase, gc.Equals, phase)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("watcher didn't emit an event")
		}
		assertNoChange()
	}

	// Initial event with no migration in progress.
	assertChange("", migration.NONE)

	// Now create a migration, should trigger watcher.
	spec := state.MigrationSpec{
		InitiatedBy: names.NewUserTag("someone"),
		TargetInfo: migration.TargetInfo{
			ControllerTag: names.NewControllerTag(utils.MustNewUUID().String()),
			Addrs:         []string{"1.2.3.4:5"},
			CACert:        "cert",
			AuthTag:       names.NewUserTag("dog"),
			Password:      "sekret",
		},
	}
	mig, err := hostedState.CreateMigration(spec)
	c.Assert(err, jc.ErrorIsNil)
	assertChange(mig.Id(), migration.QUIESCE)

	// Now abort the migration, this should be reported too.
	c.Assert(mig.SetPhase(migration.ABORT), jc.ErrorIsNil)
	assertChange(mig.Id(), migration.ABORT)
	c.Assert(mig.SetPhase(migration.ABORTDONE), jc.ErrorIsNil)
	assertChange(mig.Id(), migration.ABORTDONE)

	// Start a new migration, this should also trigger.
	mig2, err := hostedState.CreateMigration(spec)
	c.Assert(err, jc.ErrorIsNil)
	assertChange(mig2.Id(), migration.QUIESCE)
}
