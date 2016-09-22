// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/network"
	"github.com/juju/juju/payload"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testing/factory"
)

type MigrationImportSuite struct {
	MigrationBaseSuite
}

var _ = gc.Suite(&MigrationImportSuite{})

func (s *MigrationImportSuite) checkStatusHistory(c *gc.C, exported, imported status.StatusHistoryGetter, size int) {
	exportedHistory, err := exported.StatusHistory(status.StatusHistoryFilter{Size: size})
	c.Assert(err, jc.ErrorIsNil)
	importedHistory, err := imported.StatusHistory(status.StatusHistoryFilter{Size: size})
	c.Assert(err, jc.ErrorIsNil)
	for i := 0; i < size; i++ {
		c.Check(importedHistory[i].Status, gc.Equals, exportedHistory[i].Status)
		c.Check(importedHistory[i].Message, gc.Equals, exportedHistory[i].Message)
		c.Check(importedHistory[i].Data, jc.DeepEquals, exportedHistory[i].Data)
		c.Check(importedHistory[i].Since, jc.DeepEquals, exportedHistory[i].Since)
	}
}

func (s *MigrationImportSuite) TestExisting(c *gc.C) {
	out, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.State.Import(out)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *MigrationImportSuite) importModel(c *gc.C) (*state.Model, *state.State) {
	out, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	uuid := utils.MustNewUUID().String()
	in := newModel(out, uuid, "new")

	newModel, newSt, err := s.State.Import(in)
	c.Assert(err, jc.ErrorIsNil)
	// add the cleanup here to close the model.
	s.AddCleanup(func(c *gc.C) {
		c.Check(newSt.Close(), jc.ErrorIsNil)
	})
	return newModel, newSt
}

func (s *MigrationImportSuite) assertAnnotations(c *gc.C, newSt *state.State, entity state.GlobalEntity) {
	annotations, err := newSt.Annotations(entity)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(annotations, jc.DeepEquals, testAnnotations)
}

func (s *MigrationImportSuite) TestNewModel(c *gc.C) {
	cons := constraints.MustParse("arch=amd64 mem=8G")
	latestTools := version.MustParse("2.0.1")
	s.setLatestTools(c, latestTools)
	c.Assert(s.State.SetModelConstraints(cons), jc.ErrorIsNil)
	machineSeq := s.setRandSequenceValue(c, "machine")
	fooSeq := s.setRandSequenceValue(c, "application-foo")
	s.State.SwitchBlockOn(state.ChangeBlock, "locked down")

	original, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SetAnnotations(original, testAnnotations)
	c.Assert(err, jc.ErrorIsNil)

	out, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	uuid := utils.MustNewUUID().String()
	in := newModel(out, uuid, "new")

	newModel, newSt, err := s.State.Import(in)
	c.Assert(err, jc.ErrorIsNil)
	defer newSt.Close()

	c.Assert(newModel.Owner(), gc.Equals, original.Owner())
	c.Assert(newModel.LatestToolsVersion(), gc.Equals, latestTools)
	c.Assert(newModel.MigrationMode(), gc.Equals, state.MigrationModeImporting)
	s.assertAnnotations(c, newSt, newModel)

	originalConfig, err := original.Config()
	c.Assert(err, jc.ErrorIsNil)
	originalAttrs := originalConfig.AllAttrs()

	newConfig, err := newModel.Config()
	c.Assert(err, jc.ErrorIsNil)
	newAttrs := newConfig.AllAttrs()

	c.Assert(newAttrs["uuid"], gc.Equals, uuid)
	c.Assert(newAttrs["name"], gc.Equals, "new")

	// Now drop the uuid and name and the rest of the attributes should match.
	delete(newAttrs, "uuid")
	delete(newAttrs, "name")
	delete(originalAttrs, "uuid")
	delete(originalAttrs, "name")
	c.Assert(newAttrs, jc.DeepEquals, originalAttrs)

	newCons, err := newSt.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	// Can't test the constraints directly, so go through the string repr.
	c.Assert(newCons.String(), gc.Equals, cons.String())

	seq, err := state.Sequence(newSt, "machine")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(seq, gc.Equals, machineSeq)
	seq, err = state.Sequence(newSt, "application-foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(seq, gc.Equals, fooSeq)

	blocks, err := newSt.AllBlocks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, gc.HasLen, 1)
	c.Assert(blocks[0].Type(), gc.Equals, state.ChangeBlock)
	c.Assert(blocks[0].Message(), gc.Equals, "locked down")
}

