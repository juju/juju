// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/checkers"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/crossmodelrelations"
	"github.com/juju/juju/api/migrationminion"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/status"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
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
	// Create two applications, relate them, and add one unit to each - a
	// principal and a subordinate.
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("mysql", "logging")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	principal, err := mysql.AddUnit(state.AddUnitParams{})
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
	// units.
	err = subordinate.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = subordinate.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = principal.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// Expect both changes are passed back.
	wc.AssertChange("mysql/0", "logging/0")
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

	// Create an application, deploy a unit of it on the machine.
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	principal, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = principal.AssignToMachine(s.rawMachine)
	c.Assert(err, jc.ErrorIsNil)
}

// TODO(fwereade): 2015-11-18 lp:1517391
func (s *watcherSuite) TestWatchMachineStorage(c *gc.C) {
	s.Factory.MakeMachine(c, &factory.MachineParams{
		Volumes: []state.HostVolumeParams{{
			Volume: state.VolumeParams{
				Pool: "modelscoped",
				Size: 1024,
			},
		}},
	})

	var results params.MachineStorageIdsWatchResults
	args := params.Entities{Entities: []params.Entity{{
		Tag: s.Model.ModelTag().String(),
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

func (s *watcherSuite) assertSetupRelationStatusWatch(
	c *gc.C, rel *state.Relation,
) (func(life life.Value, suspended bool, reason string), func()) {
	// Export the relation so it can be found with a token.
	re := s.State.RemoteEntities()
	token, err := re.ExportLocalEntity(rel.Tag())
	c.Assert(err, jc.ErrorIsNil)

	// Create the offer connection details.
	s.Factory.MakeUser(c, &factory.UserParams{Name: "fred"})
	offers := state.NewApplicationOffers(s.State)
	offer, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Owner:           "admin",
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
		OfferUUID:       offer.OfferUUID,
		Username:        "fred",
		RelationKey:     rel.String(),
		RelationId:      rel.Id(),
		SourceModelUUID: s.State.ModelUUID(),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Add the consume permission for the offer so the macaroon
	// discharge can occur.
	err = s.State.CreateOfferAccess(
		names.NewApplicationOfferTag("hosted-mysql"),
		names.NewUserTag("fred"), permission.ConsumeAccess)
	c.Assert(err, jc.ErrorIsNil)

	// Create a macaroon for authorisation.
	store, err := s.State.NewBakeryStorage()
	c.Assert(err, jc.ErrorIsNil)
	bakery, err := bakery.NewService(bakery.NewServiceParams{
		Location: "juju model " + s.State.ModelUUID(),
		Store:    store,
	})
	c.Assert(err, jc.ErrorIsNil)
	mac, err := bakery.NewMacaroon(
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.State.ModelUUID()),
			checkers.DeclaredCaveat("relation-key", rel.String()),
			checkers.DeclaredCaveat("username", "fred"),
		})
	c.Assert(err, jc.ErrorIsNil)

	// Start watching for a relation change.
	client := crossmodelrelations.NewClient(s.stateAPI)
	w, err := client.WatchRelationSuspendedStatus(params.RemoteEntityArg{
		Token:     token,
		Macaroons: macaroon.Slice{mac},
	})
	c.Assert(err, jc.ErrorIsNil)
	stop := func() {
		workertest.CleanKill(c, w)
	}
	modelUUID := s.BackingState.ModelUUID()
	assertNoChange := func() {
		s.WaitForModelWatchersIdle(c, modelUUID)
		select {
		case _, ok := <-w.Changes():
			c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
		case <-time.After(coretesting.ShortWait):
		}
	}

	assertChange := func(life life.Value, suspended bool, reason string) {
		s.WaitForModelWatchersIdle(c, modelUUID)
		select {
		case changes, ok := <-w.Changes():
			c.Check(ok, jc.IsTrue)
			c.Check(changes, gc.HasLen, 1)
			c.Check(changes[0].Life, gc.Equals, life)
			c.Check(changes[0].Suspended, gc.Equals, suspended)
			c.Check(changes[0].SuspendedReason, gc.Equals, reason)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("watcher didn't emit an event")
		}
		assertNoChange()
	}

	// Initial event.
	assertChange(life.Alive, false, "")
	return assertChange, stop
}

func (s *watcherSuite) TestRelationStatusWatcher(c *gc.C) {
	// Create a pair of services and a relation between them.
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	u, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	m := s.Factory.MakeMachine(c, &factory.MachineParams{})
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	relUnit, err := rel.Unit(u)
	c.Assert(err, jc.ErrorIsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	assertChange, stop := s.assertSetupRelationStatusWatch(c, rel)
	defer stop()

	err = rel.SetSuspended(true, "another reason")
	c.Assert(err, jc.ErrorIsNil)
	assertChange(life.Alive, true, "another reason")

	err = rel.SetSuspended(false, "")
	c.Assert(err, jc.ErrorIsNil)
	assertChange(life.Alive, false, "")

	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertChange(life.Dying, false, "")
}

func (s *watcherSuite) TestRelationStatusWatcherDeadRelation(c *gc.C) {
	// Create a pair of services and a relation between them.
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	assertChange, stop := s.assertSetupRelationStatusWatch(c, rel)
	defer stop()

	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertChange(life.Dead, false, "")
}

func (s *watcherSuite) setupOfferStatusWatch(
	c *gc.C,
) (func(status status.Status, message string), func()) {
	// Create the offer connection details.
	s.Factory.MakeUser(c, &factory.UserParams{Name: "fred"})
	offers := state.NewApplicationOffers(s.State)
	offer, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Owner:           "admin",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Add the consume permission for the offer so the macaroon
	// discharge can occur.
	err = s.State.CreateOfferAccess(
		names.NewApplicationOfferTag("hosted-mysql"),
		names.NewUserTag("fred"), permission.ConsumeAccess)
	c.Assert(err, jc.ErrorIsNil)

	// Create a macaroon for authorisation.
	store, err := s.State.NewBakeryStorage()
	c.Assert(err, jc.ErrorIsNil)
	bakery, err := bakery.NewService(bakery.NewServiceParams{
		Location: "juju model " + s.State.ModelUUID(),
		Store:    store,
	})
	c.Assert(err, jc.ErrorIsNil)
	mac, err := bakery.NewMacaroon(
		[]checkers.Caveat{
			checkers.DeclaredCaveat("source-model-uuid", s.State.ModelUUID()),
			checkers.DeclaredCaveat("offer-uuid", offer.OfferUUID),
			checkers.DeclaredCaveat("username", "fred"),
		})
	c.Assert(err, jc.ErrorIsNil)

	// Start watching for a relation change.
	client := crossmodelrelations.NewClient(s.stateAPI)
	w, err := client.WatchOfferStatus(params.OfferArg{
		OfferUUID: offer.OfferUUID,
		Macaroons: macaroon.Slice{mac},
	})
	c.Assert(err, jc.ErrorIsNil)
	stop := func() {
		workertest.CleanKill(c, w)
	}

	assertNoChange := func() {
		s.BackingState.StartSync()
		select {
		case _, ok := <-w.Changes():
			c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
		case <-time.After(coretesting.ShortWait):
		}
	}

	assertChange := func(status status.Status, message string) {
		s.BackingState.StartSync()
		select {
		case changes, ok := <-w.Changes():
			c.Check(ok, jc.IsTrue)
			if status == "" {
				c.Assert(changes, gc.HasLen, 0)
				break
			}
			c.Assert(changes, gc.HasLen, 1)
			c.Check(changes[0].Name, gc.Equals, "hosted-mysql")
			c.Check(changes[0].Status.Status, gc.Equals, status)
			c.Check(changes[0].Status.Message, gc.Equals, message)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("watcher didn't emit an event")
		}
		assertNoChange()
	}

	// Initial event.
	assertChange(status.Waiting, "waiting for machine")
	return assertChange, stop
}

func (s *watcherSuite) TestOfferStatusWatcher(c *gc.C) {
	// Create a pair of services and a relation between them.
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))

	assertChange, stop := s.setupOfferStatusWatch(c)
	defer stop()

	err := mysql.SetStatus(status.StatusInfo{Status: status.Waiting, Message: "another message"})
	c.Assert(err, jc.ErrorIsNil)
	assertChange(status.Waiting, "another message")

	// Deleting the offer results in an empty change set.
	offers := state.NewApplicationOffers(s.State)
	err = offers.Remove("hosted-mysql", false)
	c.Assert(err, jc.ErrorIsNil)
	err = mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertChange("", "")
}

type migrationSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&migrationSuite{})

func (s *migrationSuite) startSync(c *gc.C, st *state.State) {
	backingSt, err := s.BackingStatePool.Get(st.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	backingSt.StartSync()
	backingSt.Release()
}

func (s *migrationSuite) TestMigrationStatusWatcher(c *gc.C) {
	const nonce = "noncey"

	// Create a model to migrate.
	hostedState := s.Factory.MakeModel(c, &factory.ModelParams{})
	defer hostedState.Close()
	hostedFactory := factory.NewFactory(hostedState, s.StatePool)

	// Create a machine in the hosted model to connect as.
	m, password := hostedFactory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: nonce,
	})

	// Connect as the machine to watch for migration status.
	apiInfo := s.APIInfo(c)
	apiInfo.Tag = m.Tag()
	apiInfo.Password = password

	hostedModel, err := hostedState.Model()
	c.Assert(err, jc.ErrorIsNil)

	apiInfo.ModelTag = hostedModel.ModelTag()
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
