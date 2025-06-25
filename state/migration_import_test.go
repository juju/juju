// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"
	"strconv"
	"time" // only uses time.Time values

	"github.com/juju/charm/v12"
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	"github.com/kr/pretty"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/state/mocks"
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
	out, err := s.State.Export(map[string]string{})
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.Controller.Import(out)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *MigrationImportSuite) importModel(
	c *gc.C, st *state.State, transform ...func(map[string]interface{}),
) (*state.Model, *state.State) {
	desc, err := st.Export(map[string]string{})
	c.Assert(err, jc.ErrorIsNil)
	return s.importModelDescription(c, desc, transform...)
}

func (s *MigrationImportSuite) importModelDescription(
	c *gc.C, desc description.Model, transform ...func(map[string]interface{}),
) (*state.Model, *state.State) {

	// When working with importing models, it becomes very handy to read the
	// model in a human-readable format.
	// yaml.Marshal will do this in a decent manor.
	//	bytes, _ := yaml.Marshal(desc)
	//	fmt.Println(string(bytes))

	if len(transform) > 0 {
		var outM map[string]interface{}
		outYaml, err := description.Serialize(desc)
		c.Assert(err, jc.ErrorIsNil)
		err = yaml.Unmarshal(outYaml, &outM)
		c.Assert(err, jc.ErrorIsNil)

		for _, transform := range transform {
			transform(outM)
		}

		outYaml, err = yaml.Marshal(outM)
		c.Assert(err, jc.ErrorIsNil)
		desc, err = description.Deserialize(outYaml)
		c.Assert(err, jc.ErrorIsNil)
	}

	uuid := utils.MustNewUUID().String()
	in := newModel(desc, uuid, "new")

	newModel, newSt, err := s.Controller.Import(in)
	c.Assert(err, jc.ErrorIsNil)

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

	err = original.SetPassword("supersecret1111111111111")
	c.Assert(err, jc.ErrorIsNil)

	err = s.Model.SetAnnotations(original, testAnnotations)
	c.Assert(err, jc.ErrorIsNil)

	out, err := s.State.Export(map[string]string{})
	c.Assert(err, jc.ErrorIsNil)

	uuid := utils.MustNewUUID().String()
	in := newModel(out, uuid, "new")

	newModel, newSt, err := s.Controller.Import(in)
	c.Assert(err, jc.ErrorIsNil)
	defer newSt.Close()

	c.Assert(newModel.PasswordHash(), gc.Equals, utils.AgentPasswordHash("supersecret1111111111111"))
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

// TestEmptyCredential checks that when the model uses an empty credential
// that it doesn't matter if the attributes on the source or target controller's
// credential is empty/not-nil, they are both equal. This tests that the controller
// can handle an old or a manually edited credential when it has an empty attribute map
// and is compared to the source controller's nil attribute map (or vice versa).
func (s *MigrationImportSuite) TestEmptyCredential(c *gc.C) {
	credTag := names.NewCloudCredentialTag(s.Model.CloudName() + "/" + s.Model.Owner().Id() + "/empty")
	cred := cloud.NewEmptyCredential()
	err := s.State.UpdateCloudCredential(credTag, cred)
	c.Assert(err, jc.ErrorIsNil)
	ok, err := s.Model.SetCloudCredential(credTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsTrue)
	newModel, _ := s.importModel(c, s.State, func(m map[string]interface{}) {
		// Check the the exported credential is omitted.
		c.Assert(m["cloud-credential"].(map[any]any)["attributes"], gc.IsNil)
		// Force the credential to a non-nil empty map to test nil-map empty-map
		// credential check.
		m["cloud-credential"].(map[any]any)["attributes"] = map[string]string{}
	})
	newCredTag, ok := newModel.CloudCredentialTag()
	c.Assert(ok, jc.IsTrue)
	c.Assert(newCredTag.Name(), gc.Equals, "empty")
}

// TestCredentialAttributeMatching checks that the credentials for the target controller
// and the model being imported compare the correct credential attributes.
func (s *MigrationImportSuite) TestCredentialAttributeMatching(c *gc.C) {
	// Create foo credential for the "target" controller.
	err := s.State.UpdateCloudCredential(
		names.NewCloudCredentialTag(s.Model.CloudName()+"/"+s.Model.Owner().Id()+"/bar"),
		cloud.NewCredential(cloud.EmptyAuthType, map[string]string{
			"foo": "d09f00d",
		}))
	c.Assert(err, jc.ErrorIsNil)

	// Create bar credential for the "source" controller
	credTag := names.NewCloudCredentialTag(s.Model.CloudName() + "/" + s.Model.Owner().Id() + "/foo")
	cred := cloud.NewCredential(cloud.EmptyAuthType, map[string]string{
		"foo": "bar",
	})
	err = s.State.UpdateCloudCredential(credTag, cred)
	c.Assert(err, jc.ErrorIsNil)
	ok, err := s.Model.SetCloudCredential(credTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsTrue)

	newModel, _ := s.importModel(c, s.State, func(m map[string]interface{}) {
		// Swap out for the bar credential.
		cred := m["cloud-credential"].(map[any]any)
		c.Assert(cred["name"], gc.Equals, "foo")
		c.Assert(cred["attributes"], gc.DeepEquals, map[any]any{
			"foo": "bar",
		})
		cred["name"] = "bar"
		cred["attributes"] = map[any]any{
			"foo": "d09f00d",
		}
	})
	newCredTag, ok := newModel.CloudCredentialTag()
	c.Assert(ok, jc.IsTrue)
	c.Assert(newCredTag.Name(), gc.Equals, "bar")
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
	c.Check(newMachine.Id(), gc.Equals, oldMachine.Id())
	c.Check(newMachine.Principals(), jc.DeepEquals, oldMachine.Principals())
	c.Check(newMachine.Base().String(), gc.Equals, oldMachine.Base().String())
	c.Check(newMachine.ContainerType(), gc.Equals, oldMachine.ContainerType())
	newHardware, err := newMachine.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	oldHardware, err := oldMachine.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	if oldMachine.ContainerType() != instance.NONE && oldHardware.Arch == nil {
		// test that containers have an architecture added during import
		// if not already there.
		oldHardware.Arch = strPtr("amd64")
	}
	c.Check(newHardware, jc.DeepEquals, oldHardware, gc.Commentf("machine %q", newMachine.Id()))

	c.Check(newMachine.Jobs(), jc.DeepEquals, oldMachine.Jobs())
	c.Check(newMachine.Life(), gc.Equals, oldMachine.Life())
	newTools, err := newMachine.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	oldTools, err := oldMachine.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(newTools, jc.DeepEquals, oldTools)

	oldStatus, err := oldMachine.Status()
	c.Assert(err, jc.ErrorIsNil)
	newStatus, err := newMachine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(newStatus, jc.DeepEquals, oldStatus)

	oldInstID, oldInstDisplayName, err := oldMachine.InstanceNames()
	c.Assert(err, jc.ErrorIsNil)
	newInstID, newInstDisplayName, err := newMachine.InstanceNames()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(newInstID, gc.Equals, oldInstID)
	c.Check(newInstDisplayName, gc.Equals, oldInstDisplayName)

	oldStatus, err = oldMachine.InstanceStatus()
	c.Assert(err, jc.ErrorIsNil)
	newStatus, err = newMachine.InstanceStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(newStatus, jc.DeepEquals, oldStatus)
}

func (s *MigrationImportSuite) TestMachines(c *gc.C) {
	// Add a machine with an LXC container.
	cons := constraints.MustParse("arch=amd64 mem=8G root-disk-source=bunyan")
	source := "bunyan"
	displayName := "test-display-name"

	addr := network.NewSpaceAddress("1.1.1.1")
	addr.SpaceID = "9"

	machine1 := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: cons,
		Characteristics: &instance.HardwareCharacteristics{
			Arch:           cons.Arch,
			Mem:            cons.Mem,
			RootDiskSource: &source,
		},
		DisplayName: displayName,
		Addresses:   network.SpaceAddresses{addr},
	})
	err := s.Model.SetAnnotations(machine1, testAnnotations)
	c.Assert(err, jc.ErrorIsNil)
	s.primeStatusHistory(c, machine1, status.Started, 5)

	// machine1 should have some instance data.
	hardware, err := machine1.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hardware, gc.NotNil)

	_ = s.Factory.MakeMachineNested(c, machine1.Id(), nil)
	_ = s.Factory.MakeMachineNested(c, machine1.Id(), &factory.MachineParams{
		Constraints: constraints.MustParse("arch=arm64"),
		Characteristics: &instance.HardwareCharacteristics{
			Arch: cons.Arch,
		},
	})

	allMachines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allMachines, gc.HasLen, 3)

	newModel, newSt := s.importModel(c, s.State)

	importedMachines, err := newSt.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedMachines, gc.HasLen, 3)

	// AllMachines returns the machines in the same order, yay us.
	for i, newMachine := range importedMachines {
		s.AssertMachineEqual(c, newMachine, allMachines[i])
	}

	// And a few extra checks.
	parent := importedMachines[0]
	containers, err := parent.Containers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containers, jc.SameContents, []string{importedMachines[1].Id(), importedMachines[2].Id()})
	for _, cont := range []*state.Machine{importedMachines[1], importedMachines[2]} {
		parentId, isContainer := cont.ParentId()
		c.Assert(parentId, gc.Equals, parent.Id())
		c.Assert(isContainer, jc.IsTrue)
	}

	s.assertAnnotations(c, newModel, parent)
	s.checkStatusHistory(c, machine1, parent, 5)

	newCons, err := parent.Constraints()
	if c.Check(err, jc.ErrorIsNil) {
		// Can't test the constraints directly, so go through the string repr.
		c.Check(newCons.String(), gc.Equals, cons.String())
	}

	// Test the modification status is set to the initial state.
	modStatus, err := parent.ModificationStatus()
	if c.Check(err, jc.ErrorIsNil) {
		c.Check(modStatus.Status, gc.Equals, status.Idle)
	}

	characteristics, err := parent.HardwareCharacteristics()
	if c.Check(err, jc.ErrorIsNil) {
		c.Check(*characteristics.RootDiskSource, gc.Equals, "bunyan")
	}
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

