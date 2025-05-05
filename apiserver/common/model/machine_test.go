// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/naturalsort"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/model"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/status"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type machineSuite struct {
	machineService *MockMachineService
}

var _ = gc.Suite(&machineSuite{})

func (s *machineSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.machineService = NewMockMachineService(ctrl)
	return ctrl
}

func (s *machineSuite) TestMachineHardwareInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	one := uint64(1)
	amd64 := "amd64"
	gig := uint64(1024)
	hw := &instance.HardwareCharacteristics{
		Arch:     &amd64,
		Mem:      &gig,
		CpuCores: &one,
		CpuPower: &one,
	}
	st := mockState{
		machines: map[string]*fakeMachine{
			"1": {id: "1", life: state.Alive, containerType: instance.NONE,
				hw: hw,
			},
			"2": {id: "2", life: state.Alive, containerType: instance.LXD},
			"3": {life: state.Dying},
		},
	}
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("uuid-1", nil)
	s.machineService.EXPECT().InstanceIDAndName(gomock.Any(), machine.UUID("uuid-1")).Return("123", "one-two-three", nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("uuid-2", nil)
	s.machineService.EXPECT().InstanceIDAndName(gomock.Any(), machine.UUID("uuid-2")).Return("456", "four-five-six", nil)
	s.machineService.EXPECT().HardwareCharacteristics(gomock.Any(), machine.UUID("uuid-1")).Return(hw, nil)
	s.machineService.EXPECT().HardwareCharacteristics(gomock.Any(), machine.UUID("uuid-2")).Return(&instance.HardwareCharacteristics{}, nil)
	info, err := model.ModelMachineInfo(context.Background(), &st, s.machineService)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, []params.ModelMachineInfo{
		{
			Id:          "1",
			InstanceId:  "123",
			DisplayName: "one-two-three",
			Hardware: &params.MachineHardware{
				Arch:     &amd64,
				Mem:      &gig,
				Cores:    &one,
				CpuPower: &one,
			},
		}, {
			Id:          "2",
			InstanceId:  "456",
			DisplayName: "four-five-six",
		},
	})
}

func (s *machineSuite) TestMachineMachineNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	one := uint64(1)
	amd64 := "amd64"
	gig := uint64(1024)
	hw := &instance.HardwareCharacteristics{
		Arch:     &amd64,
		Mem:      &gig,
		CpuCores: &one,
		CpuPower: &one,
	}
	st := mockState{
		machines: map[string]*fakeMachine{
			"1": {id: "1", life: state.Alive, containerType: instance.NONE,
				hw: hw,
			},
		},
	}
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("uuid-1", nil)
	s.machineService.EXPECT().InstanceIDAndName(gomock.Any(), machine.UUID("uuid-1")).Return("123", "one-two-three", nil)
	s.machineService.EXPECT().HardwareCharacteristics(gomock.Any(), machine.UUID("uuid-1")).Return(hw, machineerrors.MachineNotFound)
	_, err := model.ModelMachineInfo(context.Background(), &st, s.machineService)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *machineSuite) TestMachineHardwareInfoMachineNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	one := uint64(1)
	amd64 := "amd64"
	gig := uint64(1024)
	hw := &instance.HardwareCharacteristics{
		Arch:     &amd64,
		Mem:      &gig,
		CpuCores: &one,
		CpuPower: &one,
	}
	st := mockState{
		machines: map[string]*fakeMachine{
			"1": {id: "1", life: state.Alive, containerType: instance.NONE,
				hw: hw,
			},
		},
	}
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("uuid-1", machineerrors.MachineNotFound)
	_, err := model.ModelMachineInfo(context.Background(), &st, s.machineService)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *machineSuite) TestMachineInstanceInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	st := mockState{
		machines: map[string]*fakeMachine{
			"1": {
				id:     "1",
				instId: "123",
				status: status.Down,
			},
			"2": {
				id:          "2",
				instId:      "456",
				displayName: "four-five-six",
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
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("uuid-1", nil)
	s.machineService.EXPECT().InstanceIDAndName(gomock.Any(), machine.UUID("uuid-1")).Return("123", "", nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("uuid-2", nil)
	s.machineService.EXPECT().InstanceIDAndName(gomock.Any(), machine.UUID("uuid-2")).Return("456", "four-five-six", nil)
	s.machineService.EXPECT().HardwareCharacteristics(gomock.Any(), machine.UUID("uuid-1")).Return(&instance.HardwareCharacteristics{}, nil)
	s.machineService.EXPECT().HardwareCharacteristics(gomock.Any(), machine.UUID("uuid-2")).Return(&instance.HardwareCharacteristics{}, nil)
	info, err := model.ModelMachineInfo(context.Background(), &st, s.machineService)
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
			DisplayName: "four-five-six",
			Status:      "allocating",
			HasVote:     false,
			WantsVote:   true,
		},
	})
}

