// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/machinemanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&MachineManagerSuite{})

type MachineManagerSuite struct {
	coretesting.BaseSuite
	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	st         *mockState
	api        *machinemanager.MachineManagerAPI
}

func (s *MachineManagerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	tag := names.NewUserTag("admin")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	s.st = &mockState{machines: make(map[string]*mockMachine)}
	machinemanager.PatchState(s, s.st)

	var err error
	s.api, err = machinemanager.NewMachineManagerAPI(nil, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineManagerSuite) TestAddMachines(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 2)
	for i := range apiParams {
		apiParams[i] = params.AddMachineParams{
			Series: "trusty",
			Jobs:   []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
		}
	}
	apiParams[0].Disks = []storage.Constraints{{Size: 1, Count: 2}, {Size: 2, Count: 1}}
	apiParams[1].Disks = []storage.Constraints{{Size: 1, Count: 2, Pool: "three"}}
	machines, err := s.api.AddMachines(params.AddMachines{MachineParams: apiParams})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines.Machines, gc.HasLen, 2)
	c.Assert(s.st.calls, gc.Equals, 2)
	c.Assert(s.st.machineTemplates, jc.DeepEquals, []state.MachineTemplate{
		{
			Series: "trusty",
			Jobs:   []state.MachineJob{state.JobHostUnits},
			Volumes: []state.MachineVolumeParams{
				{
					Volume:     state.VolumeParams{Pool: "", Size: 1},
					Attachment: state.VolumeAttachmentParams{ReadOnly: false},
				},
				{
					Volume:     state.VolumeParams{Pool: "", Size: 1},
					Attachment: state.VolumeAttachmentParams{ReadOnly: false},
				},
				{
					Volume:     state.VolumeParams{Pool: "", Size: 2},
					Attachment: state.VolumeAttachmentParams{ReadOnly: false},
				},
			},
		},
		{
			Series: "trusty",
			Jobs:   []state.MachineJob{state.JobHostUnits},
			Volumes: []state.MachineVolumeParams{
				{
					Volume:     state.VolumeParams{Pool: "three", Size: 1},
					Attachment: state.VolumeAttachmentParams{ReadOnly: false},
				},
				{
					Volume:     state.VolumeParams{Pool: "three", Size: 1},
					Attachment: state.VolumeAttachmentParams{ReadOnly: false},
				},
			},
		},
	})
}

