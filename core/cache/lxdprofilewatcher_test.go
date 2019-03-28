// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/lxdprofile"
)

type lxdProfileWatcherSuite struct {
	entitySuite

	model    *cache.Model
	machine0 *cache.Machine
	machine1 *cache.Machine
}

var _ = gc.Suite(&lxdProfileWatcherSuite{})

func (s *lxdProfileWatcherSuite) SetUpTest(c *gc.C) {
	s.entitySuite.SetUpTest(c)
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcher(c *gc.C) {
	s.setupOneMachineAppLXDProfileWatcherScenario(c)
	w := s.machine0.WatchApplicationLXDProfiles()
	defer workertest.CleanKill(c, w)
	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcherNewCharmRevNoProfile(c *gc.C) {
	s.setupOneMachineAppLXDProfileWatcherScenario(c)
	w := s.machine0.WatchApplicationLXDProfiles()
	defer workertest.CleanKill(c, w)
	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()

	// Start with a charm having a profile so change the charm's profile
	// from existing to not, should be notified to remove the profile.
	s.updateCharmForMachineAppLXDProfileWatcher("2", false)
	wc.AssertOneChange()

	// Changing the charm url, and the profile stays empty,
	// should not be notified to remove the profile.
	s.updateCharmForMachineAppLXDProfileWatcher("3", false)
	wc.AssertNoChange()
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcherNewCharmRevProfile(c *gc.C) {
	s.setupOneMachineAppLXDProfileWatcherScenario(c)
	w := s.machine0.WatchApplicationLXDProfiles()
	defer workertest.CleanKill(c, w)
	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()

	s.updateCharmForMachineAppLXDProfileWatcher("2", true)
	wc.AssertOneChange()
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcherAddUnit(c *gc.C) {
	// New unit added to existing machine doesn't have a charm url yet.
	s.testMachineAppLXDProfileWatcherAddUnit(c,
		cache.UnitChange{
			ModelUUID:   "model-uuid",
			Name:        "application-name/1",
			Application: "application-name",
			Series:      "bionic",
			//CharmURL:    "www.charm-url.com-1",
			MachineId: "0",
		}, true)
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcherAddUnitWrongMachine(c *gc.C) {
	s.testMachineAppLXDProfileWatcherAddUnit(c,
		cache.UnitChange{
			ModelUUID:   "model-uuid",
			Name:        "do-not-watch/2",
			Application: "do-no-watch",
			Series:      "bionic",
			CharmURL:    "www.no-charm-url.com-1",
			MachineId:   "42",
		}, false)
}

func (s *lxdProfileWatcherSuite) testMachineAppLXDProfileWatcherAddUnit(c *gc.C, ch cache.UnitChange, change bool) {
	s.setupOneMachineAppLXDProfileWatcherScenario(c)
	w := s.machine0.WatchApplicationLXDProfiles()
	defer workertest.CleanKill(c, w)
	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()

	s.model.UpdateUnit(ch)

	if change {
		wc.AssertOneChange()
	} else {
		wc.AssertNoChange()
	}
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcherTwoMachines(c *gc.C) {
	s.setupTwoMachineAppLXDProfileWatcherScenario(c)

	w0 := s.machine0.WatchApplicationLXDProfiles()
	defer workertest.CleanKill(c, w0)
	wc0 := NewNotifyWatcherC(c, w0)
	// Sends initial event.
	wc0.AssertOneChange()

	w1 := s.machine1.WatchApplicationLXDProfiles()
	defer workertest.CleanKill(c, w1)
	wc1 := NewNotifyWatcherC(c, w1)
	// Sends initial event.
	wc1.AssertOneChange()

	// Add a unit to machine 0
	uc := unitChange
	uc.Name = "application-name/2"
	uc.MachineId = s.machine0.Id()
	s.model.UpdateUnit(uc)

	// Assert machine 0 watcher gets the notification,
	// not machine 1.
	wc0.AssertOneChange()
	wc1.AssertNoChange()
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcherSubordinates(c *gc.C) {

}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcherRemoveUnitWithProfileTwoUnits(c *gc.C) {
	s.setupOneMachineAppLXDProfileWatcherScenario(c)
	w := s.machine0.WatchApplicationLXDProfiles()
	defer workertest.CleanKill(c, w)
	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()

	// Add a new unit of a new application.
	s.newUnitForMachineAppLXDProfileWatcherNoProfile()

	// Remove the original unit which has a profile.
	s.model.RemoveUnit(
		cache.RemoveUnit{
			ModelUUID: "model-uuid",
			Name:      "application-name/0",
		})
	wc.AssertOneChange()
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcherRemoveOnlyUnit(c *gc.C) {
	s.testMachineAppLXDProfileWatcherRemoveUnit(c,
		cache.RemoveUnit{
			ModelUUID: "model-uuid",
			Name:      "application-name/0",
		}, false)
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcherRemoveUnitWrongMachine(c *gc.C) {
	s.testMachineAppLXDProfileWatcherRemoveUnit(c,
		cache.RemoveUnit{
			ModelUUID: "model-uuid",
			Name:      "do-not-watch/2",
		}, false)
}

func (s *lxdProfileWatcherSuite) testMachineAppLXDProfileWatcherRemoveUnit(c *gc.C, ch cache.RemoveUnit, change bool) {
	s.setupOneMachineAppLXDProfileWatcherScenario(c)
	w := s.machine0.WatchApplicationLXDProfiles()
	defer workertest.CleanKill(c, w)
	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()

	s.model.RemoveUnit(ch)
	if change {
		wc.AssertOneChange()
	} else {
		wc.AssertNoChange()
	}
}

func (s *lxdProfileWatcherSuite) updateCharmForMachineAppLXDProfileWatcher(rev string, profile bool) {
	curl := "www.charm-url.com-" + rev
	ch := cache.CharmChange{
		ModelUUID: "model-uuid",
		CharmURL:  curl,
	}
	if profile {
		ch.LXDProfile = lxdprofile.Profile{
			Config: map[string]string{"key1": "value1"},
		}
	} else {
		ch.LXDProfile = lxdprofile.Profile{}
	}
	s.model.UpdateCharm(ch)

	s.model.UpdateApplication(cache.ApplicationChange{
		ModelUUID: "model-uuid",
		Name:      "application-name",
		CharmURL:  curl,
	})
}

func (s *lxdProfileWatcherSuite) newUnitForMachineAppLXDProfileWatcherNoProfile() {
	cc := cache.CharmChange{
		ModelUUID: "model-uuid",
		CharmURL:  "cs:name-me-345",
	}
	s.model.UpdateCharm(cc)

	ac := appChange
	ac.CharmURL = cc.CharmURL
	ac.Name = "name-me"
	s.model.UpdateApplication(appChange)

	uc := unitChange
	uc.Application = ac.Name
	uc.CharmURL = ac.CharmURL
	uc.Name = "name-me/0"
	uc.MachineId = "0"
	s.model.UpdateUnit(uc)
}

func (s *lxdProfileWatcherSuite) setupOneMachineAppLXDProfileWatcherScenario(c *gc.C) {
	s.model = s.newModel(modelChange)

	s.model.UpdateMachine(machineChange)
	machine, err := s.model.Machine(machineChange.Id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Id(), gc.Equals, machineChange.Id)
	s.machine0 = machine

	s.model.UpdateApplication(appChange)
	s.model.UpdateUnit(unitChange)
	s.model.UpdateCharm(charmChange)
}

func (s *lxdProfileWatcherSuite) setupTwoMachineAppLXDProfileWatcherScenario(c *gc.C) {
	s.setupOneMachineAppLXDProfileWatcherScenario(c)

	mc := machineChange
	machineChange.Id = "1"

	s.model.UpdateMachine(mc)
	machine, err := s.model.Machine(mc.Id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Id(), gc.Equals, mc.Id)
	s.machine1 = machine

	uc := unitChange
	uc.Name = "application-name/1"
	s.model.UpdateUnit(uc)
	_, err = s.model.Unit(uc.Name)
	c.Assert(err, jc.ErrorIsNil)

	s.model.UpdateCharm(charmChange)
}