func (s *MigrationImportSuite) newModelUser(c *gc.C, name string, readOnly bool, lastConnection time.Time) permission.UserAccess {
	access := permission.AdminAccess
	if readOnly {
		access = permission.ReadAccess
	}
	user, err := s.State.AddModelUser(s.State.ModelUUID(), state.UserAccessSpec{
		User:      names.NewUserTag(name),
		CreatedBy: s.Owner,
		Access:    access,
	})
	c.Assert(err, jc.ErrorIsNil)
	if !lastConnection.IsZero() {
		err = state.UpdateModelUserLastConnection(s.State, user, lastConnection)
		c.Assert(err, jc.ErrorIsNil)
	}
	return user
}

func (s *MigrationImportSuite) AssertUserEqual(c *gc.C, newUser, oldUser permission.UserAccess) {
	c.Assert(newUser.UserName, gc.Equals, oldUser.UserName)
	c.Assert(newUser.DisplayName, gc.Equals, oldUser.DisplayName)
	c.Assert(newUser.CreatedBy, gc.Equals, oldUser.CreatedBy)
	c.Assert(newUser.DateCreated, gc.Equals, oldUser.DateCreated)
	c.Assert(newUser.Access, gc.Equals, newUser.Access)

	connTime, err := s.State.LastModelConnection(oldUser.UserTag)
	if state.IsNeverConnectedError(err) {
		_, err := s.State.LastModelConnection(newUser.UserTag)
		// The new user should also return an error for last connection.
		c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	} else {
		c.Assert(err, jc.ErrorIsNil)
		newTime, err := s.State.LastModelConnection(newUser.UserTag)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(newTime, gc.Equals, connTime)
	}
}

func (s *MigrationImportSuite) TestModelUsers(c *gc.C) {
	// To be sure with this test, we create three env users, and remove
	// the owner.
	err := s.State.RemoveUserAccess(s.Owner, s.modelTag)
	c.Assert(err, jc.ErrorIsNil)

	lastConnection := state.NowToTheSecond()

	bravo := s.newModelUser(c, "bravo@external", false, lastConnection)
	charlie := s.newModelUser(c, "charlie@external", true, lastConnection)
	delta := s.newModelUser(c, "delta@external", true, time.Time{})

	newModel, newSt := s.importModel(c)

	// Check the import values of the users.
	for _, user := range []permission.UserAccess{bravo, charlie, delta} {
		newUser, err := newSt.UserAccess(user.UserTag, newModel.Tag())
		c.Assert(err, jc.ErrorIsNil)
		s.AssertUserEqual(c, newUser, user)
	}

	// Also make sure that there aren't any more.
	allUsers, err := newModel.Users()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allUsers, gc.HasLen, 3)
}

func (s *MigrationImportSuite) AssertMachineEqual(c *gc.C, newMachine, oldMachine *state.Machine) {
	c.Assert(newMachine.Id(), gc.Equals, oldMachine.Id())
	c.Assert(newMachine.Principals(), jc.DeepEquals, oldMachine.Principals())
	c.Assert(newMachine.Series(), gc.Equals, oldMachine.Series())
	c.Assert(newMachine.ContainerType(), gc.Equals, oldMachine.ContainerType())
	newHardware, err := newMachine.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	oldHardware, err := oldMachine.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newHardware, jc.DeepEquals, oldHardware)
	c.Assert(newMachine.Jobs(), jc.DeepEquals, oldMachine.Jobs())
	c.Assert(newMachine.Life(), gc.Equals, oldMachine.Life())
	newTools, err := newMachine.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	oldTools, err := oldMachine.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newTools, jc.DeepEquals, oldTools)
}

