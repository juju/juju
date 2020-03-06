// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus/testutil"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
)

type lxdProfileWatcherSuite struct {
	cache.EntitySuite

	model    *cache.Model
	machine0 cache.Machine
	machine1 cache.Machine
	wc0      cache.NotifyWatcherC
}

var _ = gc.Suite(&lxdProfileWatcherSuite{})

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcher(c *gc.C) {
	w := s.assertStartOneMachineWatcher(c)

	// The worker is the first and only resource (1).
	resourceId := uint64(1)
	s.AssertWorkerResource(c, s.machine0.Resident, resourceId, true)
	workertest.CleanKill(c, w)
	s.AssertWorkerResource(c, s.machine0.Resident, resourceId, false)
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherError(c *gc.C) {
	s.setupOneMachineLXDProfileWatcherScenario(c)
	// set up a subordinate unit without a principal.
	uc := unitChange
	uc.Name = "subordinate/0"
	uc.Subordinate = true
	uc.Principal = "principal/0"
	s.model.UpdateUnit(uc, s.Manager)
	_, err := s.machine0.WatchLXDProfileVerificationNeeded()
	c.Assert(err, gc.ErrorMatches, "failed to get units to start MachineLXDProfileWatcher: principal unit \"principal/0\" for subordinate subordinate/0 not found")
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherNewCharmRevNoProfile(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))

	// Start with a charm having a profile so change the charm's profile
	// from existing to not, should be notified to remove the profile.
	s.updateCharmForMachineLXDProfileWatcher("2", false)
	s.assertChangeValidateMetrics(c, s.wc0.AssertOneChange, 0, 1, 0)

	// Changing the charm url, and the profile stays empty,
	// should not be notified to remove the profile.
	s.updateCharmForMachineLXDProfileWatcher("3", false)
	s.assertChangeValidateMetrics(c, s.wc0.AssertNoChange, 0, 1, 1)
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherNewCharmRevProfile(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))
	s.updateCharmForMachineLXDProfileWatcher("2", true)
	s.assertChangeValidateMetrics(c, s.wc0.AssertOneChange, 0, 1, 0)
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherAddUnit(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))

	// New unit added to existing machine doesn't have a charm url yet.
	// It may been added without a machine id either.
	s.model.UpdateUnit(
		cache.UnitChange{
			ModelUUID:   "model-uuid",
			Name:        "application-name/1",
			Application: "application-name",
			Series:      "bionic",
		}, s.Manager)
	s.assertChangeValidateMetrics(c, s.wc0.AssertNoChange, 0, 0, 0)

	// Add the machine id, this time we should get a notification.
	s.model.UpdateUnit(cache.UnitChange{
		ModelUUID:   "model-uuid",
		Name:        "application-name/1",
		Application: "application-name",
		Series:      "bionic",
		MachineId:   "0",
	}, s.Manager)
	s.assertChangeValidateMetrics(c, s.wc0.AssertOneChange, 0, 1, 0)
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherAddUnitWrongMachine(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))
	s.model.UpdateUnit(
		cache.UnitChange{
			ModelUUID:   "model-uuid",
			Name:        "do-not-watch/2",
			Application: "do-no-watch",
			Series:      "bionic",
			CharmURL:    "www.no-charm-url.com-1",
			MachineId:   "42",
		}, s.Manager)
	s.assertChangeValidateMetrics(c, s.wc0.AssertNoChange, 0, 0, 1)
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherTwoMachines(c *gc.C) {
	s.setupTwoMachineLXDProfileWatcherScenario(c)

	w0, err := s.machine0.WatchLXDProfileVerificationNeeded()
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w0)
	wc0 := cache.NewNotifyWatcherC(c, w0)
	// Sends initial event.
	s.assertChangeValidateMetrics(c, wc0.AssertOneChange, 0, 0, 0)

	w1, err := s.machine1.WatchLXDProfileVerificationNeeded()
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w1)
	wc1 := cache.NewNotifyWatcherC(c, w1)
	// Sends initial event.
	s.assertChangeValidateMetrics(c, wc1.AssertOneChange, 0, 0, 0)

	// Add a unit to machine 0
	uc := unitChange
	uc.Name = "application-name/2"
	uc.MachineId = s.machine0.Id()
	s.model.UpdateUnit(uc, s.Manager)

	// Assert machine 0 watcher gets the notification,
	// not machine 1.
	wc1.AssertNoChange()
	s.assertChangeValidateMetrics(c, wc0.AssertOneChange, 0, 1, 1)
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherSubordinateWithProfile(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))
	// Add a new subordinate unit with a profile of a new application.
	s.newUnitForMachineLXDProfileWatcherSubProfile(c, s.machine0.Id(), unitChange.Name)
	s.assertChangeValidateMetrics(c, s.wc0.AssertOneChange, 0, 1, 1)
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherSubordinateWithProfileUpdateUnit(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))

	// Add a new subordinate unit with a profile of a new application.
	s.newUnitForMachineLXDProfileWatcherSubProfile(c, s.machine0.Id(), unitChange.Name)
	s.assertChangeValidateMetrics(c, s.wc0.AssertOneChange, 0, 1, 1)

	// Add a new subordinate.
	subordinate := cache.UnitChange{
		ModelUUID:   "model-uuid",
		Name:        "name-me/0",
		Application: "name-me",
		Series:      "bionic",
		Principal:   unitChange.Name,
		Subordinate: true,
	}
	s.model.UpdateUnit(subordinate, s.Manager)

	s.assertChangeValidateMetrics(c, s.wc0.AssertNoChange, 0, 1, 2)

	// A subordinate status change should cause no notification.
	subordinate.AgentStatus = status.StatusInfo{
		Status: "sweet-as-bro",
	}

	s.assertChangeValidateMetrics(c, s.wc0.AssertNoChange, 0, 1, 2)
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherSubordinateNoProfile(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))
	// Add a new subordinate unit with no profile of a new application.
	s.newUnitForMachineLXDProfileWatcherNoProfile(c, s.machine0.Id(), unitChange.Name)
	s.assertChangeValidateMetrics(c, s.wc0.AssertNoChange, 0, 0, 2)
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherRemoveUnitWithProfileTwoUnits(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))

	// Add a new unit of a new application.
	s.newUnitForMachineLXDProfileWatcherNoProfile(c, s.machine0.Id(), "")
	s.assertChangeValidateMetrics(c, s.wc0.AssertNoChange, 0, 0, 2)

	// Remove the original unit which has a profile.
	c.Assert(s.model.RemoveUnit(
		cache.RemoveUnit{
			ModelUUID: "model-uuid",
			Name:      "application-name/0",
		}), jc.ErrorIsNil)
	s.assertChangeValidateMetrics(c, s.wc0.AssertOneChange, 0, 1, 2)
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherRemoveOnlyUnit(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))
	ru := cache.RemoveUnit{
		ModelUUID: "model-uuid",
		Name:      "application-name/0",
	}
	c.Assert(s.model.RemoveUnit(ru), jc.ErrorIsNil)
	s.assertChangeValidateMetrics(c, s.wc0.AssertNoChange, 0, 0, 1)
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherRemoveUnitWrongMachine(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))
	ru := cache.RemoveUnit{
		ModelUUID: "model-uuid",
		Name:      "do-not-watch/2",
	}
	c.Assert(s.model.RemoveUnit(ru), jc.ErrorIsNil)
	s.assertChangeValidateMetrics(c, s.wc0.AssertNoChange, 0, 0, 0)
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherAppChangeCharmURLNotFound(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))
	s.model.UpdateApplication(cache.ApplicationChange{
		ModelUUID: "model-uuid",
		Name:      "application-name",
		CharmURL:  "charm-url-does-not-exist",
	}, s.Manager)
	s.assertChangeValidateMetrics(c, s.wc0.AssertNoChange, 1, 0, 1)
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherUnitChangeAppNotFound(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))
	s.model.UpdateUnit(cache.UnitChange{
		ModelUUID:   "model-uuid",
		Name:        "testme/0",
		Application: "application-name-does-not-exist",
		MachineId:   s.machine0.Id(),
	}, s.Manager)
	s.assertChangeValidateMetrics(c, s.wc0.AssertNoChange, 1, 0, 1)
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherUnitChangeCharmURLNotFound(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))
	s.model.UpdateApplication(cache.ApplicationChange{
		ModelUUID: "model-uuid",
		Name:      "new-application-name",
		CharmURL:  "charm-url-does-not-exist",
	}, s.Manager)
	s.model.UpdateUnit(cache.UnitChange{
		ModelUUID:   "model-uuid",
		Name:        "testme/0",
		Application: "new-application-name",
		MachineId:   s.machine0.Id(),
	}, s.Manager)
	s.assertChangeValidateMetrics(c, s.wc0.AssertNoChange, 1, 0, 2)
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherUnitChangeUnitCharmURLIgnored(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineWatcher(c))
	s.model.UpdateUnit(cache.UnitChange{
		ModelUUID:   "model-uuid",
		Name:        "testme/0",
		Application: "application-name",
		CharmURL:    "charm-url-does-not-exist",
		MachineId:   s.machine0.Id(),
	}, s.Manager)
	s.assertChangeValidateMetrics(c, s.wc0.AssertOneChange, 0, 1, 0)
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherMachineProvisioned(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineNotProvisionedWatcher(c))
	s.model.UpdateMachine(cache.MachineChange{
		ModelUUID:  "model-uuid",
		Id:         "0",
		InstanceId: "juju-gd4c23-0",
	}, s.Manager)
	s.assertChangeValidateMetrics(c, s.wc0.AssertOneChange, 0, 1, 0)
}

