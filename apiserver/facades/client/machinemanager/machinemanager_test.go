// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"github.com/juju/errors"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/client/machinemanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&MachineManagerSuite{})

type MachineManagerSuite struct {
	coretesting.BaseSuite
	authorizer *apiservertesting.FakeAuthorizer
	st         *mockState
	pool       *mockPool
	api        *machinemanager.MachineManagerAPI
}

func (s *MachineManagerSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authorizer.Tag = user
	mm, err := machinemanager.NewMachineManagerAPI(s.st, s.pool, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.api = mm
}

func (s *MachineManagerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.st = &mockState{machines: make(map[string]*mockMachine)}
	s.pool = &mockPool{}
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin")}
	var err error
	s.api, err = machinemanager.NewMachineManagerAPI(s.st, s.pool, s.authorizer)
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
				DetachedStorage: []params.Entity{
					{"storage-disks-0"},
				},
				DestroyedStorage: []params.Entity{
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
				DetachedStorage: []params.Entity{
					{"storage-disks-0"},
				},
				DestroyedStorage: []params.Entity{
					{"storage-disks-1"},
				},
			},
		}},
	})
}

func (s *MachineManagerSuite) setupUpdateMachineSeries(c *gc.C) {
	s.st.machines = map[string]*mockMachine{
		"0": &mockMachine{series: "trusty"},
		"1": &mockMachine{series: "trusty"},
	}
}

func (s *MachineManagerSuite) TestUpdateMachineSeries(c *gc.C) {
	s.setupUpdateMachineSeries(c)
	apiV4 := machinemanager.MachineManagerAPIV4{s.api}
	results, err := apiV4.UpdateMachineSeries(
		params.UpdateSeriesArgs{
			Args: []params.UpdateSeriesArg{
				{
					Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
					Series: "xenial",
				}, {
					Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
					Series: "xenial",
					Force:  true,
				}, {
					Entity: params.Entity{Tag: names.NewMachineTag("1").String()},
					Series: "trusty",
				}, {
					Entity: params.Entity{Tag: names.NewMachineTag("76").String()},
					Series: "trusty",
				}, {
					Entity: params.Entity{Tag: names.NewUnitTag("mysql/0").String()},
					Series: "trusty",
				},
			}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{}, {}, {},
			{Error: &params.Error{Message: "machine 76 not found", Code: "not found"}},
			{Error: &params.Error{Message: "\"unit-mysql-0\" is not a valid machine tag", Code: ""}},
		}})

	mach := s.st.machines["0"]
	mach.CheckCall(c, 0, "Series")
	mach.CheckCall(c, 1, "UpdateMachineSeries", "xenial", false)
	mach.CheckCall(c, 3, "UpdateMachineSeries", "xenial", true)
	mach = s.st.machines["1"]
	mach.CheckCall(c, 0, "Series")
	c.Assert(len(mach.Calls()), gc.Equals, 1)
}