func (s *MigrationImportSuite) TestMachinePortOps(c *gc.C) {
	ctrl, mockMachine := setupMockOpenedPortRanges(c, "3")
	defer ctrl.Finish()

	ops, err := state.MachinePortOps(s.State, mockMachine)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.HasLen, 1)
	c.Assert(ops[0].Id, gc.Equals, "3")
}

func (s *MigrationImportSuite) ApplicationPortOps(c *gc.C) {
	ctrl := gomock.NewController(c)
	mockApplication := mocks.NewMockApplication(ctrl)
	mockApplicationPortRanges := mocks.NewMockPortRanges(ctrl)

	aExp := mockApplication.EXPECT()
	aExp.Name().Return("gitlab")
	aExp.OpenedPortRanges().Return(mockApplicationPortRanges)

	opExp := mockApplicationPortRanges.EXPECT()
	opExp.ByUnit().Return(nil)

	ops, err := state.ApplicationPortOps(s.State, mockApplication)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.HasLen, 1)
	c.Assert(ops[0].Id, gc.Equals, "gitlab")
}

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/description_mock.go github.com/juju/description/v9 Application,Machine,PortRanges,UnitPortRanges
func setupMockOpenedPortRanges(c *gc.C, mID string) (*gomock.Controller, *mocks.MockMachine) {
	ctrl := gomock.NewController(c)
	mockMachine := mocks.NewMockMachine(ctrl)
	mockMachinePortRanges := mocks.NewMockPortRanges(ctrl)

	mExp := mockMachine.EXPECT()
	mExp.Id().Return(mID)
	mExp.OpenedPortRanges().Return(mockMachinePortRanges)

	opExp := mockMachinePortRanges.EXPECT()
	opExp.ByUnit().Return(nil)

	return ctrl, mockMachine
}

func (s *MigrationImportSuite) setupSourceApplications(
	c *gc.C, st *state.State, cons constraints.Value,
	platform *state.Platform, primeStatusHistory bool,
) (*state.Charm, *state.Application, string) {
	// Add a application with charm settings, app config, and leadership settings.
	f := factory.NewFactory(st, s.StatePool)

	serverSpace, err := s.State.AddSpace("server", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	exposedSpaceIDs := []string{serverSpace.Id()}

	testModel, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	series := "quantal"
	if testModel.Type() == state.ModelTypeCAAS {
		series = "kubernetes"
		exposedSpaceIDs = nil
	}
	// Add a application with charm settings, app config, and leadership settings.
	testCharm := f.MakeCharm(c, &factory.CharmParams{
		Name:   "starsay", // it has resources
		Series: series,
	})
	c.Assert(testCharm.Meta().Resources, gc.HasLen, 3)
	application, pwd := f.MakeApplicationReturningPassword(c, &factory.ApplicationParams{
		Charm: testCharm,
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-hub",
			Type:     "charm",
			Revision: &charm.MustParseURL(testCharm.URL()).Revision,
			Channel: &state.Channel{
				Risk: "edge",
			},
			Hash:     "some-hash",
			ID:       "some-id",
			Platform: platform,
		},
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
	err = application.MergeExposeSettings(map[string]state.ExposedEndpoint{
		"admin": {
			ExposeToSpaceIDs: exposedSpaceIDs,
			ExposeToCIDRs:    []string{"13.37.0.0/16"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(imported.IsExposed(), gc.Equals, exported.IsExposed())
	c.Assert(imported.ExposedEndpoints(), gc.DeepEquals, exported.ExposedEndpoints())
	c.Assert(imported.MetricCredentials(), jc.DeepEquals, exported.MetricCredentials())
	c.Assert(imported.PasswordValid(pwd), jc.IsTrue)
	exportedOrigin := exported.CharmOrigin()
	if corecharm.CharmHub.Matches(exportedOrigin.Source) && exportedOrigin.Channel.Track == "" {
		exportedOrigin.Channel.Track = "latest"
	}
	c.Assert(imported.CharmOrigin(), jc.DeepEquals, exportedOrigin)

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

	rSt := newSt.Resources()
	resources, err := rSt.ListResources(imported.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resources.Resources, gc.HasLen, 3)

	if newModel.Type() == state.ModelTypeCAAS {
		agentTools := version.Binary{
			Number:  jujuversion.Current,
			Arch:    arch.HostArch(),
			Release: application.CharmOrigin().Platform.OS,
		}

		tools, err := imported.AgentTools()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(tools.Version, gc.Equals, agentTools)
	}
}

func (s *MigrationImportSuite) TestApplications(c *gc.C) {
	cons := constraints.MustParse("arch=amd64 mem=8G root-disk-source=tralfamadore")
	platform := &state.Platform{Architecture: arch.DefaultArchitecture, OS: "ubuntu", Channel: "12.10/stable"}
	testCharm, application, pwd := s.setupSourceApplications(c, s.State, cons, platform, true)

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
		URL:      testCharm.URL(),
		Revision: strconv.Itoa(testCharm.Revision()),
	})
	s.assertImportedApplication(c, application, pwd, cons, exported, newModel, newSt, true)
}

func (s *MigrationImportSuite) TestApplicationsUpdateSeriesNotPlatform(c *gc.C) {
	// The application series should be quantal, the origin platform series should
	// be focal.  After migration, the platform series should be quantal as well.
	cons := constraints.MustParse("arch=amd64 mem=8G root-disk-source=tralfamadore")
	platform := &state.Platform{
		Architecture: arch.DefaultArchitecture,
		OS:           "ubuntu",
		Channel:      "20.04/stable",
	}
	_, _, _ = s.setupSourceApplications(c, s.State, cons, platform, true)

	allApplications, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allApplications, gc.HasLen, 1)
	exportedApp := allApplications[0]
	origin := exportedApp.CharmOrigin()
	c.Check(origin, gc.NotNil)
	c.Check(origin.Platform, gc.NotNil)
	c.Check(origin.Platform.Channel, gc.Equals, "20.04/stable")

	_, newSt := s.importModel(c, s.State)

	obtainedApp, err := newSt.Application("starsay")
	c.Assert(err, jc.ErrorIsNil)
	obtainedOrigin := obtainedApp.CharmOrigin()
	c.Assert(obtainedOrigin, gc.NotNil)
	c.Assert(obtainedOrigin.Platform, gc.NotNil)
	c.Assert(obtainedOrigin.Platform.Architecture, gc.Equals, arch.DefaultArchitecture)
	c.Assert(obtainedOrigin.Platform.OS, gc.Equals, "ubuntu")
	c.Assert(obtainedOrigin.Platform.Channel, gc.Equals, "20.04/stable")
}

func (s *MigrationImportSuite) TestCharmhubApplicationCharmOriginNormalised(c *gc.C) {
	platform := &state.Platform{Architecture: arch.DefaultArchitecture, OS: "ubuntu", Channel: "12.10/stable"}
	f := factory.NewFactory(s.State, s.StatePool)

	testCharm := f.MakeCharm(c, &factory.CharmParams{Revision: "8", URL: "ch:mysql-8"})
	wrongRev := 4
	_ = f.MakeApplication(c, &factory.ApplicationParams{
		Charm: testCharm,
		Name:  "mysql",
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-hub",
			Type:     "charm",
			Platform: platform,
			Revision: &wrongRev,
			Channel:  &state.Channel{Track: "20.04", Risk: "stable", Branch: "deadbeef"},
			Hash:     "some-hash",
			ID:       "some-id",
		},
	})

	_, newSt := s.importModel(c, s.State)
	newApp, err := newSt.Application("mysql")
	c.Assert(err, jc.ErrorIsNil)
	rev := 8
	c.Assert(newApp.CharmOrigin(), gc.DeepEquals, &state.CharmOrigin{
		Source:   "charm-hub",
		Type:     "charm",
		Platform: platform,
		Revision: &rev,
		Channel:  &state.Channel{Track: "20.04", Risk: "stable", Branch: "deadbeef"},
		Hash:     "some-hash",
		ID:       "some-id",
	})
}

func (s *MigrationImportSuite) TestLocalApplicationCharmOriginNormalised(c *gc.C) {
	platform := &state.Platform{Architecture: arch.DefaultArchitecture, OS: "ubuntu", Channel: "12.10/stable"}
	f := factory.NewFactory(s.State, s.StatePool)

	testCharm := f.MakeCharm(c, &factory.CharmParams{Revision: "8", URL: "local:mysql-8"})
	wrongRev := 4
	_ = f.MakeApplication(c, &factory.ApplicationParams{
		Charm: testCharm,
		Name:  "mysql",
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-hub",
			Type:     "charm",
			Platform: platform,
			Revision: &wrongRev,
			Channel:  &state.Channel{Track: "20.04", Risk: "stable", Branch: "deadbeef"},
			Hash:     "some-hash",
			ID:       "some-id",
		},
	})

	_, newSt := s.importModel(c, s.State)
	newApp, err := newSt.Application("mysql")
	c.Assert(err, jc.ErrorIsNil)
	rev := 8
	c.Assert(newApp.CharmOrigin(), gc.DeepEquals, &state.CharmOrigin{
		Source:   "local",
		Type:     "charm",
		Platform: platform,
		Revision: &rev,
	})
}

func (s *MigrationImportSuite) TestApplicationStatus(c *gc.C) {
	cons := constraints.MustParse("arch=amd64 mem=8G")
	platform := &state.Platform{Architecture: arch.DefaultArchitecture, OS: "ubuntu", Channel: "12.10/stable"}
	testCharm, application, pwd := s.setupSourceApplications(c, s.State, cons, platform, false)

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
		URL:      testCharm.URL(),
		Revision: strconv.Itoa(testCharm.Revision()),
	})
	s.assertImportedApplication(c, application, pwd, cons, exported, newModel, newSt, false)
	newApp, err := newSt.Application(application.Name())
	c.Assert(err, jc.ErrorIsNil)
	// Has unset application status.
	appStatus, err := newApp.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appStatus.Status, gc.Equals, status.Unset)
	c.Assert(appStatus.Message, gc.Equals, "")
}