func (s *MigrationImportSuite) TestMachines(c *gc.C) {
	// Let's add a machine with an LXC container.
	cons := constraints.MustParse("arch=amd64 mem=8G")
	machine1 := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: cons,
	})
	err := s.State.SetAnnotations(machine1, testAnnotations)
	c.Assert(err, jc.ErrorIsNil)
	s.primeStatusHistory(c, machine1, status.Started, 5)

	// machine1 should have some instance data.
	hardware, err := machine1.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hardware, gc.NotNil)

	_ = s.Factory.MakeMachineNested(c, machine1.Id(), nil)

	allMachines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allMachines, gc.HasLen, 2)

	_, newSt := s.importModel(c)

	importedMachines, err := newSt.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedMachines, gc.HasLen, 2)

	// AllMachines returns the machines in the same order, yay us.
	for i, newMachine := range importedMachines {
		s.AssertMachineEqual(c, newMachine, allMachines[i])
	}

	// And a few extra checks.
	parent := importedMachines[0]
	container := importedMachines[1]
	containers, err := parent.Containers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containers, jc.DeepEquals, []string{container.Id()})
	parentId, isContainer := container.ParentId()
	c.Assert(parentId, gc.Equals, parent.Id())
	c.Assert(isContainer, jc.IsTrue)

	s.assertAnnotations(c, newSt, parent)
	s.checkStatusHistory(c, machine1, parent, 5)

	newCons, err := parent.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	// Can't test the constraints directly, so go through the string repr.
	c.Assert(newCons.String(), gc.Equals, cons.String())
}

func (s *MigrationImportSuite) TestMachineDevices(c *gc.C) {
	machine := s.Factory.MakeMachine(c, nil)
	// Create two devices, first with all fields set, second just to show that
	// we do both.
	sda := state.BlockDeviceInfo{
		DeviceName:     "sda",
		DeviceLinks:    []string{"some", "data"},
		Label:          "sda-label",
		UUID:           "some-uuid",
		HardwareId:     "magic",
		BusAddress:     "bus stop",
		Size:           16 * 1024 * 1024 * 1024,
		FilesystemType: "ext4",
		InUse:          true,
		MountPoint:     "/",
	}
	sdb := state.BlockDeviceInfo{DeviceName: "sdb", MountPoint: "/var/lib/lxd"}
	err := machine.SetMachineBlockDevices(sda, sdb)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c)

	imported, err := newSt.Machine(machine.Id())
	c.Assert(err, jc.ErrorIsNil)

	devices, err := newSt.BlockDevices(imported.MachineTag())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(devices, jc.DeepEquals, []state.BlockDeviceInfo{sda, sdb})
}

func (s *MigrationImportSuite) TestApplications(c *gc.C) {
	// Add a application with both settings and leadership settings.
	cons := constraints.MustParse("arch=amd64 mem=8G")
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Settings: map[string]interface{}{
			"foo": "bar",
		},
		Constraints: cons,
	})
	err := application.UpdateLeaderSettings(&goodToken{}, map[string]string{
		"leader": "true",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = application.SetMetricCredentials([]byte("sekrit"))
	c.Assert(err, jc.ErrorIsNil)
	// Expose the application.
	c.Assert(application.SetExposed(), jc.ErrorIsNil)
	err = s.State.SetAnnotations(application, testAnnotations)
	c.Assert(err, jc.ErrorIsNil)
	s.primeStatusHistory(c, application, status.Active, 5)

	allApplications, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allApplications, gc.HasLen, 1)

	_, newSt := s.importModel(c)

	importedApplications, err := newSt.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedApplications, gc.HasLen, 1)

	exported := allApplications[0]
	imported := importedApplications[0]

	c.Assert(imported.ApplicationTag(), gc.Equals, exported.ApplicationTag())
	c.Assert(imported.Series(), gc.Equals, exported.Series())
	c.Assert(imported.IsExposed(), gc.Equals, exported.IsExposed())
	c.Assert(imported.MetricCredentials(), jc.DeepEquals, exported.MetricCredentials())

	exportedConfig, err := exported.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	importedConfig, err := imported.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedConfig, jc.DeepEquals, exportedConfig)

	exportedLeaderSettings, err := exported.LeaderSettings()
	c.Assert(err, jc.ErrorIsNil)
	importedLeaderSettings, err := imported.LeaderSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedLeaderSettings, jc.DeepEquals, exportedLeaderSettings)

	s.assertAnnotations(c, newSt, imported)
	s.checkStatusHistory(c, application, imported, 5)

	newCons, err := imported.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	// Can't test the constraints directly, so go through the string repr.
	c.Assert(newCons.String(), gc.Equals, cons.String())
}

func (s *MigrationImportSuite) TestApplicationLeaders(c *gc.C) {
	s.makeApplicationWithLeader(c, "mysql", 2, 1)
	s.makeApplicationWithLeader(c, "wordpress", 4, 2)

	_, newSt := s.importModel(c)

	leaders := make(map[string]string)
	leases, err := state.LeadershipLeases(newSt)
	c.Assert(err, jc.ErrorIsNil)
	for key, value := range leases {
		leaders[key] = value.Holder
	}
	c.Assert(leaders, jc.DeepEquals, map[string]string{
		"mysql":     "mysql/1",
		"wordpress": "wordpress/2",
	})
}

