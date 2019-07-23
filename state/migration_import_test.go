// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"strconv"
	"time" // only uses time.Time values

	"github.com/juju/description"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/network"
	"github.com/juju/juju/payload"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
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

	_, _, err = s.Controller.Import(out)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *MigrationImportSuite) importModel(c *gc.C, st *state.State, transform ...func(map[string]interface{})) (*state.Model, *state.State) {
	out, err := st.Export()
	c.Assert(err, jc.ErrorIsNil)

	if len(transform) > 0 {
		var outM map[string]interface{}
		outYaml, err := description.Serialize(out)
		c.Assert(err, jc.ErrorIsNil)
		err = yaml.Unmarshal(outYaml, &outM)
		c.Assert(err, jc.ErrorIsNil)

		for _, transform := range transform {
			transform(outM)
		}

		outYaml, err = yaml.Marshal(outM)
		c.Assert(err, jc.ErrorIsNil)
		out, err = description.Deserialize(outYaml)
		c.Assert(err, jc.ErrorIsNil)
	}

	uuid := utils.MustNewUUID().String()
	in := newModel(out, uuid, "new")

	newModel, newSt, err := s.Controller.Import(in)
	c.Assert(err, jc.ErrorIsNil)
	// add the cleanup here to close the model.
	s.AddCleanup(func(c *gc.C) {
		c.Check(newSt.Close(), jc.ErrorIsNil)
	})
	return newModel, newSt
}

func (s *MigrationImportSuite) assertAnnotations(c *gc.C, model *state.Model, entity state.GlobalEntity) {
	annotations, err := model.Annotations(entity)
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

	environVersion := 123
	err = original.SetEnvironVersion(environVersion)
	c.Assert(err, jc.ErrorIsNil)

	err = s.Model.SetAnnotations(original, testAnnotations)
	c.Assert(err, jc.ErrorIsNil)

	out, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	uuid := utils.MustNewUUID().String()
	in := newModel(out, uuid, "new")

	newModel, newSt, err := s.Controller.Import(in)
	c.Assert(err, jc.ErrorIsNil)
	defer newSt.Close()

	c.Assert(newModel.Type(), gc.Equals, original.Type())
	c.Assert(newModel.Owner(), gc.Equals, original.Owner())
	c.Assert(newModel.LatestToolsVersion(), gc.Equals, latestTools)
	c.Assert(newModel.MigrationMode(), gc.Equals, state.MigrationModeImporting)
	c.Assert(newModel.EnvironVersion(), gc.Equals, environVersion)
	s.assertAnnotations(c, newModel, newModel)

	statusInfo, err := newModel.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statusInfo.Status, gc.Equals, status.Busy)
	c.Check(statusInfo.Message, gc.Equals, "importing")
	// One for original "available", one for "busy (importing)"
	history, err := newModel.StatusHistory(status.StatusHistoryFilter{Size: 5})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(history, gc.HasLen, 2)
	c.Check(history[0].Status, gc.Equals, status.Busy)
	c.Check(history[1].Status, gc.Equals, status.Available)

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
	user, err := s.Model.AddUser(state.UserAccessSpec{
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

	connTime, err := s.Model.LastModelConnection(oldUser.UserTag)
	if state.IsNeverConnectedError(err) {
		_, err := s.Model.LastModelConnection(newUser.UserTag)
		// The new user should also return an error for last connection.
		c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	} else {
		c.Assert(err, jc.ErrorIsNil)
		newTime, err := s.Model.LastModelConnection(newUser.UserTag)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(newTime, gc.Equals, connTime)
	}
}

func (s *MigrationImportSuite) TestModelUsers(c *gc.C) {
	// To be sure with this test, we create three env users, and remove
	// the owner.
	err := s.State.RemoveUserAccess(s.Owner, s.modelTag)
	c.Assert(err, jc.ErrorIsNil)

	lastConnection := state.NowToTheSecond(s.State)

	bravo := s.newModelUser(c, "bravo@external", false, lastConnection)
	charlie := s.newModelUser(c, "charlie@external", true, lastConnection)
	delta := s.newModelUser(c, "delta@external", true, coretesting.ZeroTime())

	newModel, newSt := s.importModel(c, s.State)

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

func (s *MigrationImportSuite) TestSLA(c *gc.C) {
	err := s.State.SetSLA("essential", "bob", []byte("creds"))
	c.Assert(err, jc.ErrorIsNil)
	newModel, newSt := s.importModel(c, s.State)

	c.Assert(newModel.SLALevel(), gc.Equals, "essential")
	c.Assert(newModel.SLACredential(), jc.DeepEquals, []byte("creds"))
	level, err := newSt.SLALevel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(level, gc.Equals, "essential")
	creds, err := newSt.SLACredential()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, []byte("creds"))
}

func (s *MigrationImportSuite) TestMeterStatus(c *gc.C) {
	err := s.State.SetModelMeterStatus("RED", "info message")
	c.Assert(err, jc.ErrorIsNil)
	newModel, newSt := s.importModel(c, s.State)

	ms := newModel.MeterStatus()
	c.Assert(ms.Code.String(), gc.Equals, "RED")
	c.Assert(ms.Info, gc.Equals, "info message")
	ms, err = newSt.ModelMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ms.Code.String(), gc.Equals, "RED")
	c.Assert(ms.Info, gc.Equals, "info message")
}

func (s *MigrationImportSuite) TestMeterStatusNotAvailable(c *gc.C) {
	newModel, newSt := s.importModel(c, s.State, func(desc map[string]interface{}) {
		c.Log(desc["meter-status"])
		desc["meter-status"].(map[interface{}]interface{})["code"] = ""
	})

	ms := newModel.MeterStatus()
	c.Assert(ms.Code.String(), gc.Equals, "NOT AVAILABLE")
	c.Assert(ms.Info, gc.Equals, "")
	ms, err := newSt.ModelMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ms.Code.String(), gc.Equals, "NOT AVAILABLE")
	c.Assert(ms.Info, gc.Equals, "")
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

	oldStatus, err := oldMachine.Status()
	c.Assert(err, jc.ErrorIsNil)
	newStatus, err := newMachine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newStatus, jc.DeepEquals, oldStatus)

	oldInstID, err := oldMachine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	newInstID, err := newMachine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newInstID, gc.Equals, oldInstID)

	oldStatus, err = oldMachine.InstanceStatus()
	c.Assert(err, jc.ErrorIsNil)
	newStatus, err = newMachine.InstanceStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newStatus, jc.DeepEquals, oldStatus)
}

func (s *MigrationImportSuite) TestMachines(c *gc.C) {
	// Let's add a machine with an LXC container.
	cons := constraints.MustParse("arch=amd64 mem=8G root-disk-source=bunyan")
	source := "bunyan"
	machine1 := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: cons,
		Characteristics: &instance.HardwareCharacteristics{
			RootDiskSource: &source,
		},
	})
	err := s.Model.SetAnnotations(machine1, testAnnotations)
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

	newModel, newSt := s.importModel(c, s.State)

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

	s.assertAnnotations(c, newModel, parent)
	s.checkStatusHistory(c, machine1, parent, 5)

	newCons, err := parent.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	// Can't test the constraints directly, so go through the string repr.
	c.Assert(newCons.String(), gc.Equals, cons.String())

	// Test the modification status is set to the initial state.
	modStatus, err := parent.ModificationStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modStatus.Status, gc.Equals, status.Idle)

	characteristics, err := parent.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*characteristics.RootDiskSource, gc.Equals, "bunyan")
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
		WWN:            "drbr",
		BusAddress:     "bus stop",
		Size:           16 * 1024 * 1024 * 1024,
		FilesystemType: "ext4",
		InUse:          true,
		MountPoint:     "/",
	}
	sdb := state.BlockDeviceInfo{DeviceName: "sdb", MountPoint: "/var/lib/lxd"}
	err := machine.SetMachineBlockDevices(sda, sdb)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State)

	imported, err := newSt.Machine(machine.Id())
	c.Assert(err, jc.ErrorIsNil)

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	devices, err := sb.BlockDevices(imported.MachineTag())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(devices, jc.DeepEquals, []state.BlockDeviceInfo{sda, sdb})
}

