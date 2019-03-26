// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"
	"sort"
	"strings"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
)

type machineSuite struct {
	entitySuite

	model    *cache.Model
	machine1 *cache.Machine
	unit1    *cache.Unit
}

var _ = gc.Suite(&machineSuite{})

func (s *machineSuite) SetUpTest(c *gc.C) {
	s.entitySuite.SetUpTest(c)
	s.model = s.newModel(modelChange)
}

func (s *machineSuite) TestUnits(c *gc.C) {
	machine, expectedUnits := s.setupMachineWithUnits(c, "0", []string{"test1", "test2"})
	obtainedUnits, err := machine.Units()
	c.Assert(err, jc.ErrorIsNil)
	sort.Sort(orderedUnits(obtainedUnits))
	sort.Sort(orderedUnits(expectedUnits))
	c.Assert(obtainedUnits, jc.DeepEquals, expectedUnits)
}

func (s *machineSuite) TestUnitsSubordinate(c *gc.C) {
	machine, expectedUnits := s.setupMachineWithUnits(c, "0", []string{"test1", "test2"})

	// add subordinate
	uc := unitChange
	uc.MachineId = ""
	uc.Name = "test5/0"
	uc.Application = "test5"
	uc.Principal = "test1/0"
	uc.Subordinate = true
	s.model.UpdateUnit(uc)
	unit, err := s.model.Unit(uc.Name)
	c.Assert(err, jc.ErrorIsNil)
	expectedUnits = append(expectedUnits, unit)

	obtainedUnits, err := machine.Units()
	c.Assert(err, jc.ErrorIsNil)
	sort.Sort(orderedUnits(obtainedUnits))
	sort.Sort(orderedUnits(expectedUnits))
	c.Assert(obtainedUnits, jc.DeepEquals, expectedUnits)
}

func (s *machineSuite) TestUnitsTwoMachines(c *gc.C) {
	machine0, expectedUnits0 := s.setupMachineWithUnits(c, "0", []string{"test1", "test2"})
	machine1, expectedUnits1 := s.setupMachineWithUnits(c, "1", []string{"test3", "test4", "test5"})

	obtainedUnits0, err := machine0.Units()
	c.Assert(err, jc.ErrorIsNil)
	obtainedUnits1, err := machine1.Units()
	c.Assert(err, jc.ErrorIsNil)

	sort.Sort(orderedUnits(obtainedUnits0))
	sort.Sort(orderedUnits(expectedUnits0))
	sort.Sort(orderedUnits(obtainedUnits1))
	sort.Sort(orderedUnits(expectedUnits1))

	c.Assert(obtainedUnits0, jc.DeepEquals, expectedUnits0)
	c.Assert(obtainedUnits1, jc.DeepEquals, expectedUnits1)
}

func (s *machineSuite) setupMachineWithUnits(c *gc.C, machineId string, apps []string) (*cache.Machine, []*cache.Unit) {
	mc := machineChange
	mc.Id = machineId
	s.model.UpdateMachine(mc)
	machine, err := s.model.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)

	units := make([]*cache.Unit, len(apps))
	for i, name := range apps {
		uc := unitChange
		uc.MachineId = machineId
		uc.Name = name + "/" + machineId
		uc.Application = name
		s.model.UpdateUnit(uc)
		unit, err := s.model.Unit(uc.Name)
		c.Assert(err, jc.ErrorIsNil)
		units[i] = unit
	}

	return machine, units
}

type orderedUnits []*cache.Unit

func (o orderedUnits) Len() int {
	return len(o)
}

func (o orderedUnits) Less(i, j int) bool {
	return strings.Compare(o[i].Application(), o[j].Application()) < 0
}

func (o orderedUnits) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
}

func (s *machineSuite) TestMachineAppLXDProfileWatcher(c *gc.C) {
	s.setupMachineAppLXDProfileWatcher(c)
	w := s.machine1.WatchApplicationLXDProfiles()
	defer workertest.CleanKill(c, w)
	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()
}

func (s *machineSuite) TestMachineAppLXDProfileWatcherNewCharmRevNoProfile(c *gc.C) {
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

func (s *machineSuite) TestMachineAppLXDProfileWatcherNewCharmRevProfile(c *gc.C) {
	s.setupMachineAppLXDProfileWatcher(c)
	w := s.machine1.WatchApplicationLXDProfiles()
	defer workertest.CleanKill(c, w)
	wc := NewNotifyWatcherC(c, w)
	// Sends initial event.
	wc.AssertOneChange()

	s.updateCharmForMachineAppLXDProfileWatcher("2", true)
	wc.AssertOneChange()
}

func (s *machineSuite) TestMachineAppLXDProfileWatcherAddUnit(c *gc.C) {
	s.testMachineAppLXDProfileWatcherAddUnit(c,
		cache.UnitChange{
			ModelUUID:   "model-uuid",
			Name:        "application-name/1",
			Application: "application-name",
			Series:      "bionic",
			CharmURL:    "www.charm-url.com-1",
			MachineId:   "0",
		}, true)
}

func (s *machineSuite) TestMachineAppLXDProfileWatcherAddUnitWrongMachine(c *gc.C) {
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

func (s *machineSuite) testMachineAppLXDProfileWatcherAddUnit(c *gc.C, ch cache.UnitChange, change bool) {
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

func (s *machineSuite) TestMachineAppLXDProfileWatcherRemoveUnitWithProfileTwoUnits(c *gc.C) {
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

func (s *machineSuite) TestMachineAppLXDProfileWatcherRemoveOnlyUnit(c *gc.C) {
	s.testMachineAppLXDProfileWatcherRemoveUnit(c,
		cache.RemoveUnit{
			ModelUUID: "model-uuid",
			Name:      "application-name/0",
		}, false)
}

func (s *machineSuite) TestMachineAppLXDProfileWatcherRemoveUnitWrongMachine(c *gc.C) {
	s.testMachineAppLXDProfileWatcherRemoveUnit(c,
		cache.RemoveUnit{
			ModelUUID: "model-uuid",
			Name:      "do-not-watch/2",
		}, false)
}

func (s *machineSuite) testMachineAppLXDProfileWatcherRemoveUnit(c *gc.C, ch cache.RemoveUnit, change bool) {
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

var machineChange = cache.MachineChange{
	ModelUUID:      "model-uuid",
	Id:             "0",
	InstanceId:     "juju-gd4c23-0",
	AgentStatus:    status.StatusInfo{Status: status.Active},
	InstanceStatus: status.StatusInfo{Status: status.Active},
	Life:           life.Alive,
	Config: map[string]interface{}{
		"key":     "value",
		"another": "foo",
	},
	Series:                   "bionic",
	SupportedContainers:      []instance.ContainerType{},
	SupportedContainersKnown: false,
	HasVote:                  true,
	WantsVote:                true,
}

func (s *machineSuite) updateCharmForMachineAppLXDProfileWatcher(rev string, profile bool) {
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

func (s *machineSuite) newUnitForMachineAppLXDProfileWatcherNoProfile() {
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

func (s *machineSuite) setupMachineAppLXDProfileWatcher(c *gc.C) {
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