func (s *lxdProfileWatcherSuite) TestMachineLXDProfileWatcherMachineProvisionedOneOnly(c *gc.C) {
	defer workertest.CleanKill(c, s.assertStartOneMachineNotProvisionedWatcher(c))
	s.model.UpdateMachine(cache.MachineChange{
		ModelUUID:  "model-uuid",
		Id:         "1",
		InstanceId: "juju-gd4c23-1",
	}, s.Manager)
	s.assertChangeValidateMetrics(c, s.wc0.AssertNoChange, 0, 0, 0)
}

func (s *lxdProfileWatcherSuite) updateCharmForMachineLXDProfileWatcher(rev string, profile bool) {
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
	s.model.UpdateCharm(ch, s.Manager)

	s.model.UpdateApplication(cache.ApplicationChange{
		ModelUUID: "model-uuid",
		Name:      "application-name",
		CharmURL:  curl,
	}, s.Manager)
}

func (s *lxdProfileWatcherSuite) newUnitForMachineLXDProfileWatcherNoProfile(c *gc.C, machineId, principal string) {
	s.newUnit(c, machineId, principal, cache.CharmChange{
		ModelUUID: "model-uuid",
		CharmURL:  "cs:name-me-345",
	})
}

func (s *lxdProfileWatcherSuite) newUnitForMachineLXDProfileWatcherSubProfile(c *gc.C, machineId, principal string) {
	s.newUnit(c, machineId, principal, cache.CharmChange{
		ModelUUID: "model-uuid",
		CharmURL:  "cs:name-me-345",
		LXDProfile: lxdprofile.Profile{
			Config: map[string]string{"keySub": "valueSub"},
		},
	})
}

