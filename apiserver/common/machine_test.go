// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/status"
)

type machineSuite struct{}

var _ = gc.Suite(&machineSuite{})

func (s *machineSuite) TestMachineJobFromParams(c *gc.C) {
	var tests = []struct {
		name multiwatcher.MachineJob
		want state.MachineJob
		err  string
	}{{
		name: multiwatcher.JobHostUnits,
		want: state.JobHostUnits,
	}, {
		name: multiwatcher.JobManageModel,
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

func (s *machineSuite) TestDestroyMachines(c *gc.C) {
	st := mockState{
		machines: map[string]*mockMachine{
			"1": {},
			"2": {destroyErr: errors.New("unit exists error")},
			"3": {life: state.Dying},
		},
	}
	err := common.MockableDestroyMachines(&st, false, "1", "2", "3", "4")

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
	err := common.MockableDestroyMachines(&st, true, "1", "2")

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
			Id: "1",
			Hardware: &params.MachineHardware{
				Arch:     &amd64,
				Mem:      &gig,
				Cores:    &one,
				CpuPower: &one,
			},
		}, {
			Id: "2",
		},
	})
}

func (s *machineSuite) TestMachineInstanceInfo(c *gc.C) {
	st := mockState{
		machines: map[string]*mockMachine{
			"1": {id: "1", instId: instance.Id("123"), status: status.Down, hasVote: true, wantsVote: true},
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
	})
}

type mockState struct {
	common.ModelManagerBackend
	machines map[string]*mockMachine
}

func (st *mockState) Machine(id string) (common.Machine, error) {
	if m, ok := st.machines[id]; ok {
		return m, nil
	}
	return nil, errors.Errorf("machine %s does not exist", id)
}

func (st *mockState) AllMachines() (machines []common.Machine, _ error) {
	// Ensure we get machines in id order.
	var ids []string
	for id := range st.machines {
		ids = append(ids, id)
	}
	utils.SortStringsNaturally(ids)
	for _, id := range ids {
		machines = append(machines, st.machines[id])
	}
	return machines, nil
}

type mockMachine struct {
	state.Machine
	id                 string
	life               state.Life
	containerType      instance.ContainerType
	hw                 *instance.HardwareCharacteristics
	instId             instance.Id
	hasVote, wantsVote bool
	status             status.Status
	statusErr          error
	destroyErr         error
	forceDestroyErr    error
	forceDestroyCalled bool
	destroyCalled      bool
	agentDead          bool
	presenceErr        error
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

func (m *mockMachine) WantsVote() bool {
	return m.wantsVote
}

func (m *mockMachine) HasVote() bool {
	return m.hasVote
}

func (m *mockMachine) Status() (status.StatusInfo, error) {
	return status.StatusInfo{
		Status: m.status,
	}, m.statusErr
}

func (m *mockMachine) HardwareCharacteristics() (*instance.HardwareCharacteristics, error) {
	return m.hw, nil
}

func (m *mockMachine) AgentPresence() (bool, error) {
	return !m.agentDead, m.presenceErr
}

func (m *mockMachine) ForceDestroy() error {
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
