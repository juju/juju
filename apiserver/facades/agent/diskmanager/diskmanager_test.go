// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import (
	"context"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/rpc/params"
)

func TestDiskManagerSuite(t *testing.T) {
	tc.Run(t, &DiskManagerSuite{})
}

type DiskManagerSuite struct {
	authorizer *apiservertesting.FakeAuthorizer
	api        *DiskManagerAPI

	machineService     *MockMachineService
	blockDeviceService *MockBlockDeviceService
}

func (s *DiskManagerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	tag := names.NewMachineTag("0")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}

	s.machineService = NewMockMachineService(ctrl)
	s.blockDeviceService = NewMockBlockDeviceService(ctrl)

	s.api = &DiskManagerAPI{
		machineService:     s.machineService,
		blockDeviceService: s.blockDeviceService,
		authorizer:         s.authorizer,
		getAuthFunc: func(ctx context.Context) (common.AuthFunc, error) {
			return func(t names.Tag) bool {
				return t == tag
			}, nil
		},
	}

	c.Cleanup(func() {
		s.authorizer = nil
		s.machineService = nil
		s.blockDeviceService = nil
	})

	return ctrl
}

func (s *DiskManagerSuite) TestSetMachineBlockDevices(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)
	expectedDevices := []blockdevice.BlockDevice{{
		DeviceName: "sda",
	}, {
		DeviceName: "sdb",
	}}

	s.machineService.EXPECT().GetMachineUUID(
		gomock.Any(), machine.Name("0")).Return(machineUUID, nil)
	s.blockDeviceService.EXPECT().UpdateBlockDevices(
		gomock.Any(), machineUUID, expectedDevices).Return(nil)

	results, err := s.api.SetMachineBlockDevices(c.Context(), params.SetMachineBlockDevices{
		MachineBlockDevices: []params.MachineBlockDevices{{
			Machine: "machine-0",
			BlockDevices: []params.BlockDevice{{
				DeviceName: "sda",
			}, {
				DeviceName: "sdb",
			}},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesEmptyArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	results, err := s.api.SetMachineBlockDevices(c.Context(), params.SetMachineBlockDevices{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 0)
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesInvalidTags(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)

	s.machineService.EXPECT().GetMachineUUID(
		gomock.Any(), machine.Name("0")).Return(machineUUID, nil)
	s.blockDeviceService.EXPECT().UpdateBlockDevices(
		gomock.Any(), machineUUID, nil).Return(nil)

	results, err := s.api.SetMachineBlockDevices(c.Context(), params.SetMachineBlockDevices{
		MachineBlockDevices: []params.MachineBlockDevices{{
			Machine: "machine-0",
		}, {
			Machine: "machine-1",
		}, {
			Machine: "unit-mysql-0",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: nil,
		}, {
			Error: &params.Error{Message: "permission denied", Code: "unauthorized access"},
		}, {
			Error: &params.Error{Message: "permission denied", Code: "unauthorized access"},
		}},
	})
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machineService.EXPECT().GetMachineUUID(
		gomock.Any(), machine.Name("0")).Return("", machineerrors.MachineNotFound)

	results, err := s.api.SetMachineBlockDevices(c.Context(), params.SetMachineBlockDevices{
		MachineBlockDevices: []params.MachineBlockDevices{{
			Machine: "machine-0",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{Message: `machine "0" not found`, Code: "not found"},
		}},
	})
}