func (s *MigrationImportSuite) TestCAASApplications(c *gc.C) {
	caasSt := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *gc.C) { caasSt.Close() })

	cons := constraints.MustParse("arch=amd64 mem=8G")
	platform := &state.Platform{Architecture: arch.DefaultArchitecture, OS: "ubuntu", Channel: "20.04/stable"}
	charm, application, pwd := s.setupSourceApplications(c, caasSt, cons, platform, true)

	model, err := caasSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	caasModel, err := model.CAASModel()
	c.Assert(err, jc.ErrorIsNil)
	err = caasModel.SetPodSpec(nil, application.ApplicationTag(), strPtr("pod spec"))
	c.Assert(err, jc.ErrorIsNil)
	addr := network.NewSpaceAddress("192.168.1.1", network.WithScope(network.ScopeCloudLocal))
	addr.SpaceID = "0"
	err = application.UpdateCloudService("provider-id", []network.SpaceAddress{addr})
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
		Series:   "kubernetes",
		URL:      charm.URL(),
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
	c.Assert(cloudService.Addresses(), jc.DeepEquals, network.SpaceAddresses{addr})
	c.Assert(newApp.GetScale(), gc.Equals, 3)
	c.Assert(newApp.GetPlacement(), gc.Equals, "")
	c.Assert(state.GetApplicationHasResources(newApp), jc.IsTrue)
}

func (s *MigrationImportSuite) TestCAASApplicationStatus(c *gc.C) {
	// Caas application status that is derived from unit statuses must survive migration.
	caasSt := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *gc.C) { caasSt.Close() })

	cons := constraints.MustParse("arch=amd64 mem=8G")
	platform := &state.Platform{Architecture: arch.DefaultArchitecture, OS: "ubuntu", Channel: "20.04"}
	testCharm, application, _ := s.setupSourceApplications(c, caasSt, cons, platform, false)
	ss, err := application.Status()
	c.Assert(err, jc.ErrorIsNil)
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
	err = caasModel.SetPodSpec(nil, application.ApplicationTag(), strPtr("pod spec"))
	c.Assert(err, jc.ErrorIsNil)
	addr := network.NewSpaceAddress("192.168.1.1", network.WithScope(network.ScopeCloudLocal))
	err = application.UpdateCloudService("provider-id", []network.SpaceAddress{addr})
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
		Series:   "kubernetes",
		URL:      testCharm.URL(),
		Revision: strconv.Itoa(testCharm.Revision()),
	})
	newApp, err := newSt.Application(application.Name())
	c.Assert(err, jc.ErrorIsNil)
	// Has unset application status.
	appStatus, err := newApp.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appStatus.Status, gc.Equals, status.Unset)
	c.Assert(appStatus.Message, gc.Equals, "")
}

func (s *MigrationImportSuite) TestApplicationsWithExposedOffers(c *gc.C) {
	_ = s.Factory.MakeUser(c, &factory.UserParams{Name: "admin"})
	fooUser := s.Factory.MakeUser(c, &factory.UserParams{Name: "foo"})
	serverSpace, err := s.State.AddSpace("server", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)

	testCharm := s.AddTestingCharm(c, "mysql")
	application := s.AddTestingApplicationWithBindings(c, "mysql",
		testCharm,
		map[string]string{
			"server": serverSpace.Id(),
		},
	)
	applicationEP, err := application.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(wordpressEP, applicationEP)
	c.Assert(err, jc.ErrorIsNil)

	stOffers := state.NewApplicationOffers(s.State)
	stOffer, err := stOffers.AddOffer(
		crossmodel.AddApplicationOfferArgs{
			OfferName:              "my-offer",
			Owner:                  "admin",
			ApplicationDescription: "my app",
			ApplicationName:        application.Name(),
			Endpoints: map[string]string{
				"server": serverSpace.Name(),
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// Allow "foo" to consume offer
	err = s.State.CreateOfferAccess(
		names.NewApplicationOfferTag(stOffer.OfferUUID),
		fooUser.UserTag(),
		permission.ConsumeAccess,
	)
	c.Assert(err, jc.ErrorIsNil)

	stateOffers := state.NewApplicationOffers(s.State)
	exportedOffers, err := stateOffers.AllApplicationOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exportedOffers, gc.HasLen, 1)
	exported := exportedOffers[0]

	_, newSt := s.importModel(c, s.State, func(_ map[string]interface{}) {
		// Application offer permissions are keyed on the offer uuid
		// rather than a model uuid.
		// If we import and the permissions still exist, the txn will fail
		// as imports all assume any records do not already exist.
		err = s.State.RemoveOfferAccess(
			names.NewApplicationOfferTag(stOffer.OfferUUID),
			fooUser.UserTag(),
		)
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.RemoveOfferAccess(
			names.NewApplicationOfferTag(stOffer.OfferUUID),
			names.NewUserTag("admin"),
		)
		c.Assert(err, jc.ErrorIsNil)
	})

	// The following is required because we don't add charms during an import,
	// these are added at a later date. When constructing an application offer,
	// the charm is required for the charm.Relation, so we need to inject it
	// into the new state.
	state.AddTestingCharm(c, newSt, "mysql")

	newStateOffers := state.NewApplicationOffers(newSt)
	importedOffers, err := newStateOffers.AllApplicationOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedOffers, gc.HasLen, 1)
	imported := importedOffers[0]
	c.Assert(exported, gc.DeepEquals, imported)

	users, err := newSt.GetOfferUsers(stOffer.OfferUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(users, gc.HasLen, 2)
	c.Assert(users, gc.DeepEquals, map[string]permission.Access{
		"admin": "admin",
		"foo":   "consume",
	})
}

func (s *MigrationImportSuite) TestExternalControllers(c *gc.C) {
	remoteApp, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "gravy-rainbow",
		URL:         "me/model.rainbow",
		SourceModel: s.Model.ModelTag(),
		Token:       "charisma",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = remoteApp.SetStatus(status.StatusInfo{Status: status.Active})
	c.Assert(err, jc.ErrorIsNil)

	stateExternalCtrl := state.NewExternalControllers(s.State)
	crossModelController, err := stateExternalCtrl.Save(crossmodel.ControllerInfo{
		ControllerTag: s.Model.ControllerTag(),
		Addrs:         []string{"192.168.1.1:8080"},
		Alias:         "magic",
		CACert:        "magic-ca-cert",
	}, s.Model.UUID())
	c.Assert(err, jc.ErrorIsNil)

	stateExternalController, err := s.State.ExternalControllerForModel(s.Model.UUID())
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State, func(map[string]interface{}) {
		err := stateExternalCtrl.Remove(s.Model.ControllerTag().Id())
		c.Assert(err, jc.ErrorIsNil)
	})

	newExternalCtrl := state.NewExternalControllers(newSt)

	newCtrl, err := newExternalCtrl.ControllerForModel(s.Model.UUID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newCtrl.ControllerInfo(), jc.DeepEquals, crossModelController.ControllerInfo())

	newExternalController, err := newSt.ExternalControllerForModel(s.Model.UUID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stateExternalController, gc.DeepEquals, newExternalController)
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

	out, err := s.State.Export(map[string]string{})
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
	c.Assert(err, jc.ErrorIsNil)
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

	out, err := s.State.Export(map[string]string{})
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

func (s *MigrationImportSuite) TestUnitWithoutAnyPersistedState(c *gc.C) {
	f := factory.NewFactory(s.State, s.StatePool)

	// Export unit without any controller-persisted state
	exported := f.MakeUnit(c, &factory.UnitParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})

	exportedState, err := exported.State()
	c.Assert(err, jc.ErrorIsNil)
	_, isSet := exportedState.CharmState()
	c.Assert(isSet, jc.IsFalse, gc.Commentf("expected charm state to be empty"))
	_, isSet = exportedState.RelationState()
	c.Assert(isSet, jc.IsFalse, gc.Commentf("expected uniter relation state to be empty"))
	_, isSet = exportedState.UniterState()
	c.Assert(isSet, jc.IsFalse, gc.Commentf("expected uniter state to be empty"))
	_, isSet = exportedState.StorageState()
	c.Assert(isSet, jc.IsFalse, gc.Commentf("expected uniter storage state to be empty"))
	_, isSet = exportedState.MeterStatusState()
	c.Assert(isSet, jc.IsFalse, gc.Commentf("expected meter status state to be empty"))

	// Import model and ensure that its UnitState was not mutated.
	_, newSt := s.importModel(c, s.State)

	importedApplications, err := newSt.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedApplications, gc.HasLen, 1)

	importedUnits, err := importedApplications[0].AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedUnits, gc.HasLen, 1)
	imported := importedUnits[0]

	c.Assert(imported.UnitTag(), gc.Equals, exported.UnitTag())

	unitState, err := imported.State()
	c.Assert(err, jc.ErrorIsNil)
	_, isSet = unitState.CharmState()
	c.Assert(isSet, jc.IsFalse, gc.Commentf("unexpected charm state after import; SetState should not have been called"))
	_, isSet = unitState.RelationState()
	c.Assert(isSet, jc.IsFalse, gc.Commentf("unexpected uniter relation state after import; SetState should not have been called"))
	_, isSet = unitState.UniterState()
	c.Assert(isSet, jc.IsFalse, gc.Commentf("unexpected uniter state after import; SetState should not have been called"))
	_, isSet = unitState.StorageState()
	c.Assert(isSet, jc.IsFalse, gc.Commentf("unexpected uniter storage state after import; SetState should not have been called"))
	_, isSet = unitState.MeterStatusState()
	c.Assert(isSet, jc.IsFalse, gc.Commentf("unexpected meter status state after import; SetState should not have been called"))
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
	us := state.NewUnitState()
	us.SetCharmState(map[string]string{"payload": "0xb4c0ffee"})
	us.SetRelationState(map[int]string{42: "magic"})
	us.SetUniterState("uniter state")
	us.SetStorageState("storage state")
	us.SetMeterStatusState("meter status state")
	err = exported.SetState(us, state.UnitStateSizeLimits{})
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
		addr := network.NewSpaceAddress("192.168.1.2", network.WithScope(network.ScopeMachineLocal))
		addr.SpaceID = "0"
		c.Assert(containerInfo.Address(), jc.DeepEquals, &addr)
	}

	meterStatus, err := imported.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meterStatus, gc.Equals, state.MeterStatus{state.MeterGreen, "some info"})
	s.assertAnnotations(c, newModel, imported)
	s.checkStatusHistory(c, exported, imported, 5)
	s.checkStatusHistory(c, exported.Agent(), imported.Agent(), 5)
	s.checkStatusHistory(c, exported.WorkloadVersionHistory(), imported.WorkloadVersionHistory(), 1)

	unitState, err := imported.State()
	c.Assert(err, jc.ErrorIsNil)
	charmState, _ := unitState.CharmState()
	c.Assert(charmState, jc.DeepEquals, map[string]string{"payload": "0xb4c0ffee"}, gc.Commentf("persisted charm state not migrated"))
	relationState, _ := unitState.RelationState()
	c.Assert(relationState, jc.DeepEquals, map[int]string{42: "magic"}, gc.Commentf("persisted relation state not migrated"))
	uniterState, _ := unitState.UniterState()
	c.Assert(uniterState, gc.Equals, "uniter state", gc.Commentf("persisted uniter state not migrated"))
	storageState, _ := unitState.StorageState()
	c.Assert(storageState, gc.Equals, "storage state", gc.Commentf("persisted uniter storage state not migrated"))
	meterStatusState, _ := unitState.MeterStatusState()
	c.Assert(meterStatusState, gc.Equals, "meter status state", gc.Commentf("persisted meter status state not migrated"))

	newCons, err := imported.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	// Can't test the constraints directly, so go through the string repr.
	c.Assert(newCons.String(), gc.Equals, cons.String())
}