func (s *MigrationImportSuite) setupSourceApplications(
	c *gc.C, st *state.State, cons constraints.Value, primeStatusHistory bool,
) (*state.Charm, *state.Application, string) {
	f := factory.NewFactory(st, s.StatePool)

	testModel, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	series := "quantal"
	if testModel.Type() == state.ModelTypeCAAS {
		series = "kubernetes"
	}
	// Add a application with charm settings, app config, and leadership settings.
	testCharm := f.MakeCharm(c, &factory.CharmParams{
		Name:   "starsay", // it has resources
		Series: series,
	})
	c.Assert(testCharm.Meta().Resources, gc.HasLen, 3)
	application, pwd := f.MakeApplicationReturningPassword(c, &factory.ApplicationParams{
		Charm: testCharm,
		CharmConfig: map[string]interface{}{
			"foo": "bar",
		},
		ApplicationConfig: map[string]interface{}{
			"app foo": "app bar",
		},
		ApplicationConfigFields: environschema.Fields{
			"app foo": environschema.Attr{Type: environschema.Tstring}},
		Constraints:  cons,
		DesiredScale: 3,
	})
	err = application.UpdateLeaderSettings(&goodToken{}, map[string]string{
		"leader": "true",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = application.SetMetricCredentials([]byte("sekrit"))
	c.Assert(err, jc.ErrorIsNil)
	// Expose the application.
	c.Assert(application.SetExposed(), jc.ErrorIsNil)
	err = testModel.SetAnnotations(application, testAnnotations)
	c.Assert(err, jc.ErrorIsNil)
	if testModel.Type() == state.ModelTypeCAAS {
		application.SetOperatorStatus(status.StatusInfo{Status: status.Running})
	}
	if primeStatusHistory {
		s.primeStatusHistory(c, application, status.Active, 5)
	}
	return testCharm, application, pwd
}

func (s *MigrationImportSuite) assertImportedApplication(
	c *gc.C, application *state.Application, pwd string, cons constraints.Value,
	exported *state.Application, newModel *state.Model, newSt *state.State, checkStatusHistory bool,
) {
	importedApplications, err := newSt.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedApplications, gc.HasLen, 1)
	imported := importedApplications[0]

	c.Assert(imported.ApplicationTag(), gc.Equals, exported.ApplicationTag())
	c.Assert(imported.Series(), gc.Equals, exported.Series())
	c.Assert(imported.IsExposed(), gc.Equals, exported.IsExposed())
	c.Assert(imported.MetricCredentials(), jc.DeepEquals, exported.MetricCredentials())
	c.Assert(imported.PasswordValid(pwd), jc.IsTrue)

	exportedCharmConfig, err := exported.CharmConfig(model.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	importedCharmConfig, err := imported.CharmConfig(model.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedCharmConfig, jc.DeepEquals, exportedCharmConfig)

	exportedAppConfig, err := exported.ApplicationConfig()
	c.Assert(err, jc.ErrorIsNil)
	importedAppConfig, err := imported.ApplicationConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedAppConfig, jc.DeepEquals, exportedAppConfig)

	exportedLeaderSettings, err := exported.LeaderSettings()
	c.Assert(err, jc.ErrorIsNil)
	importedLeaderSettings, err := imported.LeaderSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedLeaderSettings, jc.DeepEquals, exportedLeaderSettings)

	s.assertAnnotations(c, newModel, imported)
	if checkStatusHistory {
		s.checkStatusHistory(c, application, imported, 5)
	}

	newCons, err := imported.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	// Can't test the constraints directly, so go through the string repr.
	c.Assert(newCons.String(), gc.Equals, cons.String())

	rSt, err := newSt.Resources()
	c.Assert(err, jc.ErrorIsNil)
	resources, err := rSt.ListResources(imported.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resources.Resources, gc.HasLen, 3)

	if newModel.Type() == state.ModelTypeCAAS {
		agentTools := version.Binary{
			Number: jujuversion.Current,
			Arch:   arch.HostArch(),
			Series: application.Series(),
		}

		tools, err := imported.AgentTools()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(tools.Version, gc.Equals, agentTools)
	}
}

func (s *MigrationImportSuite) TestApplications(c *gc.C) {
	cons := constraints.MustParse("arch=amd64 mem=8G root-disk-source=tralfamadore")
	testCharm, application, pwd := s.setupSourceApplications(c, s.State, cons, true)

	allApplications, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allApplications, gc.HasLen, 1)
	exported := allApplications[0]

	newModel, newSt := s.importModel(c, s.State)
	// Manually copy across the charm from the old model
	// as it's normally done later.
	f := factory.NewFactory(newSt, s.StatePool)
	f.MakeCharm(c, &factory.CharmParams{
		Name:     "starsay", // it has resources
		URL:      testCharm.URL().String(),
		Revision: strconv.Itoa(testCharm.Revision()),
	})
	s.assertImportedApplication(c, application, pwd, cons, exported, newModel, newSt, true)
}

func (s *MigrationImportSuite) TestApplicationStatus(c *gc.C) {
	cons := constraints.MustParse("arch=amd64 mem=8G")
	testCharm, application, pwd := s.setupSourceApplications(c, s.State, cons, false)

	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: application,
		Status: &status.StatusInfo{
			Status:  status.Active,
			Message: "unit active",
		},
	})

	allApplications, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allApplications, gc.HasLen, 1)
	exported := allApplications[0]

	newModel, newSt := s.importModel(c, s.State)
	// Manually copy across the charm from the old model
	// as it's normally done later.
	f := factory.NewFactory(newSt, s.StatePool)
	f.MakeCharm(c, &factory.CharmParams{
		Name:     "starsay", // it has resources
		URL:      testCharm.URL().String(),
		Revision: strconv.Itoa(testCharm.Revision()),
	})
	s.assertImportedApplication(c, application, pwd, cons, exported, newModel, newSt, false)
	newApp, err := newSt.Application(application.Name())
	c.Assert(err, jc.ErrorIsNil)
	appStatus, err := newApp.Status()
	c.Assert(appStatus.Status, gc.Equals, status.Active)
	c.Assert(appStatus.Message, gc.Equals, "unit active")
}

