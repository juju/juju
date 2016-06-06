// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	"github.com/juju/juju/testing/factory"
)

type MigrationImportSuite struct {
	MigrationSuite
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
	fooSeq := s.setRandSequenceValue(c, "service-foo")
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
	seq, err = state.Sequence(newSt, "service-foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(seq, gc.Equals, fooSeq)

	blocks, err := newSt.AllBlocks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, gc.HasLen, 1)
	c.Assert(blocks[0].Type(), gc.Equals, state.ChangeBlock)
	c.Assert(blocks[0].Message(), gc.Equals, "locked down")
}

func (s *MigrationImportSuite) newModelUser(c *gc.C, name string, readOnly bool, lastConnection time.Time) *state.ModelUser {
	access := state.ModelAdminAccess
	if readOnly {
		access = state.ModelReadAccess
	}
	user, err := s.State.AddModelUser(state.ModelUserSpec{
		User:      names.NewUserTag(name),
		CreatedBy: s.Owner,
		Access:    access,
	})
	c.Assert(err, jc.ErrorIsNil)
	if !lastConnection.IsZero() {
		err = state.UpdateModelUserLastConnection(user, lastConnection)
		c.Assert(err, jc.ErrorIsNil)
	}
	return user
}

func (s *MigrationImportSuite) AssertUserEqual(c *gc.C, newUser, oldUser *state.ModelUser) {
	c.Assert(newUser.UserName(), gc.Equals, oldUser.UserName())
	c.Assert(newUser.DisplayName(), gc.Equals, oldUser.DisplayName())
	c.Assert(newUser.CreatedBy(), gc.Equals, oldUser.CreatedBy())
	c.Assert(newUser.DateCreated(), gc.Equals, oldUser.DateCreated())
	c.Assert(newUser.ReadOnly(), gc.Equals, oldUser.ReadOnly())

	connTime, err := oldUser.LastConnection()
	if state.IsNeverConnectedError(err) {
		_, err := newUser.LastConnection()
		// The new user should also return an error for last connection.
		c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	} else {
		c.Assert(err, jc.ErrorIsNil)
		newTime, err := newUser.LastConnection()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(newTime, gc.Equals, connTime)
	}
}

func (s *MigrationImportSuite) TestModelUsers(c *gc.C) {
	// To be sure with this test, we create three env users, and remove
	// the owner.
	err := s.State.RemoveModelUser(s.Owner)
	c.Assert(err, jc.ErrorIsNil)

	lastConnection := state.NowToTheSecond()

	bravo := s.newModelUser(c, "bravo@external", false, lastConnection)
	charlie := s.newModelUser(c, "charlie@external", true, lastConnection)
	delta := s.newModelUser(c, "delta@external", true, time.Time{})

	newModel, newSt := s.importModel(c)

	// Check the import values of the users.
	for _, user := range []*state.ModelUser{bravo, charlie, delta} {
		newUser, err := newSt.ModelUser(user.UserTag())
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
	s.primeStatusHistory(c, machine1, status.StatusStarted, 5)

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

func (s *MigrationImportSuite) TestServices(c *gc.C) {
	// Add a service with both settings and leadership settings.
	cons := constraints.MustParse("arch=amd64 mem=8G")
	service := s.Factory.MakeService(c, &factory.ServiceParams{
		Settings: map[string]interface{}{
			"foo": "bar",
		},
		Constraints: cons,
	})
	err := service.UpdateLeaderSettings(&goodToken{}, map[string]string{
		"leader": "true",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = service.SetMetricCredentials([]byte("sekrit"))
	c.Assert(err, jc.ErrorIsNil)
	// Expose the service.
	c.Assert(service.SetExposed(), jc.ErrorIsNil)
	err = s.State.SetAnnotations(service, testAnnotations)
	c.Assert(err, jc.ErrorIsNil)
	s.primeStatusHistory(c, service, status.StatusActive, 5)

	allServices, err := s.State.AllServices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allServices, gc.HasLen, 1)

	_, newSt := s.importModel(c)

	importedServices, err := newSt.AllServices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedServices, gc.HasLen, 1)

	exported := allServices[0]
	imported := importedServices[0]

	c.Assert(imported.ServiceTag(), gc.Equals, exported.ServiceTag())
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
	s.checkStatusHistory(c, service, imported, 5)

	newCons, err := imported.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	// Can't test the constraints directly, so go through the string repr.
	c.Assert(newCons.String(), gc.Equals, cons.String())
}

func (s *MigrationImportSuite) TestServiceLeaders(c *gc.C) {
	s.makeServiceWithLeader(c, "mysql", 2, 1)
	s.makeServiceWithLeader(c, "wordpress", 4, 2)

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
	cons := constraints.MustParse("arch=amd64 mem=8G")
	exported, pwd := s.Factory.MakeUnitReturningPassword(c, &factory.UnitParams{
		Constraints: cons,
	})
	err := exported.SetMeterStatus("GREEN", "some info")
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SetAnnotations(exported, testAnnotations)
	c.Assert(err, jc.ErrorIsNil)
	s.primeStatusHistory(c, exported, status.StatusActive, 5)
	s.primeStatusHistory(c, exported.Agent(), status.StatusIdle, 5)

	_, newSt := s.importModel(c)

	importedServices, err := newSt.AllServices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedServices, gc.HasLen, 1)

	importedUnits, err := importedServices[0].AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedUnits, gc.HasLen, 1)
	imported := importedUnits[0]

	c.Assert(imported.UnitTag(), gc.Equals, exported.UnitTag())
	c.Assert(imported.PasswordValid(pwd), jc.IsTrue)

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

	newCons, err := imported.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	// Can't test the constraints directly, so go through the string repr.
	c.Assert(newCons.String(), gc.Equals, cons.String())
}

func (s *MigrationImportSuite) TestRelations(c *gc.C) {
	// Need to remove owner from service.
	ignored := s.Owner
	wordpress := state.AddTestingService(c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"), ignored)
	state.AddTestingService(c, s.State, "mysql", state.AddTestingCharm(c, s.State, "mysql"), ignored)
	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	wordpress_0 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: wordpress})

	ru, err := rel.Unit(wordpress_0)
	c.Assert(err, jc.ErrorIsNil)
	relSettings := map[string]interface{}{
		"name": "wordpress/0",
	}
	err = ru.EnterScope(relSettings)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c)

	newWordpress, err := newSt.Service("wordpress")
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

func (s *MigrationImportSuite) TestDestroyModelWithService(c *gc.C) {
	s.Factory.MakeService(c, nil)
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
	defer func() {
		c.Assert(newSt.Close(), jc.ErrorIsNil)
	}()

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
