// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"errors"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/machinemanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
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
	s.st = &mockState{}
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
	c.Assert(s.st.machines, jc.DeepEquals, []state.MachineTemplate{
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
			Error: &params.Error{"boom", ""},
		}},
	})
	c.Assert(s.st.calls, gc.Equals, 1)
}

type mockState struct {
	calls    int
	machines []state.MachineTemplate
	err      error
}

func (st *mockState) AddOneMachine(template state.MachineTemplate) (*state.Machine, error) {
	st.calls++
	st.machines = append(st.machines, template)
	m := state.Machine{}
	return &m, st.err
}

func (st *mockState) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	return &mockBlock{}, false, nil
}

func (st *mockState) EnvironConfig() (*config.Config, error) {
	panic("not implemented")
}

func (st *mockState) Environment() (*state.Environment, error) {
	panic("not implemented")
}

func (st *mockState) AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (*state.Machine, error) {
	panic("not implemented")
}

func (st *mockState) AddMachineInsideMachine(template state.MachineTemplate, parentId string, containerType instance.ContainerType) (*state.Machine, error) {
	panic("not implemented")
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

func (st *mockBlock) EnvUUID() string {
	return "uuid"
}