func (s *MigrationImportSuite) TestCAASApplications(c *gc.C) {
	caasSt := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *gc.C) { caasSt.Close() })

	cons := constraints.MustParse("arch=amd64 mem=8G")
	charm, application, pwd := s.setupSourceApplications(c, caasSt, cons, true)

	model, err := caasSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	caasModel, err := model.CAASModel()
	c.Assert(err, jc.ErrorIsNil)
	err = caasModel.SetPodSpec(application.ApplicationTag(), "pod spec")
	c.Assert(err, jc.ErrorIsNil)
	addr := network.NewScopedAddress("192.168.1.1", network.ScopeCloudLocal)
	err = application.UpdateCloudService("provider-id", []network.Address{addr})
	c.Assert(err, jc.ErrorIsNil)

	allApplications, err := caasSt.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allApplications, gc.HasLen, 1)
	exported := allApplications[0]

	newModel, newSt := s.importModel(c, caasSt)
	// Manually copy across the charm from the old model
	// as it's normally done later.
	f := factory.NewFactory(newSt, s.StatePool)
	f.MakeCharm(c, &factory.CharmParams{
		Name:     "starsay", // it has resources
		URL:      charm.URL().String(),
		Revision: strconv.Itoa(charm.Revision()),
	})
	s.assertImportedApplication(c, application, pwd, cons, exported, newModel, newSt, true)
	newCAASModel, err := newModel.CAASModel()
	c.Assert(err, jc.ErrorIsNil)
	podSpec, err := newCAASModel.PodSpec(application.ApplicationTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(podSpec, gc.Equals, "pod spec")
	newApp, err := newSt.Application(application.Name())
	c.Assert(err, jc.ErrorIsNil)
	cloudService, err := newApp.ServiceInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudService.ProviderId(), gc.Equals, "provider-id")
	c.Assert(cloudService.Addresses(), jc.DeepEquals, []network.Address{addr})
	c.Assert(newApp.GetScale(), gc.Equals, 3)
	c.Assert(newApp.GetPlacement(), gc.Equals, "")
}

func (s *MigrationImportSuite) TestCAASApplicationStatus(c *gc.C) {
	// Caas application status that is derived from unit statuses must survive migration.
	caasSt := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *gc.C) { caasSt.Close() })

	cons := constraints.MustParse("arch=amd64 mem=8G")
	testCharm, application, _ := s.setupSourceApplications(c, caasSt, cons, false)
	ss, err := application.Status()
	c.Logf("status: %s", ss)

	addUnitFactory := factory.NewFactory(caasSt, s.StatePool)
	unit := addUnitFactory.MakeUnit(c, &factory.UnitParams{
		Application: application,
		Status: &status.StatusInfo{
			Status:  status.Active,
			Message: "unit active",
		},
	})
	var updateUnits state.UpdateUnitsOperation
	updateUnits.Updates = []*state.UpdateUnitOperation{
		unit.UpdateOperation(state.UnitUpdateProperties{
			ProviderId: strPtr("provider-id"),
			Address:    strPtr("192.168.1.2"),
			Ports:      &[]string{"80"},
			CloudContainerStatus: &status.StatusInfo{
				Status:  status.Active,
				Message: "cloud container active",
			},
		})}
	err = application.UpdateUnits(&updateUnits)
	c.Assert(err, jc.ErrorIsNil)

	testModel, err := caasSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	caasModel, err := testModel.CAASModel()
	c.Assert(err, jc.ErrorIsNil)
	err = caasModel.SetPodSpec(application.ApplicationTag(), "pod spec")
	c.Assert(err, jc.ErrorIsNil)
	addr := network.NewScopedAddress("192.168.1.1", network.ScopeCloudLocal)
	err = application.UpdateCloudService("provider-id", []network.Address{addr})
	c.Assert(err, jc.ErrorIsNil)

	allApplications, err := caasSt.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allApplications, gc.HasLen, 1)

	_, newSt := s.importModel(c, caasSt)
	// Manually copy across the charm from the old model
	// as it's normally done later.
	f := factory.NewFactory(newSt, s.StatePool)
	f.MakeCharm(c, &factory.CharmParams{
		Name:     "starsay", // it has resources
		URL:      testCharm.URL().String(),
		Revision: strconv.Itoa(testCharm.Revision()),
	})
	newApp, err := newSt.Application(application.Name())
	c.Assert(err, jc.ErrorIsNil)
	// Must use derived status
	appStatus, err := newApp.Status()
	c.Assert(appStatus.Status, gc.Equals, status.Active)
	c.Assert(appStatus.Message, gc.Equals, "unit active")
}

func (s *MigrationImportSuite) TestCharmRevSequencesNotImported(c *gc.C) {
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "mysql",
			URL:  "local:trusty/mysql-2",
		}),
	})
	// Sequence should be set in the source model.
	const charmSeqName = "charmrev-local:trusty/mysql"
	nextVal, err := state.Sequence(s.State, charmSeqName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(nextVal, gc.Equals, 3)

	out, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(len(out.Applications()), gc.Equals, 1)

	uuid := utils.MustNewUUID().String()
	in := newModel(out, uuid, "new")

	_, newSt, err := s.Controller.Import(in)
	c.Assert(err, jc.ErrorIsNil)
	defer newSt.Close()

	// Charm revision sequence shouldn't have been imported. The
	// import of the charm binaries (done separately later) will
	// handle this.
	nextVal, err = state.Sequence(newSt, charmSeqName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(nextVal, gc.Equals, 0)
}

func (s *MigrationImportSuite) TestApplicationsSubordinatesAfter(c *gc.C) {
	// Test for https://bugs.launchpad.net/juju/+bug/1650249
	subordinate := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{Name: "logging"}),
	})

	principal := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{Name: "mysql"}),
	})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: principal})

	sEndpoint, err := subordinate.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	pEndpoint, err := principal.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	relation := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{sEndpoint, pEndpoint},
	})

	ru, err := relation.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the subordinate unit is created.
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	tools, err := unit.AgentTools()
	c.Assert(err, jc.ErrorIsNil)

	sUnits, err := subordinate.AllUnits()
	for _, u := range sUnits {
		// For some reason the EnterScope call doesn't set up the
		// version or enter the scope for the subordinate unit on the
		// other side of the relation.
		err := u.SetAgentVersion(tools.Version)
		c.Assert(err, jc.ErrorIsNil)
		ru, err := relation.Unit(u)
		c.Assert(err, jc.ErrorIsNil)
		err = ru.EnterScope(nil)
		c.Assert(err, jc.ErrorIsNil)
	}

	out, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	apps := out.Applications()
	c.Assert(len(apps), gc.Equals, 2)

	// This test is only valid if the subordinate logging application
	// comes first in the model output.
	if apps[0].Name() != "logging" {
		out = &swapModel{out, c}
	}

	uuid := utils.MustNewUUID().String()
	in := newModel(out, uuid, "new")

	_, newSt, err := s.Controller.Import(in)
	c.Assert(err, jc.ErrorIsNil)
	// add the cleanup here to close the model.
	s.AddCleanup(func(c *gc.C) {
		c.Check(newSt.Close(), jc.ErrorIsNil)
	})
}

func (s *MigrationImportSuite) TestUnits(c *gc.C) {
	s.assertUnitsMigrated(c, s.State, constraints.MustParse("arch=amd64 mem=8G"))
}

func (s *MigrationImportSuite) TestCAASUnits(c *gc.C) {
	caasSt := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *gc.C) { caasSt.Close() })

	s.assertUnitsMigrated(c, caasSt, constraints.MustParse("arch=amd64 mem=8G"))
}

func (s *MigrationImportSuite) TestUnitsWithVirtConstraint(c *gc.C) {
	s.assertUnitsMigrated(c, s.State, constraints.MustParse("arch=amd64 mem=8G virt-type=kvm"))
}

