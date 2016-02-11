// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/migration"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/version"
)

type MigrationImportSuite struct {
	MigrationSuite
}

var _ = gc.Suite(&MigrationImportSuite{})

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

	newEnv, newSt, err := s.State.Import(in)
	c.Assert(err, jc.ErrorIsNil)
	return newEnv, newSt
}

func (s *MigrationImportSuite) TestNewEnv(c *gc.C) {
	latestTools := version.MustParse("2.0.1")
	s.setLatestTools(c, latestTools)

	out, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	uuid := utils.MustNewUUID().String()
	in := newModel(out, uuid, "new")

	newEnv, newSt, err := s.State.Import(in)
	c.Assert(err, jc.ErrorIsNil)
	defer newSt.Close()

	original, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(newEnv.Owner(), gc.Equals, original.Owner())
	c.Assert(newEnv.LatestToolsVersion(), gc.Equals, latestTools)
	originalConfig, err := original.Config()
	c.Assert(err, jc.ErrorIsNil)
	originalAttrs := originalConfig.AllAttrs()

	newConfig, err := newEnv.Config()
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
}

func (s *MigrationImportSuite) newModelUser(c *gc.C, name string, readOnly bool, lastConnection time.Time) *state.ModelUser {
	user, err := s.State.AddModelUser(state.ModelUserSpec{
		User:      names.NewUserTag(name),
		CreatedBy: s.Owner,
		ReadOnly:  readOnly,
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

	newEnv, newSt := s.importModel(c)
	defer newSt.Close()

	// Check the import values of the users.
	for _, user := range []*state.ModelUser{bravo, charlie, delta} {
		newUser, err := newSt.ModelUser(user.UserTag())
		c.Assert(err, jc.ErrorIsNil)
		s.AssertUserEqual(c, newUser, user)
	}

	// Also make sure that there aren't any more.
	allUsers, err := newEnv.Users()
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
	machine1 := s.Factory.MakeMachine(c, nil)

	// machine1 should have some instance data.
	hardware, err := machine1.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hardware, gc.NotNil)

	_ = s.Factory.MakeMachineNested(c, machine1.Id(), nil)

	allMachines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allMachines, gc.HasLen, 2)

	_, newSt := s.importModel(c)
	defer newSt.Close()

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
}

func (s *MigrationImportSuite) TestServices(c *gc.C) {
	// Add a service with both settings and leadership settings.
	service := s.Factory.MakeService(c, &factory.ServiceParams{
		Settings: map[string]interface{}{
			"foo": "bar",
		},
	})
	err := service.UpdateLeaderSettings(&goodToken{}, map[string]string{
		"leader": "true",
	})
	c.Assert(err, jc.ErrorIsNil)
	// Expose the service.
	c.Assert(service.SetExposed(), jc.ErrorIsNil)

	allServices, err := s.State.AllServices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allServices, gc.HasLen, 1)

	_, newSt := s.importModel(c)
	defer newSt.Close()

	importedServices, err := newSt.AllServices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedServices, gc.HasLen, 1)

	exported := allServices[0]
	imported := importedServices[0]

	c.Assert(imported.ServiceTag(), gc.Equals, exported.ServiceTag())
	c.Assert(imported.Series(), gc.Equals, exported.Series())
	c.Assert(imported.IsExposed(), gc.Equals, exported.IsExposed())

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
}

func (s *MigrationImportSuite) TestUnits(c *gc.C) {
	exported, pwd := s.Factory.MakeUnitReturningPassword(c, nil)

	_, newSt := s.importModel(c)
	defer newSt.Close()

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
}

func (s *MigrationImportSuite) TestUnitsOpenPorts(c *gc.C) {
	unit := s.Factory.MakeUnit(c, nil)
	err := unit.OpenPorts("tcp", 1234, 2345)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c)
	defer newSt.Close()

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

// newModel replaces the uuid and name of the config attributes so we
// can use all the other data to validate imports. An owner and name of the
// model are unique together in a controller.
func newModel(m migration.Model, uuid, name string) migration.Model {
	return &mockModel{m, uuid, name}
}

type mockModel struct {
	migration.Model
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