func (s *MigrationImportSuite) TestUnits(c *gc.C) {
	s.assertUnitsMigrated(c, constraints.MustParse("arch=amd64 mem=8G"))
}

func (s *MigrationImportSuite) TestUnitsWithVirtConstraint(c *gc.C) {
	s.assertUnitsMigrated(c, constraints.MustParse("arch=amd64 mem=8G virt-type=kvm"))
}

func (s *MigrationImportSuite) assertUnitsMigrated(c *gc.C, cons constraints.Value) {
	exported, pwd := s.Factory.MakeUnitReturningPassword(c, &factory.UnitParams{
		Constraints: cons,
	})
	err := exported.SetMeterStatus("GREEN", "some info")
	c.Assert(err, jc.ErrorIsNil)
	err = exported.SetWorkloadVersion("amethyst")
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SetAnnotations(exported, testAnnotations)
	c.Assert(err, jc.ErrorIsNil)
	s.primeStatusHistory(c, exported, status.Active, 5)
	s.primeStatusHistory(c, exported.Agent(), status.Idle, 5)

	_, newSt := s.importModel(c)

	importedApplications, err := newSt.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedApplications, gc.HasLen, 1)

	importedUnits, err := importedApplications[0].AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedUnits, gc.HasLen, 1)
	imported := importedUnits[0]

	c.Assert(imported.UnitTag(), gc.Equals, exported.UnitTag())
	c.Assert(imported.PasswordValid(pwd), jc.IsTrue)
	version, err := imported.WorkloadVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(version, gc.Equals, "amethyst")

	exportedMachineId, err := exported.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	importedMachineId, err := imported.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedMachineId, gc.Equals, exportedMachineId)

	// Confirm machine Principals are set.
	exportedMachine, err := s.State.Machine(exportedMachineId)
	c.Assert(err, jc.ErrorIsNil)
	importedMachine, err := newSt.Machine(importedMachineId)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertMachineEqual(c, importedMachine, exportedMachine)

	meterStatus, err := imported.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meterStatus, gc.Equals, state.MeterStatus{state.MeterGreen, "some info"})
	s.assertAnnotations(c, newSt, imported)
	s.checkStatusHistory(c, exported, imported, 5)
	s.checkStatusHistory(c, exported.Agent(), imported.Agent(), 5)
	s.checkStatusHistory(c, exported.WorkloadVersionHistory(), imported.WorkloadVersionHistory(), 1)

	newCons, err := imported.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	// Can't test the constraints directly, so go through the string repr.
	c.Assert(newCons.String(), gc.Equals, cons.String())
}

func (s *MigrationImportSuite) TestRelations(c *gc.C) {
	wordpress := state.AddTestingService(c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	state.AddTestingService(c, s.State, "mysql", state.AddTestingCharm(c, s.State, "mysql"))
	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	wordpress_0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})

	ru, err := rel.Unit(wordpress_0)
	c.Assert(err, jc.ErrorIsNil)
	relSettings := map[string]interface{}{
		"name": "wordpress/0",
	}
	err = ru.EnterScope(relSettings)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c)

	newWordpress, err := newSt.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(state.RelationCount(newWordpress), gc.Equals, 1)
	rels, err := newWordpress.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rels, gc.HasLen, 1)
	units, err := newWordpress.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)

	ru, err = rels[0].Unit(units[0])
	c.Assert(err, jc.ErrorIsNil)

	settings, err := ru.Settings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings.Map(), gc.DeepEquals, relSettings)
}

func (s *MigrationImportSuite) TestUnitsOpenPorts(c *gc.C) {
	unit := s.Factory.MakeUnit(c, nil)
	err := unit.OpenPorts("tcp", 1234, 2345)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c)

	// Even though the opened ports document is stored with the
	// machine, the only way to easily access it is through the units.
	imported, err := newSt.Unit(unit.Name())
	c.Assert(err, jc.ErrorIsNil)

	ports, err := imported.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, 1)
	c.Assert(ports[0], gc.Equals, network.PortRange{
		FromPort: 1234,
		ToPort:   2345,
		Protocol: "tcp",
	})
}