func (s *MigrationImportSuite) assertUnitsMigrated(c *gc.C, st *state.State, cons constraints.Value) {
	f := factory.NewFactory(st, s.StatePool)

	exported, pwd := f.MakeUnitReturningPassword(c, &factory.UnitParams{
		Constraints: cons,
	})
	err := exported.SetMeterStatus("GREEN", "some info")
	c.Assert(err, jc.ErrorIsNil)
	err = exported.SetWorkloadVersion("amethyst")
	c.Assert(err, jc.ErrorIsNil)
	testModel, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = testModel.SetAnnotations(exported, testAnnotations)
	c.Assert(err, jc.ErrorIsNil)

	if testModel.Type() == state.ModelTypeCAAS {
		var updateUnits state.UpdateUnitsOperation
		// need to set a cloud container status so that SetStatus for
		// the unit doesn't throw away the history writes.
		updateUnits.Updates = []*state.UpdateUnitOperation{
			exported.UpdateOperation(state.UnitUpdateProperties{
				ProviderId: strPtr("provider-id"),
				Address:    strPtr("192.168.1.2"),
				Ports:      &[]string{"80"},
				CloudContainerStatus: &status.StatusInfo{
					Status:  status.Active,
					Message: "cloud container active",
				},
			})}
		app, err := exported.Application()
		c.Assert(err, jc.ErrorIsNil)
		err = app.UpdateUnits(&updateUnits)
		c.Assert(err, jc.ErrorIsNil)
	}
	s.primeStatusHistory(c, exported, status.Active, 5)
	s.primeStatusHistory(c, exported.Agent(), status.Idle, 5)

	newModel, newSt := s.importModel(c, st)

	importedApplications, err := newSt.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedApplications, gc.HasLen, 1)

	importedUnits, err := importedApplications[0].AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedUnits, gc.HasLen, 1)
	imported := importedUnits[0]

	c.Assert(imported.UnitTag(), gc.Equals, exported.UnitTag())
	c.Assert(imported.PasswordValid(pwd), jc.IsTrue)
	v, err := imported.WorkloadVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, gc.Equals, "amethyst")

	if newModel.Type() == state.ModelTypeIAAS {
		exportedMachineId, err := exported.AssignedMachineId()
		c.Assert(err, jc.ErrorIsNil)
		importedMachineId, err := imported.AssignedMachineId()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(importedMachineId, gc.Equals, exportedMachineId)

		// Confirm machine Principals are set.
		exportedMachine, err := st.Machine(exportedMachineId)
		c.Assert(err, jc.ErrorIsNil)
		importedMachine, err := newSt.Machine(importedMachineId)
		c.Assert(err, jc.ErrorIsNil)
		s.AssertMachineEqual(c, importedMachine, exportedMachine)
	}
	if newModel.Type() == state.ModelTypeCAAS {
		containerInfo, err := imported.ContainerInfo()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(containerInfo.ProviderId(), gc.Equals, "provider-id")
		c.Assert(containerInfo.Ports(), jc.DeepEquals, []string{"80"})
		addr := network.NewScopedAddress("192.168.1.2", network.ScopeMachineLocal)
		c.Assert(containerInfo.Address(), jc.DeepEquals, &addr)
	}

	meterStatus, err := imported.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meterStatus, gc.Equals, state.MeterStatus{state.MeterGreen, "some info"})
	s.assertAnnotations(c, newModel, imported)
	s.checkStatusHistory(c, exported, imported, 5)
	s.checkStatusHistory(c, exported.Agent(), imported.Agent(), 5)
	s.checkStatusHistory(c, exported.WorkloadVersionHistory(), imported.WorkloadVersionHistory(), 1)

	newCons, err := imported.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	// Can't test the constraints directly, so go through the string repr.
	c.Assert(newCons.String(), gc.Equals, cons.String())
}

func (s *MigrationImportSuite) TestRelations(c *gc.C) {
	wordpress := state.AddTestingApplication(c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	state.AddTestingApplication(c, s.State, "mysql", state.AddTestingCharm(c, s.State, "mysql"))
	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	err = rel.SetStatus(status.StatusInfo{Status: status.Joined})
	c.Assert(err, jc.ErrorIsNil)
	wordpress_0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})

	ru, err := rel.Unit(wordpress_0)
	c.Assert(err, jc.ErrorIsNil)
	relSettings := map[string]interface{}{
		"name": "wordpress/0",
	}
	err = ru.EnterScope(relSettings)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State)

	newWordpress, err := newSt.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(state.RelationCount(newWordpress), gc.Equals, 1)
	rels, err := newWordpress.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rels, gc.HasLen, 1)
	units, err := newWordpress.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)

	relStatus, err := rels[0].Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relStatus.Status, gc.Equals, status.Joined)

	ru, err = rels[0].Unit(units[0])
	c.Assert(err, jc.ErrorIsNil)

	settings, err := ru.Settings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings.Map(), gc.DeepEquals, relSettings)
}

func (s *MigrationImportSuite) assertRelationsMissingStatus(c *gc.C, hasUnits bool) {
	wordpress := state.AddTestingApplication(c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	state.AddTestingApplication(c, s.State, "mysql", state.AddTestingCharm(c, s.State, "mysql"))
	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	if hasUnits {
		wordpress_0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})
		ru, err := rel.Unit(wordpress_0)
		c.Assert(err, jc.ErrorIsNil)
		relSettings := map[string]interface{}{
			"name": "wordpress/0",
		}
		err = ru.EnterScope(relSettings)
		c.Assert(err, jc.ErrorIsNil)
	}

	_, newSt := s.importModel(c, s.State, func(desc map[string]interface{}) {
		relations := desc["relations"].(map[interface{}]interface{})
		for _, item := range relations["relations"].([]interface{}) {
			relation := item.(map[interface{}]interface{})
			delete(relation, "status")
		}
	})

	newWordpress, err := newSt.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(state.RelationCount(newWordpress), gc.Equals, 1)
	rels, err := newWordpress.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rels, gc.HasLen, 1)

	relStatus, err := rels[0].Status()
	c.Assert(err, jc.ErrorIsNil)
	if hasUnits {
		c.Assert(relStatus.Status, gc.Equals, status.Joined)
	} else {
		c.Assert(relStatus.Status, gc.Equals, status.Joining)
	}
}

func (s *MigrationImportSuite) TestRelationsMissingStatusWithUnits(c *gc.C) {
	s.assertRelationsMissingStatus(c, true)
}

func (s *MigrationImportSuite) TestRelationsMissingStatusNoUnits(c *gc.C) {
	s.assertRelationsMissingStatus(c, false)
}

func (s *MigrationImportSuite) TestEndpointBindings(c *gc.C) {
	// Endpoint bindings need both valid charms, applications, and spaces.
	s.Factory.MakeSpace(c, &factory.SpaceParams{
		Name: "one", ProviderID: corenetwork.Id("provider"), IsPublic: true})
	state.AddTestingApplicationWithBindings(
		c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"),
		map[string]string{"db": "one"})

	_, newSt := s.importModel(c, s.State)

	newWordpress, err := newSt.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)

	bindings, err := newWordpress.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	// There are empty values for every charm endpoint, but we only care about the
	// db endpoint.
	c.Assert(bindings["db"], gc.Equals, "one")
}