func (s *MigrationImportSuite) TestRemoteEntities(c *gc.C) {
	srcRemoteEntities := s.State.RemoteEntities()
	err := srcRemoteEntities.ImportRemoteEntity(names.NewApplicationTag("uuid3"), "xxx-aaa-bbb")
	c.Assert(err, jc.ErrorIsNil)
	err = srcRemoteEntities.ImportRemoteEntity(names.NewApplicationTag("uuid4"), "ccc-ddd-zzz")
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State)

	newRemoteEntities := newSt.RemoteEntities()
	token, err := newRemoteEntities.GetToken(names.NewApplicationTag("uuid3"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(token, gc.Equals, "xxx-aaa-bbb")

	token, err = newRemoteEntities.GetToken(names.NewApplicationTag("uuid4"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(token, gc.Equals, "ccc-ddd-zzz")
}

func (s *MigrationImportSuite) TestRelationNetworks(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	srcRelationNetworks := state.NewRelationIngressNetworks(s.State)
	_, err = srcRelationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State)

	newRelationNetworks := state.NewRelationNetworks(newSt)
	networks, err := newRelationNetworks.AllRelationNetworks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(networks, gc.HasLen, 1)

	entity0 := networks[0]
	c.Assert(entity0.RelationKey(), gc.Equals, "wordpress:db mysql:server")
	c.Assert(entity0.CIDRS(), gc.DeepEquals, []string{"192.168.1.0/16"})
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

	wordpress0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})
	ru, err := rel.Unit(wordpress0)
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

func (s *MigrationImportSuite) TestCMRRemoteRelationScope(c *gc.C) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "gravy-rainbow",
		URL:         "me/model.rainbow",
		SourceModel: s.Model.ModelTag(),
		Token:       "charisma",
		OfferUUID:   "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	wordpress := state.AddTestingApplication(c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	eps, err := s.State.InferEndpoints("gravy-rainbow", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	wordpress0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})
	localRU, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)

	wordpressSettings := map[string]interface{}{"name": "wordpress/0"}
	err = localRU.EnterScope(wordpressSettings)
	c.Assert(err, jc.ErrorIsNil)

	remoteRU, err := rel.RemoteUnit("gravy-rainbow/0")
	c.Assert(err, jc.ErrorIsNil)

	gravySettings := map[string]interface{}{"name": "gravy-rainbow/0"}
	err = remoteRU.EnterScope(gravySettings)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State)

	newWordpress, err := newSt.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)

	rels, err := newWordpress.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rels, gc.HasLen, 1)

	ru, err := rels[0].RemoteUnit("gravy-rainbow/0")
	c.Assert(err, jc.ErrorIsNil)

	inScope, err := ru.InScope()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(inScope, jc.IsTrue)
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
	space := s.Factory.MakeSpace(c, &factory.SpaceParams{
		Name: "one", ProviderID: "provider", IsPublic: true})
	state.AddTestingApplicationWithBindings(
		c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"),
		map[string]string{"db": space.Id()})

	_, newSt := s.importModel(c, s.State)

	newWordpress, err := newSt.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)

	bindings, err := newWordpress.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	// Check the "db" endpoint has the correct space ID, the others
	// should have the AlphaSpaceId
	c.Assert(bindings.Map()["db"], gc.Equals, space.Id())
	c.Assert(bindings.Map()[""], gc.Equals, network.AlphaSpaceId)
}

func (s *MigrationImportSuite) TestIncompleteEndpointBindings(c *gc.C) {
	// Ensure we handle the case coming from an early 2.7 controller
	// where the default binding is missing.
	space := s.Factory.MakeSpace(c, &factory.SpaceParams{
		Name: "one", ProviderID: "provider", IsPublic: true})
	state.AddTestingApplicationWithBindings(
		c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"),
		map[string]string{"db": space.Id()})

	_, newSt := s.importModel(c, s.State, func(desc map[string]interface{}) {
		apps := desc["applications"].(map[interface{}]interface{})
		for _, item := range apps["applications"].([]interface{}) {
			bindings, ok := item.(map[interface{}]interface{})["endpoint-bindings"].(map[interface{}]interface{})
			if !ok {
				continue
			}
			delete(bindings, "")
		}
	})

	newWordpress, err := newSt.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)

	bindings, err := newWordpress.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bindings.Map()["db"], gc.Equals, space.Id())
	c.Assert(bindings.Map()[""], gc.Equals, network.AlphaSpaceId)
}

func (s *MigrationImportSuite) TestNilEndpointBindings(c *gc.C) {
	app := state.AddTestingApplicationWithEmptyBindings(
		c, s.State, "dummy", state.AddTestingCharm(c, s.State, "dummy"))

	bindings, err := app.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bindings.Map(), gc.HasLen, 0)

	_, newSt := s.importModel(c, s.State)

	newApp, err := newSt.Application("dummy")
	c.Assert(err, jc.ErrorIsNil)

	newBindings, err := newApp.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newBindings.Map()[""], gc.Equals, network.AlphaSpaceId)
}

func (s *MigrationImportSuite) TestUnitsOpenPorts(c *gc.C) {
	unit := s.Factory.MakeUnit(c, nil)

	unitPortRanges, err := unit.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	unitPortRanges.Open(allEndpoints, network.MustParsePortRange("1234-2345/tcp"))
	c.Assert(s.State.ApplyOperation(unitPortRanges.Changes()), jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State)

	// Even though the opened ports document is stored with the
	// machine, the only way to easily access it is through the units.
	imported, err := newSt.Unit(unit.Name())
	c.Assert(err, jc.ErrorIsNil)

	unitPortRanges, err = imported.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitPortRanges.UniquePortRanges(), gc.HasLen, 1)

	portRanges := unitPortRanges.ForEndpoint(allEndpoints)
	c.Assert(portRanges, gc.HasLen, 1)
	c.Assert(portRanges[0], gc.Equals, network.PortRange{
		FromPort: 1234,
		ToPort:   2345,
		Protocol: "tcp",
	})
}

