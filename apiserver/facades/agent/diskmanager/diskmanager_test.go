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
		Provenance: blockdevice.MachineProvenance,
	}, {
		DeviceName: "sdb",
		Provenance: blockdevice.MachineProvenance,
	}}

	s.machineService.EXPECT().GetMachineUUID(
		gomock.Any(), machine.Name("0")).Return(machineUUID, nil)
	s.blockDeviceService.EXPECT().UpdateBlockDevicesForMachine(
		gomock.Any(), machineUUID, expectedDevices).Return(nil)

	results, err := s.api.SetMachineBlockDevices(c.Context(), params.SetMachineBlockDevices{
		MachineBlockDevices: []params.MachineBlockDevices{{
			Machine: "machine-0",
			BlockDevices: []params.BlockDevice{{
				DeviceName: "sda",
				Provenance: params.BlockDeviceProvenanceMachine,
			}, {
				DeviceName: "sdb",
				Provenance: params.BlockDeviceProvenanceMachine,
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
	s.blockDeviceService.EXPECT().UpdateBlockDevicesForMachine(
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

func TestDiskManagerV2Suite(t *testing.T) {
	tc.Run(t, &DiskManagerV2Suite{})
}

type DiskManagerV2Suite struct {
	authorizer *apiservertesting.FakeAuthorizer
	api        *DiskManagerAPIV2

	machineService     *MockMachineService
	blockDeviceService *MockBlockDeviceService
}

func (s *DiskManagerV2Suite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	tag := names.NewMachineTag("0")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}

	s.machineService = NewMockMachineService(ctrl)
	s.blockDeviceService = NewMockBlockDeviceService(ctrl)

	s.api = &DiskManagerAPIV2{
		DiskManagerAPI: &DiskManagerAPI{
			machineService:     s.machineService,
			blockDeviceService: s.blockDeviceService,
			authorizer:         s.authorizer,
			getAuthFunc: func(ctx context.Context) (common.AuthFunc, error) {
				return func(t names.Tag) bool {
					return t == tag
				}, nil
			},
		},
	}

	c.Cleanup(func() {
		s.authorizer = nil
		s.machineService = nil
		s.blockDeviceService = nil
	})

	return ctrl
}

// TestV2SetMachineBlockDevicesSetsProvenanceMachine verifies that V2
// sets provenance on supplied block devices.
func (s *DiskManagerV2Suite) TestV2SetMachineBlockDevicesSetsProvenanceMachine(
	c *tc.C,
) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)
	expectedDevices := []blockdevice.BlockDevice{{
		DeviceName: "sda",
		Provenance: blockdevice.MachineProvenance,
	}, {
		DeviceName: "sdb",
		Provenance: blockdevice.MachineProvenance,
	}}

	s.machineService.EXPECT().GetMachineUUID(
		gomock.Any(), machine.Name("0")).Return(machineUUID, nil)
	s.blockDeviceService.EXPECT().UpdateBlockDevicesForMachine(
		gomock.Any(), machineUUID, expectedDevices).Return(nil)

	results, err := s.api.SetMachineBlockDevices(
		c.Context(),
		params.SetMachineBlockDevices{
			MachineBlockDevices: []params.MachineBlockDevices{{
				Machine: "machine-0",
				// Devices arrive with ProviderProvenance; V2 must
				// replace it with MachineProvenance before forwarding.
				BlockDevices: []params.BlockDevice{{
					DeviceName: "sda",
				}, {
					DeviceName: "sdb",
				}},
			}},
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
}