func (s *MigrationImportSuite) TestUnitsOpenPorts(c *gc.C) {
	unit := s.Factory.MakeUnit(c, nil)
	err := unit.OpenPorts("tcp", 1234, 2345)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State)

	// Even though the opened ports document is stored with the
	// machine, the only way to easily access it is through the units.
	imported, err := newSt.Unit(unit.Name())
	c.Assert(err, jc.ErrorIsNil)

	ports, err := imported.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, 1)
	c.Assert(ports[0], gc.Equals, corenetwork.PortRange{
		FromPort: 1234,
		ToPort:   2345,
		Protocol: "tcp",
	})
}

func (s *MigrationImportSuite) TestSpaces(c *gc.C) {
	space := s.Factory.MakeSpace(c, &factory.SpaceParams{
		Name: "one", ProviderID: corenetwork.Id("provider"), IsPublic: true})

	_, newSt := s.importModel(c, s.State)

	imported, err := newSt.Space(space.Name())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(imported.Name(), gc.Equals, space.Name())
	c.Assert(imported.ProviderId(), gc.Equals, space.ProviderId())
	c.Assert(imported.IsPublic(), gc.Equals, space.IsPublic())
}

func (s *MigrationImportSuite) TestDestroyEmptyModel(c *gc.C) {
	newModel, _ := s.importModel(c, s.State)
	s.assertDestroyModelAdvancesLife(c, newModel, state.Dying)
}

func (s *MigrationImportSuite) TestDestroyModelWithMachine(c *gc.C) {
	s.Factory.MakeMachine(c, nil)
	newModel, _ := s.importModel(c, s.State)
	s.assertDestroyModelAdvancesLife(c, newModel, state.Dying)
}

func (s *MigrationImportSuite) TestDestroyModelWithApplication(c *gc.C) {
	s.Factory.MakeApplication(c, nil)
	newModel, _ := s.importModel(c, s.State)
	s.assertDestroyModelAdvancesLife(c, newModel, state.Dying)
}

func (s *MigrationImportSuite) assertDestroyModelAdvancesLife(c *gc.C, m *state.Model, life state.Life) {
	c.Assert(m.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(m.Refresh(), jc.ErrorIsNil)
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
	_, newSt := s.importModel(c, s.State)

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
	_, newSt := s.importModel(c, s.State)

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
	original, err := s.State.AddSubnet(corenetwork.SubnetInfo{
		CIDR:              "10.0.0.0/24",
		ProviderId:        corenetwork.Id("foo"),
		ProviderNetworkId: corenetwork.Id("elm"),
		VLANTag:           64,
		AvailabilityZones: []string{"bar"},
		SpaceName:         "bam",
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("bam", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State)

	subnet, err := newSt.Subnet(original.CIDR())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnet.CIDR(), gc.Equals, "10.0.0.0/24")
	c.Assert(subnet.ProviderId(), gc.Equals, corenetwork.Id("foo"))
	c.Assert(subnet.ProviderNetworkId(), gc.Equals, corenetwork.Id("elm"))
	c.Assert(subnet.VLANTag(), gc.Equals, 64)
	c.Assert(subnet.AvailabilityZone(), gc.Equals, "bar")
	c.Assert(subnet.SpaceName(), gc.Equals, "bam")
	c.Assert(subnet.FanLocalUnderlay(), gc.Equals, "")
	c.Assert(subnet.FanOverlay(), gc.Equals, "")
}

func (s *MigrationImportSuite) TestSubnetsWithFan(c *gc.C) {
	_, err := s.State.AddSubnet(corenetwork.SubnetInfo{
		CIDR:      "100.2.0.0/16",
		SpaceName: "bam",
	})
	c.Assert(err, jc.ErrorIsNil)

	sn := corenetwork.SubnetInfo{
		CIDR:              "10.0.0.0/24",
		ProviderId:        corenetwork.Id("foo"),
		ProviderNetworkId: corenetwork.Id("elm"),
		VLANTag:           64,
		AvailabilityZones: []string{"bar"},
	}
	sn.SetFan("100.2.0.0/16", "253.0.0.0/8")

	original, err := s.State.AddSubnet(sn)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("bam", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State)

	subnet, err := newSt.Subnet(original.CIDR())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnet.CIDR(), gc.Equals, "10.0.0.0/24")
	c.Assert(subnet.ProviderId(), gc.Equals, corenetwork.Id("foo"))
	c.Assert(subnet.ProviderNetworkId(), gc.Equals, corenetwork.Id("elm"))
	c.Assert(subnet.VLANTag(), gc.Equals, 64)
	c.Assert(subnet.AvailabilityZone(), gc.Equals, "bar")
	c.Assert(subnet.SpaceName(), gc.Equals, "bam")
	c.Assert(subnet.FanLocalUnderlay(), gc.Equals, "100.2.0.0/16")
	c.Assert(subnet.FanOverlay(), gc.Equals, "253.0.0.0/8")
}

func (s *MigrationImportSuite) TestIPAddress(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	_, err := s.State.AddSubnet(corenetwork.SubnetInfo{CIDR: "0.1.2.0/24"})
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

	_, newSt := s.importModel(c, s.State)

	addresses, _ := newSt.AllIPAddresses()
	c.Assert(addresses, gc.HasLen, 1)
	c.Assert(err, jc.ErrorIsNil)
	addr := addresses[0]
	c.Assert(addr.Value(), gc.Equals, "0.1.2.3")
	c.Assert(addr.MachineID(), gc.Equals, machine.Id())
	c.Assert(addr.DeviceName(), gc.Equals, "foo")
	c.Assert(addr.ConfigMethod(), gc.Equals, state.StaticAddress)
	c.Assert(addr.SubnetCIDR(), gc.Equals, "0.1.2.0/24")
	c.Assert(addr.ProviderID(), gc.Equals, corenetwork.Id("bar"))
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

	_, newSt := s.importModel(c, s.State)

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
	attrsCustom := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region-custom",
		Version:         "14.04",
		Series:          "trusty",
		Arch:            "arch",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test",
		RootStorageSize: &storageSize,
		Source:          "custom",
	}
	metadata := []cloudimagemetadata.Metadata{
		{attrs, 2, "1", 2},
		{attrsCustom, 3, "2", 3},
	}

	err := s.State.CloudImageMetadataStorage.SaveMetadata(metadata)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State, func(map[string]interface{}) {
		// Image metadata collection is global so we need to delete it
		// to properly test import.
		all, err := s.State.CloudImageMetadataStorage.AllCloudImageMetadata()
		c.Assert(err, jc.ErrorIsNil)
		for _, m := range all {
			err := s.State.CloudImageMetadataStorage.DeleteMetadata(m.ImageId)
			c.Assert(err, jc.ErrorIsNil)
		}
	})
	defer func() {
		c.Assert(newSt.Close(), jc.ErrorIsNil)
	}()

	images, err := newSt.CloudImageMetadataStorage.AllCloudImageMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(images, gc.HasLen, 1)
	image := images[0]
	c.Check(image.Stream, gc.Equals, "stream")
	c.Check(image.Region, gc.Equals, "region-custom")
	c.Check(image.Version, gc.Equals, "14.04")
	c.Check(image.Arch, gc.Equals, "arch")
	c.Check(image.VirtType, gc.Equals, "virtType-test")
	c.Check(image.RootStorageType, gc.Equals, "rootStorageType-test")
	c.Check(*image.RootStorageSize, gc.Equals, uint64(3))
	c.Check(image.Source, gc.Equals, "custom")
	c.Check(image.Priority, gc.Equals, 3)
	c.Check(image.ImageId, gc.Equals, "2")
	c.Check(image.DateCreated, gc.Equals, int64(3))
}

