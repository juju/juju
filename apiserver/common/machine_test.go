// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/naturalsort"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type machineSuite struct{}

var _ = gc.Suite(&machineSuite{})

func (s *machineSuite) TestMachineJobFromParams(c *gc.C) {
	var tests = []struct {
		name model.MachineJob
		want state.MachineJob
		err  string
	}{{
		name: model.JobHostUnits,
		want: state.JobHostUnits,
	}, {
		name: model.JobManageModel,
		want: state.JobManageModel,
	}, {
		name: "invalid",
		want: -1,
		err:  `invalid machine job "invalid"`,
	}}
	for _, test := range tests {
		got, err := common.MachineJobFromParams(test.name)
		if err != nil {
			c.Check(err, gc.ErrorMatches, test.err)
		}
		c.Check(got, gc.Equals, test.want)
	}
}

const (
	dontWait = time.Duration(0)
)

func (s *machineSuite) TestDestroyMachines(c *gc.C) {
	st := mockState{
		machines: map[string]*mockMachine{
			"1": {},
			"2": {destroyErr: errors.New("unit exists error")},
			"3": {life: state.Dying},
		},
	}
	err := common.MockableDestroyMachines(&st, false, dontWait, "1", "2", "3", "4")

	c.Assert(st.machines["1"].Life(), gc.Equals, state.Dying)
	c.Assert(st.machines["1"].forceDestroyCalled, jc.IsFalse)

	c.Assert(st.machines["2"].Life(), gc.Equals, state.Alive)
	c.Assert(st.machines["2"].forceDestroyCalled, jc.IsFalse)

	c.Assert(st.machines["3"].forceDestroyCalled, jc.IsFalse)
	c.Assert(st.machines["3"].destroyCalled, jc.IsFalse)

	c.Assert(err, gc.ErrorMatches, "some machines were not destroyed: unit exists error; machine 4 does not exist")
}

func (s *machineSuite) TestForceDestroyMachines(c *gc.C) {
	st := mockState{
		machines: map[string]*mockMachine{
			"1": {},
			"2": {life: state.Dying},
		},
	}
	err := common.MockableDestroyMachines(&st, true, dontWait, "1", "2")

	c.Assert(st.machines["1"].Life(), gc.Equals, state.Dying)
	c.Assert(st.machines["1"].forceDestroyCalled, jc.IsTrue)
	c.Assert(st.machines["2"].forceDestroyCalled, jc.IsTrue)

	c.Assert(err, jc.ErrorIsNil)
}

func (s *machineSuite) TestMachineHardwareInfo(c *gc.C) {
	one := uint64(1)
	amd64 := "amd64"
	gig := uint64(1024)
	st := mockState{
		machines: map[string]*mockMachine{
			"1": {id: "1", life: state.Alive, containerType: instance.NONE,
				hw: &instance.HardwareCharacteristics{
					Arch:     &amd64,
					Mem:      &gig,
					CpuCores: &one,
					CpuPower: &one,
				}},
			"2": {id: "2", life: state.Alive, containerType: instance.LXD},
			"3": {life: state.Dying},
		},
	}
	info, err := common.ModelMachineInfo(&st)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, []params.ModelMachineInfo{
		{
			Id:          "1",
			DisplayName: "",
			Hardware: &params.MachineHardware{
				Arch:     &amd64,
				Mem:      &gig,
				Cores:    &one,
				CpuPower: &one,
			},
		}, {
			Id:          "2",
			DisplayName: "",
		},
	})
}

func (s *machineSuite) TestMachineInstanceInfo(c *gc.C) {
	st := mockState{
		machines: map[string]*mockMachine{
			"1": {
				id:     "1",
				instId: "123",
				status: status.Down,
			},
			"2": {
				id:          "2",
				instId:      "456",
				displayName: "two",
				status:      status.Allocating,
			},
		},
		controllerNodes: map[string]*mockControllerNode{
			"1": {
				id:        "1",
				hasVote:   true,
				wantsVote: true,
			},
			"2": {
				id:        "2",
				hasVote:   false,
				wantsVote: true,
			},
		},
	}
	info, err := common.ModelMachineInfo(&st)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, []params.ModelMachineInfo{
		{
			Id:         "1",
			InstanceId: "123",
			Status:     "down",
			HasVote:    true,
			WantsVote:  true,
		},
		{
			Id:          "2",
			InstanceId:  "456",
			DisplayName: "two",
			Status:      "allocating",
			HasVote:     false,
			WantsVote:   true,
		},
	})
}

func (s *machineSuite) TestMachineInstanceInfoWithEmptyDisplayName(c *gc.C) {
	st := mockState{
		machines: map[string]*mockMachine{
			"1": {
				id:          "1",
				instId:      "123",
				displayName: "",
				status:      status.Down,
			},
		},
		controllerNodes: map[string]*mockControllerNode{
			"1": {
				id:        "1",
				hasVote:   true,
				wantsVote: true,
			},
		},
	}
	info, err := common.ModelMachineInfo(&st)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, []params.ModelMachineInfo{
		{
			Id:          "1",
			InstanceId:  "123",
			DisplayName: "",
			Status:      "down",
			HasVote:     true,
			WantsVote:   true,
		},
	})
}

