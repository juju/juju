// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	stdtesting "testing"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/storage/provider/registry"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

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
	s.stateAPI, s.rawMachine = s.OpenAPIAsNewMachine(c, state.JobManageEnviron, state.JobHostUnits)
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

	// params.NotifyWatcher conforms to the state.NotifyWatcher interface
	w := watcher.NewNotifyWatcher(s.stateAPI, result)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *watcherSuite) TestNotifyWatcherStopsWithPendingSend(c *gc.C) {
	var results params.NotifyWatchResults
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}}}
	err := s.stateAPI.APICall("Machiner", s.stateAPI.BestFacadeVersion("Machiner"), "", "Watch", args, &results)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)

	// params.NotifyWatcher conforms to the state.NotifyWatcher interface
	w := watcher.NewNotifyWatcher(s.stateAPI, result)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)

	// Now, without reading any changes try stopping the watcher.
	statetesting.AssertCanStopWhenSending(c, w)
	wc.AssertClosed()
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
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
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

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
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
	wc := statetesting.NewStringsWatcherC(c, s.State, w)

	// Create a service, deploy a unit of it on the machine.
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	principal, err := mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = principal.AssignToMachine(s.rawMachine)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the initial event is delivered. Then test the watcher
	// can be stopped cleanly without reading the pending change.
	s.BackingState.StartSync()
	statetesting.AssertCanStopWhenSending(c, w)
	wc.AssertClosed()
}

func (s *watcherSuite) TestWatchMachineStorage(c *gc.C) {
	registry.RegisterProvider(
		"envscoped",
		&dummy.StorageProvider{
			StorageScope: storage.ScopeEnviron,
		},
	)
	registry.RegisterEnvironStorageProviders("dummy", "envscoped")
	defer registry.RegisterProvider("envscoped", nil)

	f := factory.NewFactory(s.BackingState)
	f.MakeMachine(c, &factory.MachineParams{
		Volumes: []state.MachineVolumeParams{{
			Volume: state.VolumeParams{
				Pool: "envscoped",
				Size: 1024,
			},
		}},
	})

	var results params.MachineStorageIdsWatchResults
	args := params.Entities{Entities: []params.Entity{{
		Tag: s.State.EnvironTag().String(),
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
	select {
	case changes, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(changes, jc.SameContents, []params.MachineStorageId{{
			MachineTag:    "machine-1",
			AttachmentTag: "volume-0",
		}})
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for change")
	}
	select {
	case <-w.Changes():
		c.Fatalf("received unexpected change")
	case <-time.After(coretesting.ShortWait):
	}

	statetesting.AssertStop(c, w)
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, jc.IsFalse)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for watcher channel to be closed")
	}
}