func (s *MigrationImportSuite) TestAction(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})

	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	_, err = m.EnqueueAction(machine.MachineTag(), "foo", nil)
	c.Assert(err, jc.ErrorIsNil)

	newModel, newState := s.importModel(c, s.State)
	defer func() {
		c.Assert(newState.Close(), jc.ErrorIsNil)
	}()

	actions, _ := newModel.AllActions()
	c.Assert(actions, gc.HasLen, 1)
	action := actions[0]
	c.Check(action.Receiver(), gc.Equals, machine.Id())
	c.Check(action.Name(), gc.Equals, "foo")
	c.Check(action.Status(), gc.Equals, state.ActionPending)
}

func (s *MigrationImportSuite) TestVolumes(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Volumes: []state.HostVolumeParams{{
			Volume:     state.VolumeParams{Size: 1234},
			Attachment: state.VolumeAttachmentParams{ReadOnly: true},
		}, {
			Volume:     state.VolumeParams{Size: 4000},
			Attachment: state.VolumeAttachmentParams{ReadOnly: true},
		}, {
			Volume:     state.VolumeParams{Size: 3000},
			Attachment: state.VolumeAttachmentParams{ReadOnly: true},
		}},
	})
	machineTag := machine.MachineTag()

	// We know that the first volume is called "0/0" - although I don't know why.
	volTag := names.NewVolumeTag("0/0")
	volInfo := state.VolumeInfo{
		HardwareId: "magic",
		WWN:        "drbr",
		Size:       1500,
		Pool:       "loop",
		VolumeId:   "volume id",
		Persistent: true,
	}
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	err = sb.SetVolumeInfo(volTag, volInfo)
	c.Assert(err, jc.ErrorIsNil)
	volAttachmentInfo := state.VolumeAttachmentInfo{
		DeviceName: "device name",
		DeviceLink: "device link",
		BusAddress: "bus address",
		ReadOnly:   true,
	}

	err = sb.SetVolumeAttachmentInfo(machineTag, volTag, volAttachmentInfo)
	c.Assert(err, jc.ErrorIsNil)

	// attach a iSCSI volume
	iscsiVolTag := names.NewVolumeTag("0/2")
	iscsiVolInfo := state.VolumeInfo{
		HardwareId: "magic",
		WWN:        "iscsi",
		Size:       1500,
		Pool:       "loop",
		VolumeId:   "iscsi id",
		Persistent: true,
	}

	deviceAttrs := map[string]string{
		"iqn":         "bogusIQN",
		"address":     "192.168.1.1",
		"port":        "9999",
		"chap-user":   "example",
		"chap-secret": "supersecretpassword",
	}

	attachmentPlanInfo := state.VolumeAttachmentPlanInfo{
		DeviceType:       storage.DeviceTypeISCSI,
		DeviceAttributes: deviceAttrs,
	}

	iscsiVolAttachmentInfo := state.VolumeAttachmentInfo{
		DeviceName: "iscsi device",
		DeviceLink: "iscsi link",
		BusAddress: "iscsi address",
		ReadOnly:   true,
		PlanInfo:   &attachmentPlanInfo,
	}

	err = sb.SetVolumeInfo(iscsiVolTag, iscsiVolInfo)
	c.Assert(err, jc.ErrorIsNil)

	err = sb.SetVolumeAttachmentInfo(machineTag, iscsiVolTag, iscsiVolAttachmentInfo)
	c.Assert(err, jc.ErrorIsNil)

	err = sb.CreateVolumeAttachmentPlan(machineTag, iscsiVolTag, attachmentPlanInfo)
	c.Assert(err, jc.ErrorIsNil)

	deviceLinks := []string{"/dev/sdb", "/dev/mapper/testDevice"}

	blockInfo := state.BlockDeviceInfo{
		WWN:         "testWWN",
		DeviceLinks: deviceLinks,
		HardwareId:  "test-id",
	}

	err = sb.SetVolumeAttachmentPlanBlockInfo(machineTag, iscsiVolTag, blockInfo)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State)
	newSb, err := state.NewStorageBackend(newSt)
	c.Assert(err, jc.ErrorIsNil)

	volume, err := newSb.Volume(volTag)
	c.Assert(err, jc.ErrorIsNil)

	// TODO: check status
	// TODO: check storage instance
	info, err := volume.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(info, jc.DeepEquals, volInfo)

	attachment, err := newSb.VolumeAttachment(machineTag, volTag)
	c.Assert(err, jc.ErrorIsNil)
	attInfo, err := attachment.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(attInfo, jc.DeepEquals, volAttachmentInfo)

	_, err = newSb.VolumeAttachmentPlan(machineTag, volTag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	volTag = names.NewVolumeTag("0/1")
	volume, err = newSb.Volume(volTag)
	c.Assert(err, jc.ErrorIsNil)

	params, needsProvisioning := volume.Params()
	c.Check(needsProvisioning, jc.IsTrue)
	c.Check(params.Pool, gc.Equals, "loop")
	c.Check(params.Size, gc.Equals, uint64(4000))

	attachment, err = newSb.VolumeAttachment(machineTag, volTag)
	c.Assert(err, jc.ErrorIsNil)
	attParams, needsProvisioning := attachment.Params()
	c.Check(needsProvisioning, jc.IsTrue)
	c.Check(attParams.ReadOnly, jc.IsTrue)

	iscsiVolume, err := newSb.Volume(iscsiVolTag)
	c.Assert(err, jc.ErrorIsNil)

	iscsiInfo, err := iscsiVolume.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(iscsiInfo, jc.DeepEquals, iscsiVolInfo)

	attachment, err = newSb.VolumeAttachment(machineTag, iscsiVolTag)
	c.Assert(err, jc.ErrorIsNil)
	attInfo, err = attachment.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(attInfo, jc.DeepEquals, iscsiVolAttachmentInfo)

	attachmentPlan, err := newSb.VolumeAttachmentPlan(machineTag, iscsiVolTag)
	c.Assert(err, gc.IsNil)
	c.Assert(attachmentPlan.Volume(), gc.Equals, iscsiVolTag)
	c.Assert(attachmentPlan.Machine(), gc.Equals, machineTag)

	planInfo, err := attachmentPlan.PlanInfo()
	c.Assert(err, gc.IsNil)
	c.Assert(planInfo, jc.DeepEquals, attachmentPlanInfo)

	volBlockInfo, err := attachmentPlan.BlockDeviceInfo()
	c.Assert(err, gc.IsNil)
	c.Assert(volBlockInfo, jc.DeepEquals, blockInfo)
}