func (s *MigrationImportSuite) TestSpaces(c *gc.C) {
	space := s.Factory.MakeSpace(c, &factory.SpaceParams{
		Name: "one", ProviderID: network.Id("provider"), IsPublic: true})

	_, newSt := s.importModel(c)

	imported, err := newSt.Space(space.Name())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(imported.Name(), gc.Equals, space.Name())
	c.Assert(imported.ProviderId(), gc.Equals, space.ProviderId())
	c.Assert(imported.IsPublic(), gc.Equals, space.IsPublic())
}

func (s *MigrationImportSuite) TestDestroyEmptyModel(c *gc.C) {
	newModel, _ := s.importModel(c)
	s.assertDestroyModelAdvancesLife(c, newModel, state.Dead)
}

func (s *MigrationImportSuite) TestDestroyModelWithMachine(c *gc.C) {
	s.Factory.MakeMachine(c, nil)
	newModel, _ := s.importModel(c)
	s.assertDestroyModelAdvancesLife(c, newModel, state.Dying)
}

func (s *MigrationImportSuite) TestDestroyModelWithApplication(c *gc.C) {
	s.Factory.MakeApplication(c, nil)
	newModel, _ := s.importModel(c)
	s.assertDestroyModelAdvancesLife(c, newModel, state.Dying)
}

func (s *MigrationImportSuite) assertDestroyModelAdvancesLife(c *gc.C, m *state.Model, life state.Life) {
	err := m.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = m.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, life)
}

func (s *MigrationImportSuite) TestLinkLayerDevice(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	deviceArgs := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: state.EthernetDevice,
	}
	err := machine.SetLinkLayerDevices(deviceArgs)
	c.Assert(err, jc.ErrorIsNil)
	_, newSt := s.importModel(c)

	devices, err := newSt.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, gc.HasLen, 1)
	device := devices[0]
	c.Assert(device.Name(), gc.Equals, "foo")
	c.Assert(device.Type(), gc.Equals, state.EthernetDevice)
}

func (s *MigrationImportSuite) TestLinkLayerDeviceMigratesReferences(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	machine2 := s.Factory.MakeMachineNested(c, machine.Id(), &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	deviceArgs := []state.LinkLayerDeviceArgs{{
		Name: "foo",
		Type: state.BridgeDevice,
	}, {
		Name:       "bar",
		ParentName: "foo",
		Type:       state.EthernetDevice,
	}}
	for _, args := range deviceArgs {
		err := machine.SetLinkLayerDevices(args)
		c.Assert(err, jc.ErrorIsNil)
	}
	machine2DeviceArgs := state.LinkLayerDeviceArgs{
		Name:       "baz",
		ParentName: fmt.Sprintf("m#%v#d#foo", machine.Id()),
		Type:       state.EthernetDevice,
	}
	err := machine2.SetLinkLayerDevices(machine2DeviceArgs)
	c.Assert(err, jc.ErrorIsNil)
	_, newSt := s.importModel(c)

	devices, err := newSt.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, gc.HasLen, 3)
	var parent *state.LinkLayerDevice
	others := []*state.LinkLayerDevice{}
	for _, device := range devices {
		if device.Name() == "foo" {
			parent = device
		} else {
			others = append(others, device)
		}
	}
	// Assert we found the parent.
	c.Assert(others, gc.HasLen, 2)
	err = parent.Remove()
	c.Assert(err, gc.ErrorMatches, `.*parent device "foo" has 2 children.*`)
	err = others[0].Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = parent.Remove()
	c.Assert(err, gc.ErrorMatches, `.*parent device "foo" has 1 children.*`)
	err = others[1].Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = parent.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationImportSuite) TestSubnets(c *gc.C) {
	original, err := s.State.AddSubnet(state.SubnetInfo{
		CIDR:             "10.0.0.0/24",
		ProviderId:       network.Id("foo"),
		VLANTag:          64,
		AvailabilityZone: "bar",
		SpaceName:        "bam",
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("bam", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c)

	subnet, err := newSt.Subnet(original.CIDR())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnet.CIDR(), gc.Equals, "10.0.0.0/24")
	c.Assert(subnet.ProviderId(), gc.Equals, network.Id("foo"))
	c.Assert(subnet.VLANTag(), gc.Equals, 64)
	c.Assert(subnet.AvailabilityZone(), gc.Equals, "bar")
	c.Assert(subnet.SpaceName(), gc.Equals, "bam")
}

func (s *MigrationImportSuite) TestIPAddress(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	_, err := s.State.AddSubnet(state.SubnetInfo{CIDR: "0.1.2.0/24"})
	c.Assert(err, jc.ErrorIsNil)
	deviceArgs := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: state.EthernetDevice,
	}
	err = machine.SetLinkLayerDevices(deviceArgs)
	c.Assert(err, jc.ErrorIsNil)
	args := state.LinkLayerDeviceAddress{
		DeviceName:       "foo",
		ConfigMethod:     state.StaticAddress,
		CIDRAddress:      "0.1.2.3/24",
		ProviderID:       "bar",
		DNSServers:       []string{"bam", "mam"},
		DNSSearchDomains: []string{"weeee"},
		GatewayAddress:   "0.1.2.1",
	}
	err = machine.SetDevicesAddresses(args)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c)

	addresses, _ := newSt.AllIPAddresses()
	c.Assert(addresses, gc.HasLen, 1)
	c.Assert(err, jc.ErrorIsNil)
	addr := addresses[0]
	c.Assert(addr.Value(), gc.Equals, "0.1.2.3")
	c.Assert(addr.MachineID(), gc.Equals, machine.Id())
	c.Assert(addr.DeviceName(), gc.Equals, "foo")
	c.Assert(addr.ConfigMethod(), gc.Equals, state.StaticAddress)
	c.Assert(addr.SubnetCIDR(), gc.Equals, "0.1.2.0/24")
	c.Assert(addr.ProviderID(), gc.Equals, network.Id("bar"))
	c.Assert(addr.DNSServers(), jc.DeepEquals, []string{"bam", "mam"})
	c.Assert(addr.DNSSearchDomains(), jc.DeepEquals, []string{"weeee"})
	c.Assert(addr.GatewayAddress(), gc.Equals, "0.1.2.1")
}