func (s *machineSuite) TestMachineInstanceInfoWithSetDisplayName(c *gc.C) {
	st := mockState{
		machines: map[string]*mockMachine{
			"1": {
				id:          "1",
				instId:      "123",
				displayName: "snowflake",
				status:      status.Down,
			},
		},
		controllerNodes: map[string]*mockControllerNode{
			"1": {
				id:        "1",
				hasVote:   true,
				wantsVote: true,
			},
		},
	}
	info, err := common.ModelMachineInfo(&st)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, []params.ModelMachineInfo{
		{
			Id:          "1",
			InstanceId:  "123",
			DisplayName: "snowflake",
			Status:      "down",
			HasVote:     true,
			WantsVote:   true,
		},
	})
}

func (s *machineSuite) TestMachineInstanceInfoWithHAPrimary(c *gc.C) {
	st := mockState{
		machines: map[string]*mockMachine{
			"1": {
				id:          "1",
				instId:      "123",
				displayName: "snowflake",
				status:      status.Down,
			},
		},
		controllerNodes: map[string]*mockControllerNode{
			"1": {
				id:        "1",
				hasVote:   true,
				wantsVote: true,
			},
			"2": {
				id:        "1",
				hasVote:   true,
				wantsVote: true,
			},
		},
		haPrimaryMachineF: func() (names.MachineTag, error) {
			return names.NewMachineTag("1"), nil
		},
	}
	info, err := common.ModelMachineInfo(&st)
	c.Assert(err, jc.ErrorIsNil)
	_true := true
	c.Assert(info, jc.DeepEquals, []params.ModelMachineInfo{
		{
			Id:          "1",
			InstanceId:  "123",
			DisplayName: "snowflake",
			Status:      "down",
			HasVote:     true,
			WantsVote:   true,
			HAPrimary:   &_true,
		},
	})
}

type mockState struct {
	common.ModelManagerBackend
	machines          map[string]*mockMachine
	controllerNodes   map[string]*mockControllerNode
	haPrimaryMachineF func() (names.MachineTag, error)
}

func (st *mockState) Machine(id string) (common.Machine, error) {
	if m, ok := st.machines[id]; ok {
		return m, nil
	}
	return nil, errors.Errorf("machine %s does not exist", id)
}

func (st *mockState) AllMachines() (machines []common.Machine, err error) {
	// Ensure we get machines in id order.
	var ids []string
	for id := range st.machines {
		ids = append(ids, id)
	}
	_, err = naturalsort.Sort(ids)
	if err != nil {
		return nil, err
	}

	for _, id := range ids {
		machines = append(machines, st.machines[id])
	}
	return machines, nil
}

func (st *mockState) ControllerNodes() ([]common.ControllerNode, error) {
	var result []common.ControllerNode
	for _, n := range st.controllerNodes {
		result = append(result, n)
	}
	return result, nil
}

func (st *mockState) HAPrimaryMachine() (names.MachineTag, error) {
	if st.haPrimaryMachineF == nil {
		return names.MachineTag{}, nil
	}
	return st.haPrimaryMachineF()
}

type mockControllerNode struct {
	id        string
	hasVote   bool
	wantsVote bool
}

func (m *mockControllerNode) Id() string {
	return m.id
}

func (m *mockControllerNode) WantsVote() bool {
	return m.wantsVote
}

func (m *mockControllerNode) HasVote() bool {
	return m.hasVote
}

type mockMachine struct {
	state.Machine
	id                 string
	life               state.Life
	containerType      instance.ContainerType
	hw                 *instance.HardwareCharacteristics
	instId             instance.Id
	displayName        string
	status             status.Status
	statusErr          error
	destroyErr         error
	forceDestroyErr    error
	forceDestroyCalled bool
	destroyCalled      bool
}

func (m *mockMachine) Id() string {
	return m.id
}

func (m *mockMachine) Life() state.Life {
	return m.life
}

func (m *mockMachine) InstanceId() (instance.Id, error) {
	return m.instId, nil
}

func (m *mockMachine) InstanceNames() (instance.Id, string, error) {
	instId, err := m.InstanceId()
	return instId, m.displayName, err
}

func (m *mockMachine) Status() (status.StatusInfo, error) {
	return status.StatusInfo{
		Status: m.status,
	}, m.statusErr
}

func (m *mockMachine) HardwareCharacteristics() (*instance.HardwareCharacteristics, error) {
	return m.hw, nil
}

func (m *mockMachine) ForceDestroy(time.Duration) error {
	m.forceDestroyCalled = true
	if m.forceDestroyErr != nil {
		return m.forceDestroyErr
	}
	m.life = state.Dying
	return nil
}

func (m *mockMachine) Destroy() error {
	m.destroyCalled = true
	if m.destroyErr != nil {
		return m.destroyErr
	}
	m.life = state.Dying
	return nil
}