func (s *MigrationImportSuite) TestFilesystems(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Filesystems: []state.HostFilesystemParams{{
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
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	err = sb.SetFilesystemInfo(fsTag, fsInfo)
	c.Assert(err, jc.ErrorIsNil)
	fsAttachmentInfo := state.FilesystemAttachmentInfo{
		MountPoint: "/mnt/foo",
		ReadOnly:   true,
	}
	err = sb.SetFilesystemAttachmentInfo(machineTag, fsTag, fsAttachmentInfo)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State)
	newSb, err := state.NewStorageBackend(newSt)
	c.Assert(err, jc.ErrorIsNil)

	filesystem, err := newSb.Filesystem(fsTag)
	c.Assert(err, jc.ErrorIsNil)

	// TODO: check status
	// TODO: check storage instance
	info, err := filesystem.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(info, jc.DeepEquals, fsInfo)

	attachment, err := newSb.FilesystemAttachment(machineTag, fsTag)
	c.Assert(err, jc.ErrorIsNil)
	attInfo, err := attachment.Info()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(attInfo, jc.DeepEquals, fsAttachmentInfo)

	fsTag = names.NewFilesystemTag("0/1")
	filesystem, err = newSb.Filesystem(fsTag)
	c.Assert(err, jc.ErrorIsNil)

	params, needsProvisioning := filesystem.Params()
	c.Check(needsProvisioning, jc.IsTrue)
	c.Check(params.Pool, gc.Equals, "rootfs")
	c.Check(params.Size, gc.Equals, uint64(4000))

	attachment, err = newSb.FilesystemAttachment(machineTag, fsTag)
	c.Assert(err, jc.ErrorIsNil)
	attParams, needsProvisioning := attachment.Params()
	c.Check(needsProvisioning, jc.IsTrue)
	c.Check(attParams.ReadOnly, jc.IsTrue)
}

func (s *MigrationImportSuite) TestStorage(c *gc.C) {
	app, u, storageTag := s.makeUnitWithStorage(c)
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	original, err := sb.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	originalCount := state.StorageAttachmentCount(original)
	c.Assert(originalCount, gc.Equals, 1)
	originalAttachments, err := sb.StorageAttachments(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(originalAttachments, gc.HasLen, 1)
	c.Assert(originalAttachments[0].Unit(), gc.Equals, u.UnitTag())
	appName := app.Name()

	_, newSt := s.importModel(c, s.State)

	app, err = newSt.Application(appName)
	c.Assert(err, jc.ErrorIsNil)
	cons, err := app.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cons, jc.DeepEquals, map[string]state.StorageConstraints{
		"data":    {Pool: "modelscoped", Size: 0x400, Count: 1},
		"allecto": {Pool: "loop", Size: 0x400},
	})

	newSb, err := state.NewStorageBackend(newSt)
	c.Assert(err, jc.ErrorIsNil)

	testInstance, err := newSb.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(testInstance.Tag(), gc.Equals, original.Tag())
	c.Check(testInstance.Kind(), gc.Equals, original.Kind())
	c.Check(testInstance.Life(), gc.Equals, original.Life())
	c.Check(testInstance.StorageName(), gc.Equals, original.StorageName())
	c.Check(testInstance.Pool(), gc.Equals, original.Pool())
	c.Check(state.StorageAttachmentCount(testInstance), gc.Equals, originalCount)

	attachments, err := newSb.StorageAttachments(storageTag)
	c.Assert(attachments, gc.HasLen, 1)
	c.Assert(attachments[0].Unit(), gc.Equals, u.UnitTag())
}

func (s *MigrationImportSuite) TestStorageDetached(c *gc.C) {
	_, u, storageTag := s.makeUnitWithStorage(c)
	err := u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	err = sb.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)

	s.importModel(c, s.State)
}

func (s *MigrationImportSuite) TestStorageInstanceConstraints(c *gc.C) {
	_, _, storageTag := s.makeUnitWithStorage(c)
	_, newSt := s.importModel(c, s.State, func(desc map[string]interface{}) {
		storages := desc["storages"].(map[interface{}]interface{})
		for _, item := range storages["storages"].([]interface{}) {
			testStorage := item.(map[interface{}]interface{})
			cons := testStorage["constraints"].(map[interface{}]interface{})
			cons["pool"] = "static"
		}
	})
	newSb, err := state.NewStorageBackend(newSt)
	c.Assert(err, jc.ErrorIsNil)
	testInstance, err := newSb.StorageInstance(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(testInstance.Pool(), gc.Equals, "static")
}

func (s *MigrationImportSuite) TestStorageInstanceConstraintsFallback(c *gc.C) {
	_, u, storageTag0 := s.makeUnitWithStorage(c)

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	_, err = sb.AddStorageForUnit(u.UnitTag(), "allecto", state.StorageConstraints{
		Count: 3,
		Size:  1234,
		Pool:  "modelscoped",
	})
	c.Assert(err, jc.ErrorIsNil)
	storageTag1 := names.NewStorageTag("allecto/1")
	storageTag2 := names.NewStorageTag("allecto/2")

	// We delete the storage instance constraints for each storage
	// instance. For data/0 and allecto/1 we also delete the volume,
	// and we delete the application storage constraints for "data".
	//
	// We expect:
	//  - for data/0, to get the defaults (loop, 1G)
	//  - for allecto/1, to get the application storage constraints
	//  - for allecto/2, to get the volume pool/size

	_, newSt := s.importModel(c, s.State, func(desc map[string]interface{}) {
		applications := desc["applications"].(map[interface{}]interface{})
		volumes := desc["volumes"].(map[interface{}]interface{})
		storages := desc["storages"].(map[interface{}]interface{})
		storages["version"] = 2

		app := applications["applications"].([]interface{})[0].(map[interface{}]interface{})
		sc := app["storage-constraints"].(map[interface{}]interface{})
		delete(sc, "data")
		sc["allecto"].(map[interface{}]interface{})["pool"] = "modelscoped-block"

		var keepVolumes []interface{}
		for _, item := range volumes["volumes"].([]interface{}) {
			volume := item.(map[interface{}]interface{})
			switch volume["storage-id"] {
			case storageTag0.Id(), storageTag1.Id():
			default:
				keepVolumes = append(keepVolumes, volume)
			}
		}
		volumes["volumes"] = keepVolumes

		for _, item := range storages["storages"].([]interface{}) {
			testStorage := item.(map[interface{}]interface{})
			delete(testStorage, "constraints")
		}
	})

	newSb, err := state.NewStorageBackend(newSt)
	c.Assert(err, jc.ErrorIsNil)

	instance0, err := newSb.StorageInstance(storageTag0)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(instance0.Pool(), gc.Equals, "loop")

	instance1, err := newSb.StorageInstance(storageTag1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(instance1.Pool(), gc.Equals, "modelscoped-block")

	instance2, err := newSb.StorageInstance(storageTag2)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(instance2.Pool(), gc.Equals, "modelscoped")
}

func (s *MigrationImportSuite) TestStoragePools(c *gc.C) {
	pm := poolmanager.New(state.NewStateSettings(s.State), provider.CommonStorageProviders())
	_, err := pm.Create("test-pool", provider.LoopProviderType, map[string]interface{}{
		"value": 42,
	})
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State)

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

	_, newSt := s.importModel(c, s.State)

	unit, err := newSt.Unit(unitID)
	c.Assert(err, jc.ErrorIsNil)

	up, err = newSt.UnitPayloads(unit)
	c.Assert(err, jc.ErrorIsNil)

	result, err := up.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result[0].Payload, gc.NotNil)

	testPayload := result[0].Payload

	machineID, err := unit.AssignedMachineId()
	c.Check(err, jc.ErrorIsNil)
	c.Check(testPayload.Name, gc.Equals, original.Name)
	c.Check(testPayload.Type, gc.Equals, original.Type)
	c.Check(testPayload.ID, gc.Equals, original.ID)
	c.Check(testPayload.Status, gc.Equals, original.Status)
	c.Check(testPayload.Labels, jc.DeepEquals, original.Labels)
	c.Check(testPayload.Unit, gc.Equals, unitID)
	c.Check(testPayload.Machine, gc.Equals, machineID)
}