func (s *MigrationImportSuite) TestSpaces(c *gc.C) {
	space := s.Factory.MakeSpace(c, &factory.SpaceParams{
		Name: "one", ProviderID: network.Id("provider"), IsPublic: true})

	spaceNoID := s.Factory.MakeSpace(c, &factory.SpaceParams{
		Name: "no-id", ProviderID: network.Id("provider2"), IsPublic: true})

	// Blank the ID from the second space to check that import creates it.
	_, newSt := s.importModel(c, s.State, func(desc map[string]interface{}) {
		spaces := desc["spaces"].(map[interface{}]interface{})
		for _, item := range spaces["spaces"].([]interface{}) {
			sp := item.(map[interface{}]interface{})
			if sp["name"] == spaceNoID.Name() {
				sp["id"] = ""
			}
		}
	})

	imported, err := newSt.SpaceByName(space.Name())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(imported.Id(), gc.Equals, space.Id())
	c.Check(imported.Name(), gc.Equals, space.Name())
	c.Check(imported.ProviderId(), gc.Equals, space.ProviderId())
	c.Check(imported.IsPublic(), gc.Equals, space.IsPublic())

	imported, err = newSt.SpaceByName(spaceNoID.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(imported.Id(), gc.Not(gc.Equals), "")
}

func (s *MigrationImportSuite) TestFirewallRules(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	sshCIDRs := []string{"192.168.1.0/24", "192.0.2.1/24"}
	sshRule := state.NewMockFirewallRule(ctrl)
	sshRule.EXPECT().WellKnownService().Return("ssh")
	sshRule.EXPECT().WhitelistCIDRs().Return(sshCIDRs)

	saasCIDRs := []string{"10.0.0.0/16"}
	saasRule := state.NewMockFirewallRule(ctrl)
	saasRule.EXPECT().WellKnownService().Return("juju-application-offer")
	saasRule.EXPECT().WhitelistCIDRs().Return(saasCIDRs)

	base, err := s.State.Export(map[string]string{})
	c.Assert(err, jc.ErrorIsNil)
	uuid := utils.MustNewUUID().String()
	model := newModel(base, uuid, "new")
	model.fwRules = []description.FirewallRule{sshRule, saasRule}

	_, newSt := s.importModelDescription(c, model)

	m, err := newSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := m.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cfg.SSHAllow(), gc.DeepEquals, sshCIDRs)
	c.Assert(cfg.SAASIngressAllow(), gc.DeepEquals, saasCIDRs)
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
		Name:            "foo",
		Type:            network.EthernetDevice,
		VirtualPortType: network.OvsPort,
	}
	err := machine.SetLinkLayerDevices(deviceArgs)
	c.Assert(err, jc.ErrorIsNil)
	_, newSt := s.importModel(c, s.State)

	devices, err := newSt.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devices, gc.HasLen, 1)
	device := devices[0]
	c.Assert(device.Name(), gc.Equals, "foo")
	c.Assert(device.Type(), gc.Equals, network.EthernetDevice)
	c.Assert(device.VirtualPortType(), gc.Equals, network.OvsPort, gc.Commentf("VirtualPortType not migrated correctly"))
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
		Type: network.BridgeDevice,
	}, {
		Name:       "bar",
		ParentName: "foo",
		Type:       network.EthernetDevice,
	}}
	for _, args := range deviceArgs {
		err := machine.SetLinkLayerDevices(args)
		c.Assert(err, jc.ErrorIsNil)
	}
	machine2DeviceArgs := state.LinkLayerDeviceArgs{
		Name:       "baz",
		ParentName: fmt.Sprintf("m#%v#d#foo", machine.Id()),
		Type:       network.EthernetDevice,
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
	sp, err := s.State.AddSpace("bam", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	original, err := s.State.AddSubnet(network.SubnetInfo{
		CIDR:              "10.0.0.0/24",
		ProviderId:        "foo",
		ProviderNetworkId: "elm",
		VLANTag:           64,
		SpaceID:           sp.Id(),
		AvailabilityZones: []string{"bar"},
		IsPublic:          true,
	})
	c.Assert(err, jc.ErrorIsNil)
	originalNoID, err := s.State.AddSubnet(network.SubnetInfo{
		CIDR:              "10.76.0.0/24",
		ProviderId:        "bar",
		ProviderNetworkId: "oak",
		VLANTag:           64,
		SpaceID:           sp.Id(),
		AvailabilityZones: []string{"bar"},
	})
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State, func(desc map[string]interface{}) {
		subnets := desc["subnets"].(map[interface{}]interface{})
		for _, item := range subnets["subnets"].([]interface{}) {
			sn := item.(map[interface{}]interface{})

			if sn["subnet-id"] == originalNoID.ID() {
				// Remove the subnet ID, to check that it is created by import.
				sn["subnet-id"] = ""

				// Swap the space ID for a space name to check migrating from
				// a pre-2.7 model.
				sn["space-id"] = ""
				sn["space-name"] = sp.Name()
			}
		}
	})

	subnet, err := newSt.Subnet(original.ID())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnet.CIDR(), gc.Equals, "10.0.0.0/24")
	c.Assert(subnet.ProviderId(), gc.Equals, network.Id("foo"))
	c.Assert(subnet.ProviderNetworkId(), gc.Equals, network.Id("elm"))
	c.Assert(subnet.VLANTag(), gc.Equals, 64)
	c.Assert(subnet.AvailabilityZones(), gc.DeepEquals, []string{"bar"})
	c.Assert(subnet.SpaceID(), gc.Equals, sp.Id())
	c.Assert(subnet.FanLocalUnderlay(), gc.Equals, "")
	c.Assert(subnet.FanOverlay(), gc.Equals, "")
	c.Assert(subnet.IsPublic(), gc.Equals, true)

	imported, err := newSt.SubnetByCIDR(originalNoID.CIDR())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(imported, gc.Not(gc.Equals), "")
}

func (s *MigrationImportSuite) TestSubnetsWithFan(c *gc.C) {
	subnet, err := s.State.AddSubnet(network.SubnetInfo{
		CIDR: "100.2.0.0/16",
	})
	c.Assert(err, jc.ErrorIsNil)
	sp, err := s.State.AddSpace("bam", "", []string{subnet.ID()}, true)
	c.Assert(err, jc.ErrorIsNil)

	sn := network.SubnetInfo{
		CIDR:              "10.0.0.0/24",
		ProviderId:        network.Id("foo"),
		ProviderNetworkId: network.Id("elm"),
		VLANTag:           64,
		AvailabilityZones: []string{"bar"},
	}
	sn.SetFan("100.2.0.0/16", "253.0.0.0/8")

	original, err := s.State.AddSubnet(sn)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State)

	subnet, err = newSt.SubnetByCIDR(original.CIDR())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnet.CIDR(), gc.Equals, "10.0.0.0/24")
	c.Assert(subnet.ProviderId(), gc.Equals, network.Id("foo"))
	c.Assert(subnet.ProviderNetworkId(), gc.Equals, network.Id("elm"))
	c.Assert(subnet.VLANTag(), gc.Equals, 64)
	c.Assert(subnet.AvailabilityZones(), gc.DeepEquals, []string{"bar"})
	c.Assert(subnet.SpaceID(), gc.Equals, sp.Id())
	c.Assert(subnet.FanLocalUnderlay(), gc.Equals, "100.2.0.0/16")
	c.Assert(subnet.FanOverlay(), gc.Equals, "253.0.0.0/8")
}