func (s *MigrationImportSuite) TestSSHHostKey(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	err := s.State.SetSSHHostKeys(machine.MachineTag(), []string{"bam", "mam"})
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c)

	machine2, err := newSt.Machine(machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	keys, err := newSt.GetSSHHostKeys(machine2.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, jc.DeepEquals, state.SSHHostKeys{"bam", "mam"})
}

func (s *MigrationImportSuite) TestCloudImageMetadata(c *gc.C) {
	storageSize := uint64(3)
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region-test",
		Version:         "14.04",
		Series:          "trusty",
		Arch:            "arch",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test",
		RootStorageSize: &storageSize,
		Source:          "test",
	}
	metadata := []cloudimagemetadata.Metadata{{attrs, 2, "1", 2}}

	err := s.State.CloudImageMetadataStorage.SaveMetadata(metadata)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c)
	defer func() {
		c.Assert(newSt.Close(), jc.ErrorIsNil)
	}()

	images, err := s.State.CloudImageMetadataStorage.AllCloudImageMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(images, gc.HasLen, 1)
	image := images[0]
	c.Check(image.Stream, gc.Equals, "stream")
	c.Check(image.Region, gc.Equals, "region-test")
	c.Check(image.Version, gc.Equals, "14.04")
	c.Check(image.Arch, gc.Equals, "arch")
	c.Check(image.VirtType, gc.Equals, "virtType-test")
	c.Check(image.RootStorageType, gc.Equals, "rootStorageType-test")
	c.Check(*image.RootStorageSize, gc.Equals, uint64(3))
	c.Check(image.Source, gc.Equals, "test")
	c.Check(image.Priority, gc.Equals, 2)
	c.Check(image.ImageId, gc.Equals, "1")
	c.Check(image.DateCreated, gc.Equals, int64(2))
}

func (s *MigrationImportSuite) TestAction(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	_, err := s.State.EnqueueAction(machine.MachineTag(), "foo", nil)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c)
	defer func() {
		c.Assert(newSt.Close(), jc.ErrorIsNil)
	}()

	actions, _ := newSt.AllActions()
	c.Assert(actions, gc.HasLen, 1)
	action := actions[0]
	c.Check(action.Receiver(), gc.Equals, machine.Id())
	c.Check(action.Name(), gc.Equals, "foo")
	c.Check(action.Status(), gc.Equals, state.ActionPending)
}

