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
	wc0      NotifyWatcherC
}

var _ = gc.Suite(&lxdProfileWatcherSuite{})

func (s *lxdProfileWatcherSuite) SetUpTest(c *gc.C) {
	s.entitySuite.SetUpTest(c)
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcher(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcherError(c *gc.C) {
	s.setupOneMachineAppLXDProfileWatcherScenario(c)
	// set up a subordinate unit without a principal.
	uc := unitChange
	uc.Name = "subordinate/0"
	uc.Subordinate = true
	uc.Principal = "principal/0"
	s.model.UpdateUnit(uc)
	_, err := s.machine0.WatchApplicationLXDProfiles()
	c.Assert(err, gc.ErrorMatches, "failed to get units to start MachineAppLXDProfileWatcher: principal unit \"principal/0\" for subordinate subordinate/0 not found")
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcherNewCharmRevNoProfile(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))

	// Start with a charm having a profile so change the charm's profile
	// from existing to not, should be notified to remove the profile.
	s.updateCharmForMachineAppLXDProfileWatcher("2", false)
	s.wc0.AssertOneChange()

	// Changing the charm url, and the profile stays empty,
	// should not be notified to remove the profile.
	s.updateCharmForMachineAppLXDProfileWatcher("3", false)
	s.wc0.AssertNoChange()
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcherNewCharmRevProfile(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))
	s.updateCharmForMachineAppLXDProfileWatcher("2", true)
	s.wc0.AssertOneChange()
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcherAddUnit(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))

	// New unit added to existing machine doesn't have a charm url yet.
	// It may been added without a machine id either.
	s.model.UpdateUnit(
		cache.UnitChange{
			ModelUUID:   "model-uuid",
			Name:        "application-name/1",
			Application: "application-name",
			Series:      "bionic",
		})
	s.wc0.AssertNoChange()

	// Add the machine id, this time we should get a notification.
	s.model.UpdateUnit(cache.UnitChange{
		ModelUUID:   "model-uuid",
		Name:        "application-name/1",
		Application: "application-name",
		Series:      "bionic",
		MachineId:   "0",
	})
	s.wc0.AssertOneChange()
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcherAddUnitWrongMachine(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))
	s.model.UpdateUnit(
		cache.UnitChange{
			ModelUUID:   "model-uuid",
			Name:        "do-not-watch/2",
			Application: "do-no-watch",
			Series:      "bionic",
			CharmURL:    "www.no-charm-url.com-1",
			MachineId:   "42",
		})
	s.wc0.AssertNoChange()
}

func (s *lxdProfileWatcherSuite) TestMachineAppLXDProfileWatcherTwoMachines(c *gc.C) {
	s.setupTwoMachineAppLXDProfileWatcherScenario(c)

	w0, err := s.machine0.WatchApplicationLXDProfiles()
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w0)
	wc0 := NewNotifyWatcherC(c, w0)
	// Sends initial event.
	wc0.AssertOneChange()

	w1, err := s.machine1.WatchApplicationLXDProfiles()
	c.Assert(err, jc.ErrorIsNil)
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
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))

	// Add a new unit of a new application.
	s.newUnitForMachineAppLXDProfileWatcherNoProfile()

	// Remove the original unit which has a profile.
	s.model.RemoveUnit(
		cache.RemoveUnit{
			ModelUUID: "model-uuid",
			Name:      "application-name/0",
		})
	s.wc0.AssertOneChange()
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
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))
	s.model.RemoveUnit(ch)
	if change {
		s.wc0.AssertOneChange()
	} else {
		s.wc0.AssertNoChange()
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
	mc.Id = "1"

	s.model.UpdateMachine(mc)
	machine, err := s.model.Machine(mc.Id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Id(), gc.Equals, mc.Id)
	s.machine1 = machine

	uc := unitChange
	uc.Name = "application-name/1"
	uc.MachineId = "1"
	s.model.UpdateUnit(uc)
	_, err = s.model.Unit(uc.Name)
	c.Assert(err, jc.ErrorIsNil)

	s.model.UpdateCharm(charmChange)
}

func (s *lxdProfileWatcherSuite) assertStartOneMachineWatcher(c *gc.C) *cache.MachineAppLXDProfileWatcher {
	s.setupOneMachineAppLXDProfileWatcherScenario(c)
	w, err := s.machine0.WatchApplicationLXDProfiles()
	c.Assert(err, jc.ErrorIsNil)
	//defer workertest.CleanKill(c, w)
	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()
	s.wc0 = wc
	return w
}
