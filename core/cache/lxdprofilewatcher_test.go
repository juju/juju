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
	machine1 *cache.Machine
	unit1    *cache.Unit
}

var _ = gc.Suite(&lxdProfileWatcherSuite{})

func (s *lxdProfileWatcherSuite) SetUpTest(c *gc.C) {
	s.entitySuite.SetUpTest(c)
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcher(c *gc.C) {
	s.setupMachineAppLXDProfileWatcher(c)
	w := s.machine1.WatchApplicationLXDProfiles()
	defer workertest.CleanKill(c, w)
	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcherNewCharmRevNoProfile(c *gc.C) {
	s.setupMachineAppLXDProfileWatcher(c)
	w := s.machine1.WatchApplicationLXDProfiles()
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
	s.setupMachineAppLXDProfileWatcher(c)
	w := s.machine1.WatchApplicationLXDProfiles()
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
	s.setupMachineAppLXDProfileWatcher(c)
	w := s.machine1.WatchApplicationLXDProfiles()
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

}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcherRemoveUnitWithProfileTwoUnits(c *gc.C) {
	s.setupMachineAppLXDProfileWatcher(c)
	w := s.machine1.WatchApplicationLXDProfiles()
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
	s.setupMachineAppLXDProfileWatcher(c)
	w := s.machine1.WatchApplicationLXDProfiles()
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

func (s *lxdProfileWatcherSuite) setupMachineAppLXDProfileWatcher(c *gc.C) {
	s.model = s.newModel(modelChange)

	s.model.UpdateMachine(machineChange)
	machine, err := s.model.Machine(machineChange.Id)
	c.Assert(err, jc.ErrorIsNil)
	s.machine1 = machine

	s.model.UpdateApplication(appChange)

	s.model.UpdateUnit(unitChange)
	unit, err := s.model.Unit(unitChange.Name)
	c.Assert(err, jc.ErrorIsNil)
	s.unit1 = unit

	s.model.UpdateCharm(charmChange)

	m, err := s.model.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, "0")
}