func (s *MigrationImportSuite) TestVolumes(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Volumes: []state.MachineVolumeParams{{
			Volume:     state.VolumeParams{Size: 1234},
			Attachment: state.VolumeAttachmentParams{ReadOnly: true},
		}, {
			Volume:     state.VolumeParams{Size: 4000},
			Attachment: state.VolumeAttachmentParams{ReadOnly: true},
		}},
	})
	machineTag := machine.MachineTag()

	// We know that the first volume is called "0/0" - although I don't know why.
	volTag := names.NewVolumeTag("0/0")
	volInfo := state.VolumeInfo{
		HardwareId: "magic",
		Size:       1500,
		Pool:       "loop",
		VolumeId:   "volume id",
		Persistent: true,
	}
	err := s.State.SetVolumeInfo(volTag, volInfo)
	c.Assert(err, jc.ErrorIsNil)
	volAttachmentInfo := state.VolumeAttachmentInfo{
		DeviceName: "device name",
		DeviceLink: "device link",
		BusAddress: "bus address",
		ReadOnly:   true,
	}
	err = s.State.SetVolumeAttachmentInfo(machineTag, volTag, volAttachmentInfo)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c)

	volume, err := newSt.Volume(volTag)
	c.Assert(err, jc.ErrorIsNil)

	// TODO: check status
	// TODO: check storage instance
	info, err := volume.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(info, jc.DeepEquals, volInfo)

	attachment, err := newSt.VolumeAttachment(machineTag, volTag)
	c.Assert(err, jc.ErrorIsNil)
	attInfo, err := attachment.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(attInfo, jc.DeepEquals, volAttachmentInfo)

	volTag = names.NewVolumeTag("0/1")
	volume, err = newSt.Volume(volTag)
	c.Assert(err, jc.ErrorIsNil)

	params, needsProvisioning := volume.Params()
	c.Check(needsProvisioning, jc.IsTrue)
	c.Check(params.Pool, gc.Equals, "loop")
	c.Check(params.Size, gc.Equals, uint64(4000))

	attachment, err = newSt.VolumeAttachment(machineTag, volTag)
	c.Assert(err, jc.ErrorIsNil)
	attParams, needsProvisioning := attachment.Params()
	c.Check(needsProvisioning, jc.IsTrue)
	c.Check(attParams.ReadOnly, jc.IsTrue)
}

func (s *MigrationImportSuite) TestFilesystems(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Filesystems: []state.MachineFilesystemParams{{
			Filesystem: state.FilesystemParams{Size: 1234},
			Attachment: state.FilesystemAttachmentParams{
				Location: "location",
				ReadOnly: true},
		}, {
			Filesystem: state.FilesystemParams{Size: 4000},
			Attachment: state.FilesystemAttachmentParams{
				ReadOnly: true},
		}},
	})
	machineTag := machine.MachineTag()

	// We know that the first filesystem is called "0/0" as it is the first
	// filesystem (filesystems use sequences), and it is bound to machine 0.
	fsTag := names.NewFilesystemTag("0/0")
	fsInfo := state.FilesystemInfo{
		Size:         1500,
		Pool:         "rootfs",
		FilesystemId: "filesystem id",
	}
	err := s.State.SetFilesystemInfo(fsTag, fsInfo)
	c.Assert(err, jc.ErrorIsNil)
	fsAttachmentInfo := state.FilesystemAttachmentInfo{
		MountPoint: "/mnt/foo",
		ReadOnly:   true,
	}
	err = s.State.SetFilesystemAttachmentInfo(machineTag, fsTag, fsAttachmentInfo)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c)

	filesystem, err := newSt.Filesystem(fsTag)
	c.Assert(err, jc.ErrorIsNil)

	// TODO: check status
	// TODO: check storage instance
	info, err := filesystem.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(info, jc.DeepEquals, fsInfo)

	attachment, err := newSt.FilesystemAttachment(machineTag, fsTag)
	c.Assert(err, jc.ErrorIsNil)
	attInfo, err := attachment.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(attInfo, jc.DeepEquals, fsAttachmentInfo)

	fsTag = names.NewFilesystemTag("0/1")
	filesystem, err = newSt.Filesystem(fsTag)
	c.Assert(err, jc.ErrorIsNil)

	params, needsProvisioning := filesystem.Params()
	c.Check(needsProvisioning, jc.IsTrue)
	c.Check(params.Pool, gc.Equals, "rootfs")
	c.Check(params.Size, gc.Equals, uint64(4000))

	attachment, err = newSt.FilesystemAttachment(machineTag, fsTag)
	c.Assert(err, jc.ErrorIsNil)
	attParams, needsProvisioning := attachment.Params()
	c.Check(needsProvisioning, jc.IsTrue)
	c.Check(attParams.ReadOnly, jc.IsTrue)
}

