// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common/model"
	"github.com/juju/juju/core/instance"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
)

type machineSuite struct {
	machineService *MockMachineService
	statusService  *MockStatusService
}

func TestMachineSuite(t *testing.T) {
	tc.Run(t, &machineSuite{})
}

func (s *machineSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.machineService = NewMockMachineService(ctrl)
	s.statusService = NewMockStatusService(ctrl)

	c.Cleanup(func() {
		s.machineService = nil
		s.statusService = nil
	})

	return ctrl
}

func (s *machineSuite) TestMachineHardwareInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machine1Name := machine.Name("1")
	machine1UUID := machine.GenUUID(c)
	machine2Name := machine.Name("2")
	machine2UUID := machine.GenUUID(c)
	machine3Name := machine.Name("3")

	s.machineService.EXPECT().AllMachineNames(gomock.Any()).Return([]machine.Name{
		machine1Name, machine2Name, machine3Name,
	}, nil)
	s.statusService.EXPECT().GetAllMachineStatuses(gomock.Any()).Return(map[machine.Name]status.StatusInfo{
		"1": {
			Status:  status.Down,
			Message: "it's down",
		},
		"2": {
			Status:  status.Allocating,
			Message: "it's allocating",
		},
	}, nil)

	s.machineService.EXPECT().GetMachineLife(gomock.Any(), machine1Name).Return(corelife.Alive, nil)
	s.machineService.EXPECT().GetMachineLife(gomock.Any(), machine2Name).Return(corelife.Alive, nil)
	s.machineService.EXPECT().GetMachineLife(gomock.Any(), machine3Name).Return(corelife.Dying, nil)

	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine1Name).Return(machine1UUID, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine2Name).Return(machine2UUID, nil)

	s.machineService.EXPECT().GetInstanceIDAndName(gomock.Any(), machine1UUID).Return("123", "one-two-three", nil)
	s.machineService.EXPECT().GetInstanceIDAndName(gomock.Any(), machine2UUID).Return("456", "four-five-six", nil)

	s.machineService.EXPECT().GetSupportedContainersTypes(gomock.Any(), machine1UUID).Return([]instance.ContainerType{}, nil)
	s.machineService.EXPECT().GetSupportedContainersTypes(gomock.Any(), machine2UUID).Return([]instance.ContainerType{instance.LXD}, nil)

	one := uint64(1)
	amd64 := "amd64"
	gig := uint64(1024)
	hw := &instance.HardwareCharacteristics{
		Arch:     &amd64,
		Mem:      &gig,
		CpuCores: &one,
		CpuPower: &one,
	}
	s.machineService.EXPECT().GetHardwareCharacteristics(gomock.Any(), machine1UUID).Return(hw, nil)

	info, err := model.ModelMachineInfo(c.Context(), s.machineService, s.statusService)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, []params.ModelMachineInfo{
		{
			Id:          "1",
			InstanceId:  "123",
			DisplayName: "one-two-three",
			Status:      "down",
			Message:     "it's down",
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
			Status:      "allocating",
			Message:     "it's allocating",
		},
	})
}