func (s *lxdProfileWatcherSuite) newUnit(c *gc.C, machineId, principal string, cc cache.CharmChange) {
	s.model.UpdateCharm(cc, s.Manager)
	_, err := s.model.Charm(cc.CharmURL)
	c.Assert(err, jc.ErrorIsNil)

	ac := appChange
	ac.CharmURL = cc.CharmURL
	ac.Name = "name-me"
	s.model.UpdateApplication(ac, s.Manager)
	_, err = s.model.Application(ac.Name)
	c.Assert(err, jc.ErrorIsNil)

	uc := unitChange
	uc.Application = ac.Name
	uc.CharmURL = ac.CharmURL
	uc.Name = "name-me/0"
	if principal != "" {
		uc.Principal = principal
		uc.Subordinate = true
	} else {
		uc.MachineId = machineId
	}
	s.model.UpdateUnit(uc, s.Manager)
	_, err = s.model.Unit(uc.Name)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *lxdProfileWatcherSuite) setupOneMachineLXDProfileWatcherScenario(c *gc.C) {
	s.model = s.NewModel(modelChange)

	s.model.UpdateMachine(machineChange, s.Manager)
	machine, err := s.model.Machine(machineChange.Id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Id(), gc.Equals, machineChange.Id)
	s.machine0 = machine

	s.model.UpdateApplication(appChange, s.Manager)
	s.model.UpdateUnit(unitChange, s.Manager)
	s.model.UpdateCharm(charmChange, s.Manager)
}

func (s *lxdProfileWatcherSuite) setupTwoMachineLXDProfileWatcherScenario(c *gc.C) {
	s.setupOneMachineLXDProfileWatcherScenario(c)

	mc := machineChange
	mc.Id = "1"

	s.model.UpdateMachine(mc, s.Manager)
	machine, err := s.model.Machine(mc.Id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Id(), gc.Equals, mc.Id)
	s.machine1 = machine

	uc := unitChange
	uc.Name = "application-name/1"
	uc.MachineId = "1"
	s.model.UpdateUnit(uc, s.Manager)
	_, err = s.model.Unit(uc.Name)
	c.Assert(err, jc.ErrorIsNil)

	s.model.UpdateCharm(charmChange, s.Manager)
}

func (s *lxdProfileWatcherSuite) assertStartOneMachineWatcher(c *gc.C) *cache.MachineLXDProfileWatcher {
	s.setupOneMachineLXDProfileWatcherScenario(c)
	w, err := s.machine0.WatchLXDProfileVerificationNeeded()
	c.Assert(err, jc.ErrorIsNil)

	wc := cache.NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()
	s.wc0 = wc
	return w
}

func (s *lxdProfileWatcherSuite) assertStartOneMachineNotProvisionedWatcher(c *gc.C) *cache.MachineLXDProfileWatcher {
	s.model = s.NewModel(modelChange)

	mChange := cache.MachineChange{
		ModelUUID: "model-uuid",
		Id:        "0",
	}
	s.model.UpdateMachine(mChange, s.Manager)
	machine, err := s.model.Machine(machineChange.Id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Id(), gc.Equals, machineChange.Id)
	s.machine0 = machine

	w, err := s.machine0.WatchLXDProfileVerificationNeeded()
	c.Assert(err, jc.ErrorIsNil)

	wc := cache.NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()
	s.wc0 = wc
	return w
}

func (s *lxdProfileWatcherSuite) assertChangeValidateMetrics(c *gc.C, assert func(), err, hit, miss int) {
	assert()
	c.Check(testutil.ToFloat64(s.Gauges.LXDProfileChangeError), gc.Equals, float64(err))
	c.Check(testutil.ToFloat64(s.Gauges.LXDProfileChangeNotification), gc.Equals, float64(hit))
	c.Check(testutil.ToFloat64(s.Gauges.LXDProfileNoChange), gc.Equals, float64(miss))
}