func (s *MachineManagerSuite) TestUpdateMachineSeriesNoSeries(c *gc.C) {
	apiV4 := machinemanager.MachineManagerAPIV4{s.api}
	results, err := apiV4.UpdateMachineSeries(
		params.UpdateSeriesArgs{
			Args: []params.UpdateSeriesArg{{
				Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
			}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.ErrorResult{
		Error: &params.Error{
			Code:    params.CodeBadRequest,
			Message: `series missing from args`,
		},
	})
}

func (s *MachineManagerSuite) TestUpdateMachineSeriesNoParams(c *gc.C) {
	apiV4 := machinemanager.MachineManagerAPIV4{s.api}
	results, err := apiV4.UpdateMachineSeries(
		params.UpdateSeriesArgs{
			Args: []params.UpdateSeriesArg{},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{}})
}

func (s *MachineManagerSuite) TestUpdateMachineSeriesIncompatibleSeries(c *gc.C) {
	s.setupUpdateMachineSeries(c)
	s.st.machines["0"].SetErrors(&state.ErrIncompatibleSeries{[]string{"yakkety", "zesty"}, "xenial"})
	apiV4 := machinemanager.MachineManagerAPIV4{s.api}
	results, err := apiV4.UpdateMachineSeries(
		params.UpdateSeriesArgs{
			Args: []params.UpdateSeriesArg{{
				Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
				Series: "xenial",
			}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.ErrorResult{
		Error: &params.Error{
			Code:    params.CodeIncompatibleSeries,
			Message: "series \"xenial\" not supported by charm, supported series are: yakkety,zesty",
		},
	})
}

func (s *MachineManagerSuite) TestUpdateMachineSeriesBlockedChanges(c *gc.C) {
	apiV4 := machinemanager.MachineManagerAPIV4{s.api}
	s.st.blockMsg = "TestUpdateMachineSeriesBlockedChanges"
	s.st.block = state.ChangeBlock
	_, err := apiV4.UpdateMachineSeries(
		params.UpdateSeriesArgs{
			Args: []params.UpdateSeriesArg{{
				Entity: params.Entity{
					Tag: names.NewMachineTag("0").String()},
				Series: "xenial",
			}},
		},
	)
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue, gc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), jc.DeepEquals, &params.Error{
		Message: "TestUpdateMachineSeriesBlockedChanges",
		Code:    "operation is blocked",
	})
}

func (s *MachineManagerSuite) TestUpdateMachineSeriesPermissionDenied(c *gc.C) {
	user := names.NewUserTag("fred")
	s.setAPIUser(c, user)
	apiV4 := machinemanager.MachineManagerAPIV4{s.api}
	_, err := apiV4.UpdateMachineSeries(
		params.UpdateSeriesArgs{
			Args: []params.UpdateSeriesArg{{
				Entity: params.Entity{
					Tag: names.NewMachineTag("0").String()},
				Series: "xenial",
			}},
		},
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

type mockState struct {
	machinemanager.Backend
	calls            int
	machineTemplates []state.MachineTemplate
	machines         map[string]*mockMachine
	err              error
	blockMsg         string
	block            state.BlockType
}

func (st *mockState) AddOneMachine(template state.MachineTemplate) (*state.Machine, error) {
	st.calls++
	st.machineTemplates = append(st.machineTemplates, template)
	m := state.Machine{}
	return &m, st.err
}

func (st *mockState) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	if st.block == t {
		return &mockBlock{t: t, m: st.blockMsg}, true, nil
	} else {
		return nil, false, nil
	}
}

func (st *mockState) ModelTag() names.ModelTag {
	return names.NewModelTag("deadbeef-2f18-4fd2-967d-db9663db7bea")
}

func (st *mockState) Model() (machinemanager.Model, error) {
	return &mockModel{}, nil
}

func (st *mockState) CloudCredential(tag names.CloudCredentialTag) (cloud.Credential, error) {
	return cloud.Credential{}, nil
}

func (st *mockState) Cloud(string) (cloud.Cloud, error) {
	return cloud.Cloud{}, nil
}

func (st *mockState) Machine(id string) (machinemanager.Machine, error) {
	if m, ok := st.machines[id]; !ok {
		return nil, errors.NotFoundf("machine %v", id)
	} else {
		return m, nil
	}
}

func (st *mockState) StorageInstance(tag names.StorageTag) (state.StorageInstance, error) {
	return &mockStorage{
		tag:  tag,
		kind: state.StorageKindBlock,
	}, nil
}

func (st *mockState) StorageInstanceVolume(tag names.StorageTag) (state.Volume, error) {
	return &mockVolume{
		detachable: tag.Id() == "disks/0",
	}, nil
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
	t state.BlockType
	m string
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
	return st.m
}

func (st *mockBlock) ModelUUID() string {
	return "uuid"
}

type mockMachine struct {
	jtesting.Stub
	machinemanager.Machine

	keep   bool
	series string
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

func (m *mockMachine) Series() string {
	m.MethodCall(m, "Series")
	return m.series
}

func (m *mockMachine) Units() ([]machinemanager.Unit, error) {
	return []machinemanager.Unit{
		&mockUnit{names.NewUnitTag("foo/0")},
		&mockUnit{names.NewUnitTag("foo/1")},
		&mockUnit{names.NewUnitTag("foo/2")},
	}, nil
}

func (m *mockMachine) UpdateMachineSeries(series string, force bool) error {
	m.MethodCall(m, "UpdateMachineSeries", series, force)
	return m.NextErr()
}

type mockUnit struct {
	tag names.UnitTag
}

func (u *mockUnit) UnitTag() names.UnitTag {
	return u.tag
}

type mockStorage struct {
	state.StorageInstance
	tag  names.StorageTag
	kind state.StorageKind
}

func (a *mockStorage) StorageTag() names.StorageTag {
	return a.tag
}

func (a *mockStorage) Kind() state.StorageKind {
	return a.kind
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

type mockVolume struct {
	state.Volume
	detachable bool
}

func (v *mockVolume) Detachable() bool {
	return v.detachable
}