func (s *MigrationImportSuite) TestIPAddress(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	space, err := s.State.AddSpace("testme", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSubnet(network.SubnetInfo{CIDR: "0.1.2.0/24", SpaceID: space.Id()})
	c.Assert(err, jc.ErrorIsNil)
	deviceArgs := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: network.EthernetDevice,
	}
	err = machine.SetLinkLayerDevices(deviceArgs)
	c.Assert(err, jc.ErrorIsNil)
	args := state.LinkLayerDeviceAddress{
		DeviceName:        "foo",
		ConfigMethod:      network.ConfigStatic,
		CIDRAddress:       "0.1.2.3/24",
		ProviderID:        "bar",
		DNSServers:        []string{"bam", "mam"},
		DNSSearchDomains:  []string{"weeee"},
		GatewayAddress:    "0.1.2.1",
		ProviderNetworkID: "p-net-id",
		ProviderSubnetID:  "p-sub-id",
		Origin:            network.OriginProvider,
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
	c.Assert(addr.ConfigMethod(), gc.Equals, network.ConfigStatic)
	c.Assert(addr.SubnetCIDR(), gc.Equals, "0.1.2.0/24")
	c.Assert(addr.ProviderID(), gc.Equals, network.Id("bar"))
	c.Assert(addr.DNSServers(), jc.DeepEquals, []string{"bam", "mam"})
	c.Assert(addr.DNSSearchDomains(), jc.DeepEquals, []string{"weeee"})
	c.Assert(addr.GatewayAddress(), gc.Equals, "0.1.2.1")
	c.Assert(addr.ProviderNetworkID().String(), gc.Equals, "p-net-id")
	c.Assert(addr.ProviderSubnetID().String(), gc.Equals, "p-sub-id")
	c.Assert(addr.Origin(), gc.Equals, network.OriginProvider)
}

func (s *MigrationImportSuite) TestIPAddressCompatibility(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})

	_, err := s.State.AddSubnet(network.SubnetInfo{CIDR: "0.1.2.0/24"})
	c.Assert(err, jc.ErrorIsNil)
	deviceArgs := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: network.EthernetDevice,
	}
	err = machine.SetLinkLayerDevices(deviceArgs)
	c.Assert(err, jc.ErrorIsNil)
	args := state.LinkLayerDeviceAddress{
		DeviceName:   "foo",
		ConfigMethod: "dynamic",
		CIDRAddress:  "0.1.2.3/24",
		Origin:       network.OriginProvider,
	}
	err = machine.SetDevicesAddresses(args)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State)

	addresses, _ := newSt.AllIPAddresses()
	c.Assert(addresses, gc.HasLen, 1)
	c.Assert(addresses[0].ConfigMethod(), gc.Equals, network.ConfigDHCP)
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
		Version:         "22.04",
		Arch:            "arch",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test",
		RootStorageSize: &storageSize,
		Source:          "test",
	}
	attrsCustom := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region-custom",
		Version:         "22.04",
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
	c.Check(image.Version, gc.Equals, "22.04")
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

	// pending action.
	operationIDPending, err := m.EnqueueOperation("a test", 2)
	c.Assert(err, jc.ErrorIsNil)
	actionPending, err := m.EnqueueAction(operationIDPending, machine.MachineTag(), "action-pending", nil, true, "group", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actionPending.Status(), gc.Equals, state.ActionPending)

	// running action.
	operationIDRunning, err := m.EnqueueOperation("another test", 2)
	c.Assert(err, jc.ErrorIsNil)
	actionRunning, err := m.EnqueueAction(operationIDRunning, machine.MachineTag(), "action-running", nil, true, "group", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actionRunning.Status(), gc.Equals, state.ActionPending)
	actionRunning, err = actionRunning.Begin()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actionRunning.Status(), gc.Equals, state.ActionRunning)

	// aborting action.
	operationIDAborting, err := m.EnqueueOperation("another test", 2)
	c.Assert(err, jc.ErrorIsNil)
	actionAborting, err := m.EnqueueAction(operationIDAborting, machine.MachineTag(), "action-aborting", nil, true, "group", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actionAborting.Status(), gc.Equals, state.ActionPending)
	actionAborting, err = actionAborting.Begin()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actionAborting.Status(), gc.Equals, state.ActionRunning)
	actionAborting, err = actionAborting.Finish(state.ActionResults{Status: state.ActionAborting})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actionAborting.Status(), gc.Equals, state.ActionAborting)

	// aborted action.
	operationIDAborted, err := m.EnqueueOperation("another test", 2)
	c.Assert(err, jc.ErrorIsNil)
	actionAborted, err := m.EnqueueAction(operationIDAborted, machine.MachineTag(), "action-aborted", nil, true, "group", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actionAborted.Status(), gc.Equals, state.ActionPending)
	actionAborted, err = actionAborted.Begin()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actionAborted.Status(), gc.Equals, state.ActionRunning)
	actionAborted, err = actionAborted.Finish(state.ActionResults{Status: state.ActionAborted})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actionAborted.Status(), gc.Equals, state.ActionAborted)

	// completed action.
	operationIDCompleted, err := m.EnqueueOperation("another test", 2)
	c.Assert(err, jc.ErrorIsNil)
	actionCompleted, err := m.EnqueueAction(operationIDCompleted, machine.MachineTag(), "action-completed", nil, true, "group", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actionCompleted.Status(), gc.Equals, state.ActionPending)
	actionCompleted, err = actionCompleted.Begin()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actionCompleted.Status(), gc.Equals, state.ActionRunning)
	actionCompleted, err = actionCompleted.Finish(state.ActionResults{Status: state.ActionCompleted})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actionCompleted.Status(), gc.Equals, state.ActionCompleted)

	newModel, newState := s.importModel(c, s.State)
	defer func() {
		c.Assert(newState.Close(), jc.ErrorIsNil)
	}()

	actions, err := newModel.AllActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actions, gc.HasLen, 5)

	actionPending, err = newModel.ActionByTag(actionPending.ActionTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(actionPending.Receiver(), gc.Equals, machine.Id())
	c.Check(actionPending.Name(), gc.Equals, "action-pending")
	c.Check(state.ActionOperationId(actionPending), gc.Equals, operationIDPending)
	c.Check(actionPending.Status(), gc.Equals, state.ActionPending)
	c.Check(actionPending.Parallel(), jc.IsTrue)
	c.Check(actionPending.ExecutionGroup(), gc.Equals, "group")

	actionRunning, err = newModel.ActionByTag(actionRunning.ActionTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(actionRunning.Receiver(), gc.Equals, machine.Id())
	c.Check(actionRunning.Name(), gc.Equals, "action-running")
	c.Check(state.ActionOperationId(actionRunning), gc.Equals, operationIDRunning)
	c.Check(actionRunning.Status(), gc.Equals, state.ActionRunning)
	c.Check(actionRunning.Parallel(), jc.IsTrue)
	c.Check(actionRunning.ExecutionGroup(), gc.Equals, "group")

	actionAborting, err = newModel.ActionByTag(actionAborting.ActionTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(actionAborting.Receiver(), gc.Equals, machine.Id())
	c.Check(actionAborting.Name(), gc.Equals, "action-aborting")
	c.Check(state.ActionOperationId(actionAborting), gc.Equals, operationIDAborting)
	c.Check(actionAborting.Status(), gc.Equals, state.ActionAborting)
	c.Check(actionAborting.Parallel(), jc.IsTrue)
	c.Check(actionAborting.ExecutionGroup(), gc.Equals, "group")

	actionAborted, err = newModel.ActionByTag(actionAborted.ActionTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(actionAborted.Receiver(), gc.Equals, machine.Id())
	c.Check(actionAborted.Name(), gc.Equals, "action-aborted")
	c.Check(state.ActionOperationId(actionAborted), gc.Equals, operationIDAborted)
	c.Check(actionAborted.Status(), gc.Equals, state.ActionAborted)
	c.Check(actionAborted.Parallel(), jc.IsTrue)
	c.Check(actionAborted.ExecutionGroup(), gc.Equals, "group")

	actionCompleted, err = newModel.ActionByTag(actionCompleted.ActionTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(actionCompleted.Receiver(), gc.Equals, machine.Id())
	c.Check(actionCompleted.Name(), gc.Equals, "action-completed")
	c.Check(state.ActionOperationId(actionCompleted), gc.Equals, operationIDCompleted)
	c.Check(actionCompleted.Status(), gc.Equals, state.ActionCompleted)
	c.Check(actionCompleted.Parallel(), jc.IsTrue)
	c.Check(actionCompleted.ExecutionGroup(), gc.Equals, "group")

	// Only pending/running/aborting actions will have action notification docs imported.
	actionIDs, err := newModel.AllActionIDsHasActionNotifications()
	c.Assert(err, jc.ErrorIsNil)
	sort.Strings(actionIDs)
	expectedIDs := []string{
		actionRunning.Id(),
		actionPending.Id(),
		actionAborting.Id(),
	}
	sort.Strings(expectedIDs)
	c.Check(actionIDs, gc.DeepEquals, expectedIDs)
}

func (s *MigrationImportSuite) TestOperation(c *gc.C) {
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	operationID, err := m.EnqueueOperation("a test", 2)
	c.Assert(err, jc.ErrorIsNil)
	err = m.FailOperationEnqueuing(operationID, "fail", 1)
	c.Assert(err, jc.ErrorIsNil)

	newModel, newState := s.importModel(c, s.State)
	defer func() {
		c.Assert(newState.Close(), jc.ErrorIsNil)
	}()

	operations, _ := newModel.AllOperations()
	c.Assert(operations, gc.HasLen, 1)
	op := operations[0]
	c.Check(op.Summary(), gc.Equals, "a test")
	c.Check(op.Fail(), gc.Equals, "fail")
	c.Check(op.Id(), gc.Equals, operationID)
	c.Check(op.Status(), gc.Equals, state.ActionPending)
	c.Check(op.SpawnedTaskCount(), gc.Equals, 1)
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
	c.Assert(err, jc.ErrorIsNil)
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
		sc := app["storage-directives"].(map[interface{}]interface{})
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
	c.Assert(pool.Attrs(), jc.DeepEquals, storage.Attrs{
		"value": 42,
	})
}

func (s *MigrationImportSuite) TestPayloads(c *gc.C) {
	originalUnit := s.Factory.MakeUnit(c, nil)
	unitID := originalUnit.UnitTag().Id()
	up, err := s.State.UnitPayloads(originalUnit)
	c.Assert(err, jc.ErrorIsNil)
	original := payloads.Payload{
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
	remoteApp, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
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
		Spaces: []*environs.ProviderSpaceInfo{{
			SpaceInfo: network.SpaceInfo{
				Name:       "unicorns",
				ProviderId: "space-provider-id",
				Subnets: []network.SubnetInfo{{
					CIDR:              "10.0.1.0/24",
					ProviderId:        "subnet-provider-id",
					AvailabilityZones: []string{"eu-west-1"},
				}},
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = remoteApp.SetStatus(status.StatusInfo{Status: status.Active})
	c.Assert(err, jc.ErrorIsNil)

	service := state.NewExternalControllers(s.State)
	_, err = service.Save(crossmodel.ControllerInfo{
		ControllerTag: s.Model.ControllerTag(),
		Addrs:         []string{"192.168.1.1:8080"},
		Alias:         "magic",
		CACert:        "magic-ca-cert",
	}, s.Model.UUID())
	c.Assert(err, jc.ErrorIsNil)

	out, err := s.State.Export(map[string]string{})
	c.Assert(err, jc.ErrorIsNil)

	uuid := utils.MustNewUUID().String()
	in := newModel(out, uuid, "new")

	_, newSt, err := s.Controller.Import(in)
	if err == nil {
		defer newSt.Close()
	}
	c.Assert(err, jc.ErrorIsNil)
	remoteApplications, err := newSt.AllRemoteApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remoteApplications, gc.HasLen, 1)

	remoteApplication := remoteApplications[0]
	c.Assert(remoteApplication.Name(), gc.Equals, "gravy-rainbow")
	c.Assert(remoteApplication.ConsumeVersion(), gc.Equals, 1)

	url, _ := remoteApplication.URL()
	c.Assert(url, gc.Equals, "me/model.rainbow")
	c.Assert(remoteApplication.SourceModel(), gc.Equals, s.Model.ModelTag())

	token, err := remoteApplication.Token()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(token, gc.Equals, "charisma")

	s.assertRemoteApplicationEndpoints(c, remoteApp, remoteApplication)
	s.assertRemoteApplicationSpaces(c, remoteApp, remoteApplication)
}

func (s *MigrationImportSuite) TestRemoteApplicationsConsumerProxy(c *gc.C) {
	remoteApp, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "gravy-rainbow",
		URL:             "me/model.rainbow",
		SourceModel:     s.Model.ModelTag(),
		Token:           "charisma",
		ConsumeVersion:  2,
		IsConsumerProxy: true,
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
		Spaces: []*environs.ProviderSpaceInfo{{
			SpaceInfo: network.SpaceInfo{
				Name:       "unicorns",
				ProviderId: "space-provider-id",
				Subnets: []network.SubnetInfo{{
					CIDR:              "10.0.1.0/24",
					ProviderId:        "subnet-provider-id",
					AvailabilityZones: []string{"eu-west-1"},
				}},
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	service := state.NewExternalControllers(s.State)
	_, err = service.Save(crossmodel.ControllerInfo{
		ControllerTag: s.Model.ControllerTag(),
		Addrs:         []string{"192.168.1.1:8080"},
		Alias:         "magic",
		CACert:        "magic-ca-cert",
	}, s.Model.UUID())
	c.Assert(err, jc.ErrorIsNil)

	out, err := s.State.Export(map[string]string{})
	c.Assert(err, jc.ErrorIsNil)

	uuid := utils.MustNewUUID().String()
	in := newModel(out, uuid, "new")

	_, newSt, err := s.Controller.Import(in)
	if err == nil {
		defer newSt.Close()
	}
	c.Assert(err, jc.ErrorIsNil)
	remoteApplications, err := newSt.AllRemoteApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remoteApplications, gc.HasLen, 1)

	remoteApplication := remoteApplications[0]
	c.Assert(remoteApplication.Name(), gc.Equals, "gravy-rainbow")
	c.Assert(remoteApplication.ConsumeVersion(), gc.Equals, 2)

	url, _ := remoteApplication.URL()
	c.Assert(url, gc.Equals, "me/model.rainbow")
	c.Assert(remoteApplication.SourceModel(), gc.Equals, s.Model.ModelTag())

	token, err := remoteApplication.Token()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(token, gc.Equals, "charisma")

	s.assertRemoteApplicationEndpoints(c, remoteApp, remoteApplication)
	s.assertRemoteApplicationSpaces(c, remoteApp, remoteApplication)
}

func (s *MigrationImportSuite) assertRemoteApplicationEndpoints(c *gc.C, expected, received *state.RemoteApplication) {
	receivedEndpoints, err := received.Endpoints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(receivedEndpoints, gc.HasLen, 3)

	expectedEndpoints, err := expected.Endpoints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(expectedEndpoints, gc.HasLen, 3)

	for k, expectedEndpoint := range expectedEndpoints {
		receivedEndpoint := receivedEndpoints[k]
		c.Assert(receivedEndpoint.Interface, gc.Equals, expectedEndpoint.Interface)
		c.Assert(receivedEndpoint.Name, gc.Equals, expectedEndpoint.Name)
	}
}

func (s *MigrationImportSuite) assertRemoteApplicationSpaces(c *gc.C, expected, received *state.RemoteApplication) {
	receivedSpaces := received.Spaces()
	c.Assert(receivedSpaces, gc.HasLen, 1)

	expectedSpaces := expected.Spaces()
	c.Assert(expectedSpaces, gc.HasLen, 1)
	for k, expectedSpace := range expectedSpaces {
		receivedSpace := receivedSpaces[k]
		c.Assert(receivedSpace.Name, gc.Equals, expectedSpace.Name)
		c.Assert(receivedSpace.ProviderId, gc.Equals, expectedSpace.ProviderId)

		c.Assert(receivedSpace.Subnets, gc.HasLen, 1)
		receivedSubnet := receivedSpace.Subnets[0]

		c.Assert(expectedSpace.Subnets, gc.HasLen, 1)
		expectedSubnet := expectedSpace.Subnets[0]

		c.Assert(receivedSubnet.CIDR, gc.Equals, expectedSubnet.CIDR)
		c.Assert(receivedSubnet.ProviderId, gc.Equals, expectedSubnet.ProviderId)
		c.Assert(receivedSubnet.AvailabilityZones, gc.DeepEquals, expectedSubnet.AvailabilityZones)
	}
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
			Number:  jujuversion.Current,
			Arch:    arch.HostArch(),
			Release: app.CharmOrigin().Platform.OS,
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

func (s *MigrationImportSuite) TestImportingModelWithDefaultSeriesBefore2935(c *gc.C) {
	defaultBase, ok := s.testImportingModelWithDefaultSeries(c, version.MustParse("2.7.8"))
	c.Assert(ok, jc.IsFalse, gc.Commentf("value: %q", defaultBase))
}

func (s *MigrationImportSuite) TestImportingModelWithDefaultSeriesAfter2935(c *gc.C) {
	defaultBase, ok := s.testImportingModelWithDefaultSeries(c, version.MustParse("2.9.35"))
	c.Assert(ok, jc.IsTrue)
	c.Assert(defaultBase, gc.Equals, "ubuntu@22.04/stable")
}

func (s *MigrationImportSuite) testImportingModelWithDefaultSeries(c *gc.C, toolsVer version.Number) (string, bool) {
	testModel, err := s.State.Export(map[string]string{})
	c.Assert(err, jc.ErrorIsNil)

	newConfig := testModel.Config()
	newConfig["uuid"] = "aabbccdd-1234-8765-abcd-0123456789ab"
	newConfig["name"] = "something-new"
	newConfig["default-series"] = "jammy"
	newConfig["agent-version"] = toolsVer.String()
	importModel := description.NewModel(description.ModelArgs{
		Type:           string(state.ModelTypeIAAS),
		Owner:          testModel.Owner(),
		Config:         newConfig,
		EnvironVersion: testModel.EnvironVersion(),
		Blocks:         testModel.Blocks(),
		Cloud:          testModel.Cloud(),
		CloudRegion:    testModel.CloudRegion(),
	})
	imported, newSt, err := s.Controller.Import(importModel)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = newSt.Close() }()

	importedCfg, err := imported.Config()
	c.Assert(err, jc.ErrorIsNil)
	return importedCfg.DefaultBase()
}

func (s *MigrationImportSuite) TestImportingRelationApplicationSettings(c *gc.C) {
	state.AddTestingApplication(c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	state.AddTestingApplication(c, s.State, "mysql", state.AddTestingCharm(c, s.State, "mysql"))
	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	wordpressSettings := map[string]interface{}{
		"venusian": "superbug",
	}
	err = rel.UpdateApplicationSettings("wordpress", &fakeToken{}, wordpressSettings)
	c.Assert(err, jc.ErrorIsNil)
	mysqlSettings := map[string]interface{}{
		"planet b": "perihelion",
	}
	err = rel.UpdateApplicationSettings("mysql", &fakeToken{}, mysqlSettings)
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State)

	newWordpress, err := newSt.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(state.RelationCount(newWordpress), gc.Equals, 1)
	rels, err := newWordpress.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rels, gc.HasLen, 1)

	newRel := rels[0]

	newWpSettings, err := newRel.ApplicationSettings("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newWpSettings, gc.DeepEquals, wordpressSettings)

	newMysqlSettings, err := newRel.ApplicationSettings("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newMysqlSettings, gc.DeepEquals, mysqlSettings)
}

func (s *MigrationImportSuite) TestApplicationAddLatestCharmChannelTrack(c *gc.C) {
	st := s.State
	// Add a application with charm settings, app config, and leadership settings.
	f := factory.NewFactory(st, s.StatePool)

	// Add a application with charm settings, app config, and leadership settings.
	testCharm := f.MakeCharmV2(c, &factory.CharmParams{
		Name: "snappass-test", // it has resources
	})
	c.Assert(testCharm.Meta().Resources, gc.HasLen, 3)
	origin := &state.CharmOrigin{
		Source:   "charm-hub",
		Type:     "charm",
		Revision: &charm.MustParseURL(testCharm.URL()).Revision,
		Channel: &state.Channel{
			Risk: "edge",
		},
		ID:   "charm-hub-id",
		Hash: "charmhub-hash",
		Platform: &state.Platform{
			Architecture: charm.MustParseURL(testCharm.URL()).Architecture,
			OS:           "ubuntu",
			Channel:      "12.10/stable",
		},
	}
	application := f.MakeApplication(c, &factory.ApplicationParams{
		Charm:       testCharm,
		CharmOrigin: origin,
	})
	allApplications, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allApplications, gc.HasLen, 1)

	_, newSt := s.importModel(c, s.State)
	importedApp, err := newSt.Application(application.Name())
	c.Assert(err, jc.ErrorIsNil)
	exportedOrigin := application.CharmOrigin()
	exportedOrigin.Channel.Track = "latest"
	c.Assert(importedApp.CharmOrigin(), gc.DeepEquals, exportedOrigin, gc.Commentf("obtained %s", pretty.Sprint(importedApp.CharmOrigin())))
}

func (s *MigrationImportSuite) TestApplicationFillInCharmOriginID(c *gc.C) {
	st := s.State
	// Add a application with charm settings, app config, and leadership settings.
	f := factory.NewFactory(st, s.StatePool)

	// Add a application with charm settings, app config, and leadership settings.
	testCharm := f.MakeCharmV2(c, &factory.CharmParams{
		Name: "snappass-test", // it has resources
	})
	c.Assert(testCharm.Meta().Resources, gc.HasLen, 3)
	origin := &state.CharmOrigin{
		Source:   "charm-hub",
		Type:     "charm",
		Revision: &charm.MustParseURL(testCharm.URL()).Revision,
		Channel: &state.Channel{
			Risk: "edge",
		},
		ID:   "charm-hub-id",
		Hash: "charmhub-hash",
		Platform: &state.Platform{
			Architecture: charm.MustParseURL(testCharm.URL()).Architecture,
			OS:           "ubuntu",
			Channel:      "12.10/stable",
		},
	}
	appOne := f.MakeApplication(c, &factory.ApplicationParams{
		Name:        "one",
		Charm:       testCharm,
		CharmOrigin: origin,
	})
	originNoID := origin
	originNoID.ID = ""
	originNoID.Hash = ""
	appTwo := f.MakeApplication(c, &factory.ApplicationParams{
		Name:        "two",
		Charm:       testCharm,
		CharmOrigin: origin,
	})
	appThree := f.MakeApplication(c, &factory.ApplicationParams{
		Name:        "three",
		Charm:       testCharm,
		CharmOrigin: origin,
	})
	allApplications, err := s.State.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allApplications, gc.HasLen, 3)

	_, newSt := s.importModel(c, s.State)
	importedAppOne, err := newSt.Application(appOne.Name())
	c.Assert(err, jc.ErrorIsNil)
	importedAppTwo, err := newSt.Application(appTwo.Name())
	c.Assert(err, jc.ErrorIsNil)
	importedAppThree, err := newSt.Application(appThree.Name())
	c.Assert(err, jc.ErrorIsNil)
	obtainedChOrigOne := importedAppOne.CharmOrigin()
	obtainedChOrigTwo := importedAppTwo.CharmOrigin()
	obtainedChOrigThree := importedAppThree.CharmOrigin()
	c.Assert(obtainedChOrigTwo.ID, gc.Equals, obtainedChOrigOne.ID)
	c.Assert(obtainedChOrigThree.ID, gc.Equals, obtainedChOrigOne.ID)
}

func (s *MigrationImportSuite) TestSecrets(c *gc.C) {
	now := time.Now().UTC().Round(time.Second)
	next := now.Add(time.Minute).Round(time.Second).UTC()

	backendStore := state.NewSecretBackends(s.State)
	backendID, err := backendStore.CreateSecretBackend(state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Second),
		NextRotateTime:      ptr(next),
	})
	c.Assert(err, jc.ErrorIsNil)

	store := state.NewSecrets(s.State)
	owner := s.Factory.MakeApplication(c, nil)
	uri := secrets.NewURI()
	expire := now.Add(2 * time.Hour).Round(time.Second).UTC()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    &fakeToken{},
			RotatePolicy:   ptr(secrets.RotateDaily),
			NextRotateTime: ptr(next),
			Description:    ptr("my secret"),
			Label:          ptr("foobar"),
			ExpireTime:     ptr(expire),
			Params:         nil,
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	}
	md, err := store.CreateSecret(uri, p)
	c.Assert(err, jc.ErrorIsNil)
	updateTime := time.Now().UTC().Round(time.Second)
	md, err = store.UpdateSecret(md.URI, state.UpdateSecretParams{
		LeaderToken: &fakeToken{},
		AutoPrune:   ptr(true),
		ValueRef: &secrets.ValueRef{
			BackendID:  backendID,
			RevisionID: "rev-id",
		},
		Checksum: "deadbeef",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       owner.Tag(),
		Subject:     owner.Tag(),
		Role:        secrets.RoleManage,
	})
	c.Assert(err, jc.ErrorIsNil)

	consumer := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	err = s.State.SaveSecretConsumer(uri, consumer.Tag(), &secrets.SecretConsumerMetadata{
		Label:           "consumer label",
		CurrentRevision: 666,
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "remote-app", SourceModel: s.Model.ModelTag(), IsConsumerProxy: true})
	c.Assert(err, jc.ErrorIsNil)
	remoteConsumer := names.NewApplicationTag("remote-app")
	err = s.State.SaveSecretRemoteConsumer(uri, remoteConsumer, &secrets.SecretConsumerMetadata{
		CurrentRevision: 667,
	})
	c.Assert(err, jc.ErrorIsNil)

	backendRefCount, err := s.State.ReadBackendRefCount(backendID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(backendRefCount, gc.Equals, 1)

	err = s.Model.UpdateModelConfig(map[string]interface{}{config.SecretBackendKey: "myvault"}, nil)
	c.Assert(err, jc.ErrorIsNil)
	mCfg, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mCfg.SecretBackend(), jc.DeepEquals, "myvault")

	newModel, newSt := s.importModel(c, s.State)

	mCfg, err = newModel.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mCfg.SecretBackend(), jc.DeepEquals, "myvault")

	backendRefCount, err = s.State.ReadBackendRefCount(backendID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(backendRefCount, gc.Equals, 2)

	store = state.NewSecrets(newSt)
	all, err := store.ListSecrets(state.SecretsFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 1)
	c.Assert(all[0], jc.DeepEquals, md)

	revs, err := store.ListSecretRevisions(md.URI)
	c.Assert(err, jc.ErrorIsNil)
	mc := jc.NewMultiChecker()
	mc.AddExpr(`_.CreateTime`, jc.Almost, jc.ExpectedValue)
	mc.AddExpr(`_.UpdateTime`, jc.Almost, jc.ExpectedValue)
	c.Assert(revs, mc, []*secrets.SecretRevisionMetadata{{
		Revision:   1,
		ValueRef:   nil,
		CreateTime: now,
		UpdateTime: updateTime,
		ExpireTime: &expire,
	}, {
		Revision: 2,
		ValueRef: &secrets.ValueRef{
			BackendID:  backendID,
			RevisionID: "rev-id",
		},
		BackendName: ptr("myvault"),
		CreateTime:  now,
		UpdateTime:  now,
	}})

	access, err := newSt.SecretAccess(uri, owner.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, secrets.RoleManage)

	info, err := newSt.GetSecretConsumer(uri, consumer.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &secrets.SecretConsumerMetadata{
		Label:           "consumer label",
		CurrentRevision: 666,
		LatestRevision:  2,
	})

	info, err = newSt.GetSecretRemoteConsumer(uri, remoteConsumer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &secrets.SecretConsumerMetadata{
		CurrentRevision: 667,
		LatestRevision:  2,
	})

	backendRefCount, err = newSt.ReadBackendRefCount(backendID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(backendRefCount, gc.Equals, 2)
}

func (s *MigrationImportSuite) TestSecretsEnsureConsumerRevisionInfo(c *gc.C) {
	store := state.NewSecrets(s.State)
	owner := s.Factory.MakeApplication(c, nil)
	uri := secrets.NewURI()
	p := state.CreateSecretParams{
		Version: 1,
		Owner:   owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:  &fakeToken{},
			RotatePolicy: ptr(secrets.RotateNever),
			Data:         map[string]string{"foo": "bar"},
		},
	}
	md, err := store.CreateSecret(uri, p)
	c.Assert(err, jc.ErrorIsNil)

	consumer := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	err = s.State.SaveSecretConsumer(uri, consumer.Tag(), &secrets.SecretConsumerMetadata{
		Label:           "consumer label",
		CurrentRevision: 0,
		LatestRevision:  0,
	})
	c.Assert(err, jc.ErrorIsNil)

	_, newSt := s.importModel(c, s.State)

	store = state.NewSecrets(newSt)
	all, err := store.ListSecrets(state.SecretsFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 1)
	c.Assert(all[0], jc.DeepEquals, md)

	info, err := newSt.GetSecretConsumer(uri, consumer.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &secrets.SecretConsumerMetadata{
		Label:           "consumer label",
		CurrentRevision: 1,
		LatestRevision:  1,
	})
}

func (s *MigrationImportSuite) TestSecretsMissingBackend(c *gc.C) {
	store := state.NewSecrets(s.State)
	owner := s.Factory.MakeApplication(c, nil)
	uri := secrets.NewURI()

	backendStore := state.NewSecretBackends(s.State)
	_, err := backendStore.CreateSecretBackend(state.CreateSecretBackendParams{
		ID:          "backend-id",
		Name:        "foo",
		BackendType: "vault",
	})
	c.Assert(err, jc.ErrorIsNil)

	p := state.CreateSecretParams{
		Version: 1,
		Owner:   owner.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			ValueRef: &secrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-id",
			},
		},
	}
	_, err = store.CreateSecret(uri, p)
	c.Assert(err, jc.ErrorIsNil)

	out, err := s.State.Export(map[string]string{})
	c.Assert(err, jc.ErrorIsNil)

	err = backendStore.DeleteSecretBackend("foo", true)
	c.Assert(err, jc.ErrorIsNil)

	uuid := utils.MustNewUUID().String()
	in := newModel(out, uuid, "new")
	_, _, err = s.Controller.Import(in)
	c.Assert(err, gc.ErrorMatches, "secrets: target controller does not have all required secret backends set up")
}

func (s *MigrationImportSuite) TestDefaultSecretBackend(c *gc.C) {
	testModel, err := s.State.Export(map[string]string{})
	c.Assert(err, jc.ErrorIsNil)

	newConfig := testModel.Config()
	newConfig["uuid"] = "aabbccdd-1234-8765-abcd-0123456789ab"
	newConfig["name"] = "something-new"
	delete(newConfig, "secret-backend")
	importModel := description.NewModel(description.ModelArgs{
		Type:           string(state.ModelTypeIAAS),
		Owner:          testModel.Owner(),
		Config:         newConfig,
		EnvironVersion: testModel.EnvironVersion(),
		Blocks:         testModel.Blocks(),
		Cloud:          testModel.Cloud(),
		CloudRegion:    testModel.CloudRegion(),
	})
	imported, newSt, err := s.Controller.Import(importModel)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = newSt.Close() }()

	importedCfg, err := imported.Config()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(importedCfg.SecretBackend(), gc.Equals, "auto")
}

func (s *MigrationImportSuite) TestApplicationWithProvisioningState(c *gc.C) {
	caasSt := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *gc.C) { caasSt.Close() })

	cons := constraints.MustParse("arch=amd64 mem=8G")
	platform := &state.Platform{
		Architecture: arch.DefaultArchitecture,
		OS:           "ubuntu",
		Channel:      "20.04",
	}
	testCharm, application, _ := s.setupSourceApplications(c, caasSt, cons, platform, false)

	err := application.SetScale(1, 0, true)
	c.Assert(err, jc.ErrorIsNil)
	err = application.SetProvisioningState(state.ApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 1,
	})
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
		Series:   "kubernetes",
		URL:      testCharm.URL(),
		Revision: strconv.Itoa(testCharm.Revision()),
	})
	importedApplication, err := newSt.Application(application.Name())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(importedApplication.ProvisioningState(), jc.DeepEquals, &state.ApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 1,
	})
}

