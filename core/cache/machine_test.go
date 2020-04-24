// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	"sort"
	"strings"
	"time"

	"github.com/juju/juju/testing"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
)

type machineSuite struct {
	cache.EntitySuite

	model    *cache.Model
	machine0 cache.Machine
	wc0      cache.StringsWatcherC
}

var _ = gc.Suite(&machineSuite{})

func (s *machineSuite) SetUpTest(c *gc.C) {
	s.EntitySuite.SetUpTest(c)
	s.model = s.NewModel(modelChange)
}

func (s *machineSuite) TestInstanceId(c *gc.C) {
	machine, _ := s.setupMachineWithUnits(c, "0", []string{"test1"})

	id, err := machine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(id, gc.Equals, instance.Id("juju-gd4c23-0"))
}

func (s *machineSuite) TestEmptyInstanceId(c *gc.C) {
	mc := cache.MachineChange{
		Id: "0",
	}
	s.model.UpdateMachine(mc, s.Manager)

	machine, err := s.model.Machine("0")
	c.Assert(err, jc.ErrorIsNil)

	_, err = machine.InstanceId()
	c.Assert(err, gc.ErrorMatches, "machine 0 not provisioned")
}

func (s *machineSuite) TestCharmProfiles(c *gc.C) {
	mc := cache.MachineChange{
		Id:            "0",
		CharmProfiles: []string{"charm-profile-1"},
	}
	s.model.UpdateMachine(mc, s.Manager)

	machine, err := s.model.Machine("0")
	c.Assert(err, jc.ErrorIsNil)

	profiles := machine.CharmProfiles()
	c.Assert(profiles, gc.DeepEquals, mc.CharmProfiles)
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
	s.model.UpdateUnit(uc, s.Manager)
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

func (s *machineSuite) TestWatchContainersStops(c *gc.C) {
	s.setupMachine0WithContainerWatcher(c, false)

	// The worker is the first and only resource (1).
	resourceId := uint64(1)
	s.AssertWorkerResource(c, s.machine0.Resident, resourceId, true)
	s.wc0.AssertStops()
	s.AssertWorkerResource(c, s.machine0.Resident, resourceId, false)
}

func (s *machineSuite) TestWatchContainersStartWithContainer(c *gc.C) {
	defer workertest.CleanKill(c, s.setupMachine0WithContainerWatcher(c, true))
}

func (s *machineSuite) TestWatchContainersAddContainer(c *gc.C) {
	defer workertest.CleanKill(c, s.setupMachine0WithContainerWatcher(c, false))

	// Add a container to the machine
	mc := machineChange
	mc.Id = "0/lxd/0"
	s.model.UpdateMachine(mc, s.Manager)
	s.wc0.AssertOneChange([]string{mc.Id})
}

func (s *machineSuite) TestWatchContainersOnlyThisMachinesAddContainers(c *gc.C) {
	defer workertest.CleanKill(c, s.setupMachine0WithContainerWatcher(c, false))

	// Add a container to a different machine
	mc := machineChange
	mc.Id = "1/lxd/0"
	s.model.UpdateMachine(mc, s.Manager)
	s.wc0.AssertNoChange()
}

func (s *machineSuite) TestWatchContainersOnlyThisMachinesRemoveContainers(c *gc.C) {
	defer workertest.CleanKill(c, s.setupMachine0WithContainerWatcher(c, false))

	// Remove a container from a different machine
	rm := cache.RemoveMachine{
		ModelUUID: modelChange.ModelUUID,
		Id:        "1/lxd/0",
	}
	c.Assert(s.model.RemoveMachine(rm), jc.ErrorIsNil)
	s.wc0.AssertNoChange()
}

func (s *machineSuite) TestWatchContainersRemoveContainer(c *gc.C) {
	defer workertest.CleanKill(c, s.setupMachine0WithContainerWatcher(c, true))

	// Remove a container from this machine
	rm := cache.RemoveMachine{
		ModelUUID: modelChange.ModelUUID,
		Id:        "0/lxd/0",
	}
	c.Assert(s.model.RemoveMachine(rm), jc.ErrorIsNil)
	s.wc0.AssertOneChange([]string{rm.Id})
}

func (s *machineSuite) TestMachineArrivesProvisionedPublished(c *gc.C) {
	msg := make(chan struct{}, 1)
	unsub := s.Hub.Subscribe(
		machineChange.Id+":machine-provisioned",
		func(_ string, _ interface{}) { msg <- struct{}{} },
	)
	defer unsub()

	s.NewModel(modelChange).UpdateMachine(machineChange, s.Manager)

	select {
	case <-msg:
	case <-time.After(testing.LongWait):
		c.Fatal("machine provisioned message not received")
	}
}

func (s *machineSuite) setupMachine0(c *gc.C) {
	s.model.UpdateMachine(machineChange, s.Manager)
	machine, err := s.model.Machine(machineChange.Id)
	c.Assert(err, jc.ErrorIsNil)
	s.machine0 = machine
}

func (s *machineSuite) setupMachine0WithContainerWatcher(c *gc.C, addContainer bool) *cache.PredicateStringsWatcher {
	s.setupMachine0(c)

	if addContainer {
		s.setupMachine0Container(c)
	}

	w, err := s.machine0.WatchContainers()
	c.Assert(err, jc.ErrorIsNil)
	wc := cache.NewStringsWatcherC(c, w)
	// Sends initial event.
	if addContainer {
		wc.AssertOneChange([]string{"0/lxd/0"})
	} else {
		wc.AssertOneChange([]string{})
	}

	s.wc0 = wc
	return w
}

func (s *machineSuite) setupMachine0Container(c *gc.C) {
	// Add a container to the machine
	mc := machineChange
	mc.Id = "0/lxd/0"
	s.model.UpdateMachine(mc, s.Manager)
}

func (s *machineSuite) setupMachineWithUnits(c *gc.C, machineId string, apps []string) (cache.Machine, []cache.Unit) {
	mc := machineChange
	mc.Id = machineId
	s.model.UpdateMachine(mc, s.Manager)
	machine, err := s.model.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)

	units := make([]cache.Unit, len(apps))
	for i, name := range apps {
		uc := unitChange
		uc.MachineId = machineId
		uc.Name = name + "/" + machineId
		uc.Application = name
		s.model.UpdateUnit(uc, s.Manager)

		unit, err := s.model.Unit(uc.Name)
		c.Assert(err, jc.ErrorIsNil)
		units[i] = unit
	}

	return machine, units
}

type orderedUnits []cache.Unit

func (o orderedUnits) Len() int {
	return len(o)
}

func (o orderedUnits) Less(i, j int) bool {
	return strings.Compare(o[i].Application(), o[j].Application()) < 0
}

func (o orderedUnits) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
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
