// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
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
		name: multiwatcher.JobManageNetworking,
		want: state.JobManageNetworking,
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

type mockState struct {
	state.State
	machines map[string]*mockMachine
}

func (st *mockState) Machine(id string) (common.Machine, error) {
	if m, ok := st.machines[id]; ok {
		return m, nil
	}
	return nil, errors.Errorf("machine %s does not exist", id)
}

type mockMachine struct {
	state.Machine
	life               state.Life
	destroyErr         error
	forceDestroyErr    error
	forceDestroyCalled bool
	destroyCalled      bool
}

func (m *mockMachine) Life() state.Life {
	return m.life
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