func (s *MachineManagerSuite) TestNewMachineManagerAPINonClient(c *gc.C) {
	tag := names.NewUnitTag("mysql/0")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	_, err := machinemanager.NewMachineManagerAPI(nil, nil, s.authorizer)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *MachineManagerSuite) TestAddMachinesStateError(c *gc.C) {
	s.st.err = errors.New("boom")
	results, err := s.api.AddMachines(params.AddMachines{
		MachineParams: []params.AddMachineParams{{
			Series: "trusty",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.AddMachinesResults{
		Machines: []params.AddMachinesResult{{
			Error: &params.Error{Message: "boom", Code: ""},
		}},
	})
	c.Assert(s.st.calls, gc.Equals, 1)
}

func (s *MachineManagerSuite) TestDestroyMachine(c *gc.C) {
	s.st.machines["0"] = &mockMachine{}
	results, err := s.api.DestroyMachine(params.Entities{
		Entities: []params.Entity{{Tag: "machine-0"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Info: &params.DestroyMachineInfo{
				DestroyedUnits: []params.Entity{
					{"unit-foo-0"},
					{"unit-foo-1"},
					{"unit-foo-2"},
				},
				DestroyedStorage: []params.Entity{
					{"storage-disks-0"},
					{"storage-disks-1"},
				},
			},
		}},
	})
}

func (s *MachineManagerSuite) TestDestroyMachineWithParams(c *gc.C) {
	apiV4 := machinemanager.MachineManagerAPIV4{s.api}
	s.st.machines["0"] = &mockMachine{}
	results, err := apiV4.DestroyMachineWithParams(params.DestroyMachinesParams{
		Keep:        true,
		Force:       true,
		MachineTags: []string{"machine-0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.(*mockMachine).keep, jc.IsTrue)
	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Info: &params.DestroyMachineInfo{
				DestroyedUnits: []params.Entity{
					{"unit-foo-0"},
					{"unit-foo-1"},
					{"unit-foo-2"},
				},
				DestroyedStorage: []params.Entity{
					{"storage-disks-0"},
					{"storage-disks-1"},
				},
			},
		}},
	})
}

type mockState struct {
	calls            int
	machineTemplates []state.MachineTemplate
	machines         map[string]*mockMachine
	err              error
}

func (st *mockState) AddOneMachine(template state.MachineTemplate) (*state.Machine, error) {
	st.calls++
	st.machineTemplates = append(st.machineTemplates, template)
	m := state.Machine{}
	return &m, st.err
}

func (st *mockState) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	return &mockBlock{}, false, nil
}

func (st *mockState) ModelTag() names.ModelTag {
	return names.NewModelTag("deadbeef-2f18-4fd2-967d-db9663db7bea")
}

func (st *mockState) ModelConfig() (*config.Config, error) {
	panic("not implemented")
}

func (st *mockState) Model() (*state.Model, error) {
	panic("not implemented")
}

func (st *mockState) AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (*state.Machine, error) {
	panic("not implemented")
}

func (st *mockState) AddMachineInsideMachine(template state.MachineTemplate, parentId string, containerType instance.ContainerType) (*state.Machine, error) {
	panic("not implemented")
}

func (st *mockState) Clouds() (map[names.CloudTag]cloud.Cloud, error) {
	return nil, nil
}

func (st *mockState) CloudCredentials(user names.UserTag, cloudName string) (map[string]cloud.Credential, error) {
	return nil, nil
}

func (st *mockState) CloudCredential(tag names.CloudCredentialTag) (cloud.Credential, error) {
	return cloud.Credential{}, nil
}

func (st *mockState) Cloud(string) (cloud.Cloud, error) {
	return cloud.Cloud{}, nil
}

func (st *mockState) GetModel(t names.ModelTag) (machinemanager.Model, error) {
	return &mockModel{}, nil
}

func (st *mockState) Machine(id string) (machinemanager.Machine, error) {
	if m, ok := st.machines[id]; !ok {
		return nil, errors.NotFoundf("machine %v", id)
	} else {
		return m, nil
	}
}

func (st *mockState) StorageInstance(tag names.StorageTag) (state.StorageInstance, error) {
	return &mockStorage{tag: tag}, nil
}

func (st *mockState) UnitStorageAttachments(tag names.UnitTag) ([]state.StorageAttachment, error) {
	if tag.Id() == "foo/0" {
		return []state.StorageAttachment{
			&mockStorageAttachment{unit: tag, storage: names.NewStorageTag("disks/0")},
			&mockStorageAttachment{unit: tag, storage: names.NewStorageTag("disks/1")},
		}, nil
	}
	return nil, nil
}

type mockBlock struct {
	state.Block
}

func (st *mockBlock) Id() string {
	return "id"
}

func (st *mockBlock) Tag() (names.Tag, error) {
	return names.ParseTag("machine-1")
}

func (st *mockBlock) Type() state.BlockType {
	return state.ChangeBlock
}

func (st *mockBlock) Message() string {
	return "not allowed"
}

func (st *mockBlock) ModelUUID() string {
	return "uuid"
}

type mockMachine struct {
	keep bool
}

func (m *mockMachine) Destroy() error {
	return nil
}

func (m *mockMachine) ForceDestroy() error {
	return nil
}

func (m *mockMachine) SetKeepInstance(keep bool) error {
	m.keep = keep
	return nil
}

func (m *mockMachine) Units() ([]machinemanager.Unit, error) {
	return []machinemanager.Unit{
		&mockUnit{names.NewUnitTag("foo/0")},
		&mockUnit{names.NewUnitTag("foo/1")},
		&mockUnit{names.NewUnitTag("foo/2")},
	}, nil
}

type mockUnit struct {
	tag names.UnitTag
}

func (u *mockUnit) UnitTag() names.UnitTag {
	return u.tag
}

type mockStorage struct {
	state.StorageInstance
	tag names.StorageTag
}

func (a *mockStorage) StorageTag() names.StorageTag {
	return a.tag
}

type mockStorageAttachment struct {
	state.StorageAttachment
	unit    names.UnitTag
	storage names.StorageTag
}

func (a *mockStorageAttachment) Unit() names.UnitTag {
	return a.unit
}

func (a *mockStorageAttachment) StorageInstance() names.StorageTag {
	return a.storage
}
