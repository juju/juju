// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"
	"fmt"
	"sort"
	"time" // only uses time.Time values

	"github.com/juju/description/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"github.com/kr/pretty"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/charm"
	internalpassword "github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/state/mocks"
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
	out, err := s.State.Export(map[string]string{}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	ctrlCfg := coretesting.FakeControllerConfig()

	_, _, err = s.Controller.Import(out, ctrlCfg, state.NoopConfigSchemaSource)
	c.Assert(err, jc.ErrorIs, errors.AlreadyExists)
}

func (s *MigrationImportSuite) importModel(
	c *gc.C, st *state.State, transform ...func(map[string]interface{}),
) (*state.Model, *state.State) {
	desc, err := st.Export(map[string]string{}, state.NewObjectStore(c, s.State.ModelUUID()))
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

	uuid := uuid.MustNewUUID().String()
	in := newModel(desc, uuid, "new")

	ctrlCfg := coretesting.FakeControllerConfig()

	newModel, newSt, err := s.Controller.Import(in, ctrlCfg, state.NoopConfigSchemaSource)
	c.Assert(err, jc.ErrorIsNil)

	s.AddCleanup(func(c *gc.C) {
		c.Check(newSt.Close(), jc.ErrorIsNil)
	})
	return newModel, newSt
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

	out, err := s.State.Export(map[string]string{}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	uuid := uuid.MustNewUUID().String()
	in := newModel(out, uuid, "new")

	ctrlCfg := coretesting.FakeControllerConfig()

	newModel, newSt, err := s.Controller.Import(in, ctrlCfg, state.NoopConfigSchemaSource)
	c.Assert(err, jc.ErrorIsNil)
	defer newSt.Close()

	c.Assert(newModel.PasswordHash(), gc.Equals, internalpassword.AgentPasswordHash("supersecret1111111111111"))
	c.Assert(newModel.Type(), gc.Equals, original.Type())
	c.Assert(newModel.Owner(), gc.Equals, original.Owner())
	c.Assert(newModel.LatestToolsVersion(), gc.Equals, latestTools)
	c.Assert(newModel.MigrationMode(), gc.Equals, state.MigrationModeImporting)
	c.Assert(newModel.EnvironVersion(), gc.Equals, environVersion)

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

	_, newSt := s.importModel(c, s.State)

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

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/description_mock.go github.com/juju/description/v8 Application,Machine,PortRanges,UnitPortRanges

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

func (s *MigrationImportSuite) TestCharmhubApplicationCharmOriginNormalised(c *gc.C) {
	platform := &state.Platform{Architecture: arch.DefaultArchitecture, OS: "ubuntu", Channel: "12.10/stable"}
	f := factory.NewFactory(s.State, s.StatePool, testing.FakeControllerConfig())

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
	f := factory.NewFactory(s.State, s.StatePool, testing.FakeControllerConfig())

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

	out, err := s.State.Export(map[string]string{}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(len(out.Applications()), gc.Equals, 1)

	uuid := uuid.MustNewUUID().String()
	in := newModel(out, uuid, "new")

	ctrlCfg := coretesting.FakeControllerConfig()

	_, newSt, err := s.Controller.Import(in, ctrlCfg, state.NoopConfigSchemaSource)
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

	out, err := s.State.Export(map[string]string{}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	apps := out.Applications()
	c.Assert(len(apps), gc.Equals, 2)

	// This test is only valid if the subordinate logging application
	// comes first in the model output.
	if apps[0].Name() != "logging" {
		out = &swapModel{out, c}
	}

	uuid := uuid.MustNewUUID().String()
	in := newModel(out, uuid, "new")

	ctrlCfg := coretesting.FakeControllerConfig()

	_, newSt, err := s.Controller.Import(in, ctrlCfg, state.NoopConfigSchemaSource)
	c.Assert(err, jc.ErrorIsNil)
	// add the cleanup here to close the model.
	s.AddCleanup(func(c *gc.C) {
		c.Check(newSt.Close(), jc.ErrorIsNil)
	})
}

func (s *MigrationImportSuite) TestUnits(c *gc.C) {
	cons := constraints.MustParse("arch=amd64 mem=8G")
	f := factory.NewFactory(s.State, s.StatePool, testing.FakeControllerConfig())
	exported, pwd := f.MakeUnitReturningPassword(c, &factory.UnitParams{
		Constraints: cons,
	})
	s.assertUnitsMigrated(c, s.State, cons, exported, pwd)
}

func (s *MigrationImportSuite) TestCAASUnits(c *gc.C) {
	caasSt := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *gc.C) { caasSt.Close() })

	cons := constraints.MustParse("arch=amd64 mem=8G")
	f := factory.NewFactory(caasSt, s.StatePool, testing.FakeControllerConfig())
	app := f.MakeApplication(c, &factory.ApplicationParams{Constraints: cons})
	exported, pwd := f.MakeUnitReturningPassword(c, &factory.UnitParams{
		Application: app,
	})
	s.assertUnitsMigrated(c, caasSt, cons, exported, pwd)
}

func (s *MigrationImportSuite) TestUnitsWithVirtConstraint(c *gc.C) {
	cons := constraints.MustParse("arch=amd64 mem=8G virt-type=lxd")
	f := factory.NewFactory(s.State, s.StatePool, testing.FakeControllerConfig())
	exported, pwd := f.MakeUnitReturningPassword(c, &factory.UnitParams{
		Constraints: cons,
	})
	s.assertUnitsMigrated(c, s.State, cons, exported, pwd)
}

func (s *MigrationImportSuite) TestUnitWithoutAnyPersistedState(c *gc.C) {
	f := factory.NewFactory(s.State, s.StatePool, testing.FakeControllerConfig())

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
}

func (s *MigrationImportSuite) assertUnitsMigrated(c *gc.C, st *state.State, cons constraints.Value, exported *state.Unit, pwd string) {
	err := exported.SetWorkloadVersion("amethyst")
	c.Assert(err, jc.ErrorIsNil)
	testModel, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	us := state.NewUnitState()
	us.SetCharmState(map[string]string{"payload": "0xb4c0ffee"})
	us.SetRelationState(map[int]string{42: "magic"})
	us.SetUniterState("uniter state")
	us.SetStorageState("storage state")
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
	wordpress := state.AddTestingApplication(c, s.State, s.objectStore, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	state.AddTestingApplication(c, s.State, s.objectStore, "mysql", state.AddTestingCharm(c, s.State, "mysql"))
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

	wordpress := state.AddTestingApplication(c, s.State, s.objectStore, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
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
	wordpress := state.AddTestingApplication(c, s.State, s.objectStore, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	state.AddTestingApplication(c, s.State, s.objectStore, "mysql", state.AddTestingCharm(c, s.State, "mysql"))
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

func (s *MigrationImportSuite) TestNilEndpointBindings(c *gc.C) {
	app := state.AddTestingApplicationWithEmptyBindings(
		c, s.State, s.objectStore, "dummy", state.AddTestingCharm(c, s.State, "dummy"))

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
	c.Assert(unitPortRanges.UniquePortRanges(), gc.DeepEquals, []network.PortRange{{
		FromPort: 1234,
		ToPort:   2345,
		Protocol: "tcp",
	}})
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

	base, err := s.State.Export(map[string]string{}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	uuid := uuid.MustNewUUID().String()
	model := newModel(base, uuid, "new")
	model.fwRules = []description.FirewallRule{sshRule, saasRule}

	_, newSt := s.importModelDescription(c, model)

	m, err := newSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := m.ModelConfig(context.Background())
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
	c.Assert(err, jc.ErrorIs, errors.NotFound)

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
	err := u.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	err = sb.DetachStorage(storageTag, u.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
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
	})
	c.Assert(err, jc.ErrorIsNil)
	err = remoteApp.SetStatus(status.StatusInfo{Status: status.Active})
	c.Assert(err, jc.ErrorIsNil)

	out, err := s.State.Export(map[string]string{}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	uuid := uuid.MustNewUUID().String()
	in := newModel(out, uuid, "new")

	ctrlCfg := coretesting.FakeControllerConfig()

	_, newSt, err := s.Controller.Import(in, ctrlCfg, state.NoopConfigSchemaSource)
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
	})
	c.Assert(err, jc.ErrorIsNil)

	out, err := s.State.Export(map[string]string{}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	uuid := uuid.MustNewUUID().String()
	in := newModel(out, uuid, "new")

	ctrlCfg := coretesting.FakeControllerConfig()

	_, newSt, err := s.Controller.Import(in, ctrlCfg, state.NoopConfigSchemaSource)
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
	wordpress := state.AddTestingApplication(c, s.State, s.objectStore, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	mysql := state.AddTestingApplication(c, s.State, s.objectStore, "mysql", state.AddTestingCharm(c, s.State, "mysql"))
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

func (s *MigrationImportSuite) TestImportingModelWithBlankType(c *gc.C) {
	testModel, err := s.State.Export(map[string]string{}, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	ctrlCfg := coretesting.FakeControllerConfig()

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
	imported, newSt, err := s.Controller.Import(noTypeModel, ctrlCfg, state.NoopConfigSchemaSource)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = newSt.Close() }()

	c.Assert(imported.Type(), gc.Equals, state.ModelTypeIAAS)
}

func (s *MigrationImportSuite) TestImportingRelationApplicationSettings(c *gc.C) {
	state.AddTestingApplication(c, s.State, s.objectStore, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	state.AddTestingApplication(c, s.State, s.objectStore, "mysql", state.AddTestingCharm(c, s.State, "mysql"))
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
	f := factory.NewFactory(st, s.StatePool, testing.FakeControllerConfig())

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
	f := factory.NewFactory(st, s.StatePool, testing.FakeControllerConfig())

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