func (s *MigrationImportSuite) TestVirtualHostKeys(c *gc.C) {
	machineTag := names.NewMachineTag("0")
	testHostKey := []byte("foo")

	// Add a virtual host key
	state.AddVirtualHostKey(c, s.State, machineTag, testHostKey)

	allVirtualHostKeys, err := s.State.AllVirtualHostKeys()
	c.Assert(err, gc.IsNil)
	c.Assert(allVirtualHostKeys, gc.HasLen, 1)

	_, newSt := s.importModel(c, s.State)

	newVirtualHostKey, err := newSt.MachineVirtualHostKey(machineTag.Id())
	c.Assert(err, gc.IsNil)
	c.Assert(newVirtualHostKey.HostKey(), gc.DeepEquals, testHostKey)
}

func (s *MigrationImportSuite) TestGenerateMissingVirtualHostKeys(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	existingVirtualHostKey, err := s.State.MachineVirtualHostKey(machine.Tag().Id())
	c.Assert(err, gc.IsNil)
	c.Assert(string(existingVirtualHostKey.HostKey()), gc.Equals, "fake-host-key")

	state.RemoveVirtualHostKey(c, s.State, existingVirtualHostKey)

	_, newSt := s.importModel(c, s.State)

	newVirtualHostKey, err := newSt.MachineVirtualHostKey(machine.Tag().Id())
	c.Assert(err, gc.IsNil)
	c.Assert(string(newVirtualHostKey.HostKey()), gc.Matches, `(?s)-----BEGIN OPENSSH PRIVATE KEY-----.*`)
}

// newModel replaces the uuid and name of the config attributes so we
// can use all the other data to validate imports. An owner and name of the
// model are unique together in a controller.
// Also, optionally overwrite the return value of certain methods
func newModel(m description.Model, uuid, name string) *mockModel {
	return &mockModel{Model: m, uuid: uuid, name: name}
}

type mockModel struct {
	description.Model
	uuid    string
	name    string
	fwRules []description.FirewallRule
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

func (m *mockModel) FirewallRules() []description.FirewallRule {
	if m.fwRules == nil {
		return m.Model.FirewallRules()
	}
	return m.fwRules
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