func (s *machineSuite) TestMachineInstanceInfoWithEmptyDisplayName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	st := mockState{
		machines: map[string]*fakeMachine{
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
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("uuid-1", nil)
	s.machineService.EXPECT().InstanceIDAndName(gomock.Any(), machine.UUID("uuid-1")).Return("123", "", nil)
	s.machineService.EXPECT().HardwareCharacteristics(gomock.Any(), machine.UUID("uuid-1")).Return(&instance.HardwareCharacteristics{}, nil)
	info, err := model.ModelMachineInfo(context.Background(), &st, s.machineService)
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
	defer s.setupMocks(c).Finish()

	st := mockState{
		machines: map[string]*fakeMachine{
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
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("uuid-1", nil)
	s.machineService.EXPECT().InstanceIDAndName(gomock.Any(), machine.UUID("uuid-1")).Return("123", "snowflake", nil)
	s.machineService.EXPECT().HardwareCharacteristics(gomock.Any(), machine.UUID("uuid-1")).Return(&instance.HardwareCharacteristics{}, nil)
	info, err := model.ModelMachineInfo(context.Background(), &st, s.machineService)
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
	defer s.setupMocks(c).Finish()

	st := mockState{
		machines: map[string]*fakeMachine{
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
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("uuid-1", nil)
	s.machineService.EXPECT().InstanceIDAndName(gomock.Any(), machine.UUID("uuid-1")).Return("123", "snowflake", nil)
	s.machineService.EXPECT().HardwareCharacteristics(gomock.Any(), machine.UUID("uuid-1")).Return(&instance.HardwareCharacteristics{}, nil)
	info, err := model.ModelMachineInfo(context.Background(), &st, s.machineService)
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
	model.ModelManagerBackend
	machines          map[string]*fakeMachine
	controllerNodes   map[string]*mockControllerNode
	haPrimaryMachineF func() (names.MachineTag, error)
}

func (st *mockState) AllMachines() (machines []model.Machine, _ error) {
	// Ensure we get machines in id order.
	var ids []string
	for id := range st.machines {
		ids = append(ids, id)
	}
	naturalsort.Sort(ids)
	for _, id := range ids {
		machines = append(machines, st.machines[id])
	}
	return machines, nil
}

func (st *mockState) ControllerNodes() ([]model.ControllerNode, error) {
	var result []model.ControllerNode
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

type fakeMachine struct {
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

func (m *fakeMachine) Id() string {
	return m.id
}

func (m *fakeMachine) Life() state.Life {
	return m.life
}

func (m *fakeMachine) InstanceId() (instance.Id, error) {
	return m.instId, nil
}

func (m *fakeMachine) InstanceNames() (instance.Id, string, error) {
	instId, err := m.InstanceId()
	return instId, m.displayName, err
}

func (m *fakeMachine) Status() (status.StatusInfo, error) {
	return status.StatusInfo{
		Status: m.status,
	}, m.statusErr
}

func (m *fakeMachine) HardwareCharacteristics() (*instance.HardwareCharacteristics, error) {
	return m.hw, nil
}

func (m *fakeMachine) ForceDestroy(time.Duration) error {
	m.forceDestroyCalled = true
	if m.forceDestroyErr != nil {
		return m.forceDestroyErr
	}
	m.life = state.Dying
	return nil
}

func (m *fakeMachine) Destroy(_ objectstore.ObjectStore) error {
	m.destroyCalled = true
	if m.destroyErr != nil {
		return m.destroyErr
	}
	m.life = state.Dying
	return nil
}