func (s *MigrationImportSuite) TestRemoteApplications(c *gc.C) {
	// For now we want to prevent importing models that have remote
	// applications - cross-model relations don't support relations
	// with the models in different controllers.
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "gravy-rainbow",
		URL:         "me/model.rainbow",
		SourceModel: s.Model.ModelTag(),
		Token:       "charisma",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}, {
			Interface: "mysql-root",
			Name:      "db-admin",
			Limit:     5,
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}, {
			Interface: "logging",
			Name:      "logging",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	out, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	uuid := utils.MustNewUUID().String()
	in := newModel(out, uuid, "new")
	// Models for this version of Juju don't export remote
	// applications but we still want to guard against accidentally
	// importing any that may exist from earlier versions.
	in.AddRemoteApplication(description.RemoteApplicationArgs{
		SourceModel: coretesting.ModelTag,
		OfferUUID:   utils.MustNewUUID().String(),
		Tag:         names.NewApplicationTag("remote"),
	})

	_, newSt, err := s.Controller.Import(in)
	if err == nil {
		defer newSt.Close()
	}
	c.Assert(err, gc.ErrorMatches, "can't import models with remote applications")
}

func (s *MigrationImportSuite) TestApplicationsWithNilConfigValues(c *gc.C) {
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		CharmConfig: map[string]interface{}{
			"foo": "bar",
		},
	})
	s.primeStatusHistory(c, application, status.Active, 5)
	// Since above factory method calls newly updated state.AddApplication(...)
	// which removes config settings with nil value before writing
	// application into database,
	// strip config setting values to nil directly to simulate
	// what could happen to some applications in 2.0 and 2.1.
	// For more context, see https://bugs.launchpad.net/juju/+bug/1667199
	settings := state.GetApplicationCharmConfig(s.State, application)
	settings.Set("foo", nil)
	_, err := settings.Write()
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State)

	importedApplications, err := newSt.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedApplications, gc.HasLen, 1)
	importedApplication := importedApplications[0]

	// Ensure that during import application settings with nil config values
	// were stripped and not written into database.
	importedSettings := state.GetApplicationCharmConfig(newSt, importedApplication)
	_, importedFound := importedSettings.Get("foo")
	c.Assert(importedFound, jc.IsFalse)
}

func (s *MigrationImportSuite) TestOneSubordinateTwoGuvnors(c *gc.C) {
	// Check that invalid relationscopes aren't created when importing
	// a subordinate related to 2 principals.
	wordpress := state.AddTestingApplication(c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	mysql := state.AddTestingApplication(c, s.State, "mysql", state.AddTestingCharm(c, s.State, "mysql"))
	wordpress0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})
	mysql0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: mysql})

	logging := s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))

	addSubordinate := func(app *state.Application, unit *state.Unit) string {
		eps, err := s.State.InferEndpoints(app.Name(), logging.Name())
		c.Assert(err, jc.ErrorIsNil)
		rel, err := s.State.AddRelation(eps...)
		c.Assert(err, jc.ErrorIsNil)
		pru, err := rel.Unit(unit)
		c.Assert(err, jc.ErrorIsNil)
		err = pru.EnterScope(nil)
		c.Assert(err, jc.ErrorIsNil)
		// Need to reload the doc to get the subordinates.
		err = unit.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		subordinates := unit.SubordinateNames()
		c.Assert(subordinates, gc.HasLen, 1)
		loggingUnit, err := s.State.Unit(subordinates[0])
		c.Assert(err, jc.ErrorIsNil)
		sub, err := rel.Unit(loggingUnit)
		c.Assert(err, jc.ErrorIsNil)
		err = sub.EnterScope(nil)
		c.Assert(err, jc.ErrorIsNil)
		return rel.String()
	}

	logMysqlKey := addSubordinate(mysql, mysql0)
	logWpKey := addSubordinate(wordpress, wordpress0)

	units, err := logging.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 2)

	for _, unit := range units {
		app, err := unit.Application()
		c.Assert(err, jc.ErrorIsNil)
		agentTools := version.Binary{
			Number: jujuversion.Current,
			Arch:   arch.HostArch(),
			Series: app.Series(),
		}
		err = unit.SetAgentVersion(agentTools)
		c.Assert(err, jc.ErrorIsNil)
	}

	_, newSt := s.importModel(c, s.State)

	logMysqlRel, err := newSt.KeyRelation(logMysqlKey)
	c.Assert(err, jc.ErrorIsNil)
	logWpRel, err := newSt.KeyRelation(logWpKey)
	c.Assert(err, jc.ErrorIsNil)

	mysqlLogUnit, err := newSt.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)
	wpLogUnit, err := newSt.Unit("logging/1")
	c.Assert(err, jc.ErrorIsNil)

	// Sanity checks
	name, ok := mysqlLogUnit.PrincipalName()
	c.Assert(ok, jc.IsTrue)
	c.Assert(name, gc.Equals, "mysql/0")

	name, ok = wpLogUnit.PrincipalName()
	c.Assert(ok, jc.IsTrue)
	c.Assert(name, gc.Equals, "wordpress/0")

	checkScope := func(unit *state.Unit, rel *state.Relation, expected bool) {
		ru, err := rel.Unit(unit)
		c.Assert(err, jc.ErrorIsNil)
		// Sanity check
		valid, err := ru.Valid()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(valid, gc.Equals, expected)

		inscope, err := ru.InScope()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(inscope, gc.Equals, expected)
	}
	// The WP logging unit shouldn't be in scope for the mysql-logging
	// relation.
	checkScope(wpLogUnit, logMysqlRel, false)
	// Similarly, the mysql logging unit shouldn't be in scope for the
	// wp-logging relation.
	checkScope(mysqlLogUnit, logWpRel, false)

	// But obviously the units should be in their relations.
	checkScope(mysqlLogUnit, logMysqlRel, true)
	checkScope(wpLogUnit, logWpRel, true)
}

func (s *MigrationImportSuite) TestImportingModelWithBlankType(c *gc.C) {
	testModel, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	newConfig := testModel.Config()
	newConfig["uuid"] = "aabbccdd-1234-8765-abcd-0123456789ab"
	newConfig["name"] = "something-new"
	noTypeModel := description.NewModel(description.ModelArgs{
		Type:               "",
		Owner:              testModel.Owner(),
		Config:             newConfig,
		LatestToolsVersion: testModel.LatestToolsVersion(),
		EnvironVersion:     testModel.EnvironVersion(),
		Blocks:             testModel.Blocks(),
		Cloud:              testModel.Cloud(),
		CloudRegion:        testModel.CloudRegion(),
	})
	imported, newSt, err := s.Controller.Import(noTypeModel)
	c.Assert(err, jc.ErrorIsNil)
	defer newSt.Close()

	c.Assert(imported.Type(), gc.Equals, state.ModelTypeIAAS)
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

// swapModel will swap the order of the applications appearing in the
// model.
type swapModel struct {
	description.Model
	c *gc.C
}

func (m swapModel) Applications() []description.Application {
	values := m.Model.Applications()
	m.c.Assert(len(values), gc.Equals, 2)
	values[0], values[1] = values[1], values[0]
	return values
}