func (s *MigrationImportSuite) TestStorage(c *gc.C) {
	app, u, storageTag := s.makeUnitWithStorage(c)
	original, err := s.State.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	originalCount := state.StorageAttachmentCount(original)
	c.Assert(originalCount, gc.Equals, 1)
	originalAttachments, err := s.State.StorageAttachments(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(originalAttachments, gc.HasLen, 1)
	c.Assert(originalAttachments[0].Unit(), gc.Equals, u.UnitTag())
	appName := app.Name()

	_, newSt := s.importModel(c)

	app, err = newSt.Application(appName)
	c.Assert(err, jc.ErrorIsNil)
	cons, err := app.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cons, jc.DeepEquals, map[string]state.StorageConstraints{
		"data":    {Pool: "loop-pool", Size: 0x400, Count: 1},
		"allecto": {Pool: "loop", Size: 0x400},
	})

	instance, err := newSt.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(instance.Tag(), gc.Equals, original.Tag())
	c.Check(instance.Kind(), gc.Equals, original.Kind())
	c.Check(instance.Life(), gc.Equals, original.Life())
	c.Check(instance.StorageName(), gc.Equals, original.StorageName())
	c.Check(state.StorageAttachmentCount(instance), gc.Equals, originalCount)

	attachments, err := newSt.StorageAttachments(storageTag)

	c.Assert(attachments, gc.HasLen, 1)
	c.Assert(attachments[0].Unit(), gc.Equals, u.UnitTag())
}

func (s *MigrationImportSuite) TestStoragePools(c *gc.C) {
	pm := poolmanager.New(state.NewStateSettings(s.State), provider.CommonStorageProviders())
	_, err := pm.Create("test-pool", provider.LoopProviderType, map[string]interface{}{
		"value": 42,
	})
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c)

	pm = poolmanager.New(state.NewStateSettings(newSt), provider.CommonStorageProviders())
	pools, err := pm.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 1)

	pool := pools[0]
	c.Assert(pool.Name(), gc.Equals, "test-pool")
	c.Assert(pool.Provider(), gc.Equals, provider.LoopProviderType)
	c.Assert(pool.Attrs(), jc.DeepEquals, map[string]interface{}{
		"value": 42,
	})
}

func (s *MigrationImportSuite) TestPayloads(c *gc.C) {
	originalUnit := s.Factory.MakeUnit(c, nil)
	unitID := originalUnit.UnitTag().Id()
	up, err := s.State.UnitPayloads(originalUnit)
	c.Assert(err, jc.ErrorIsNil)
	original := payload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "something",
			Type: "special",
		},
		ID:     "42",
		Status: "running",
		Labels: []string{"foo", "bar"},
	}
	err = up.Track(original)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c)

	unit, err := newSt.Unit(unitID)
	c.Assert(err, jc.ErrorIsNil)

	up, err = newSt.UnitPayloads(unit)
	c.Assert(err, jc.ErrorIsNil)

	result, err := up.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result[0].Payload, gc.NotNil)

	payload := result[0].Payload

	machineID, err := unit.AssignedMachineId()
	c.Check(err, jc.ErrorIsNil)
	c.Check(payload.Name, gc.Equals, original.Name)
	c.Check(payload.Type, gc.Equals, original.Type)
	c.Check(payload.ID, gc.Equals, original.ID)
	c.Check(payload.Status, gc.Equals, original.Status)
	c.Check(payload.Labels, jc.DeepEquals, original.Labels)
	c.Check(payload.Unit, gc.Equals, unitID)
	c.Check(payload.Machine, gc.Equals, machineID)
}

// newModel replaces the uuid and name of the config attributes so we
// can use all the other data to validate imports. An owner and name of the
// model are unique together in a controller.
func newModel(m description.Model, uuid, name string) description.Model {
	return &mockModel{m, uuid, name}
}

type mockModel struct {
	description.Model
	uuid string
	name string
}

func (m *mockModel) Tag() names.ModelTag {
	return names.NewModelTag(m.uuid)
}

func (m *mockModel) Config() map[string]interface{} {
	c := m.Model.Config()
	c["uuid"] = m.uuid
	c["name"] = m.name
	return c
}
