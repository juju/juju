// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"context"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/network/errors"
	domainprovisioner "github.com/juju/juju/domain/provisioner"
	internalerrors "github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type provisionerMockSuite struct {
	coretesting.BaseSuite

	clock                   clock.Clock
	applicationService      *MockApplicationService
	machineService          *MockMachineService
	statusService           *MockStatusService
	networkService          *MockNetworkService
	removalService          *MockRemovalService
	controllerConfigService *MockControllerConfigService
	provisioningService     *MockProvisioningService

	authorizer *facademocks.MockAuthorizer

	api *ProvisionerAPI
}

func TestProvisionerMockSuite(t *testing.T) {
	tc.Run(t, &provisionerMockSuite{})
}

func (s *provisionerMockSuite) TestEnsureDead(c *tc.C) {
	defer s.setup(c).Finish()

	machineName := coremachine.Name("1")
	machineUUID := machinetesting.GenUUID(c)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machineName).Return(machineUUID, nil)
	s.removalService.EXPECT().MarkMachineAsDead(gomock.Any(), machineUUID).Return(nil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
	}}
	result, err := s.api.EnsureDead(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})
}

func (s *provisionerMockSuite) TestEnsureDeadMachineNotFound(c *tc.C) {
	defer s.setup(c).Finish()

	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("1")).Return("", machineerrors.MachineNotFound)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
	}}
	result, err := s.api.EnsureDead(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *provisionerMockSuite) TestHostChangesForContainers(c *tc.C) {
	defer s.setup(c).Finish()

	s.authorizer.EXPECT().GetAuthTag().Return(names.NewMachineTag("0"))

	expMach := s.machineService.EXPECT()
	expMach.GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return("m1-uuid", nil)
	expMach.GetMachineUUID(gomock.Any(), coremachine.Name("0/lxd/0")).Return("m2-uuid", nil)

	s.networkService.EXPECT().DevicesToBridge(
		gomock.Any(), coremachine.UUID("m1-uuid"), coremachine.UUID("m2-uuid"),
	).Return([]domainnetwork.DeviceToBridge{{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
		MACAddress: "mac-address",
	}}, nil)

	res, err := s.api.HostChangesForContainers(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: "machine-0-lxd-0",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(res, tc.DeepEquals, params.HostNetworkChangeResults{
		Results: []params.HostNetworkChange{{
			NewBridges: []params.DeviceBridgeInfo{{
				HostDeviceName: "eth0",
				BridgeName:     "br-eth0",
				MACAddress:     "mac-address",
			}},
		}},
	})
}

func (s *provisionerMockSuite) TestPrepareContainerInterfaceInfoNoAddrAllocation(c *tc.C) {
	defer s.setup(c).Finish()

	hostUUID := machinetesting.GenUUID(c)
	guestUUID := machinetesting.GenUUID(c)
	hostInstanceID := instance.Id("m0-instance-id")

	s.authorizer.EXPECT().GetAuthTag().Return(names.NewMachineTag("0"))

	expMach := s.machineService.EXPECT()
	expMach.GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return(hostUUID, nil)
	expMach.GetInstanceID(gomock.Any(), hostUUID).Return(hostInstanceID, nil)
	expMach.GetMachineUUID(gomock.Any(), coremachine.Name("0/lxd/0")).Return(guestUUID, nil)

	s.networkService.EXPECT().DevicesForGuest(gomock.Any(), hostUUID, guestUUID).Return([]domainnetwork.NetInterface{{
		MACAddress:       new("some:mac:address"),
		Name:             "eth0",
		ParentDeviceName: "br-eth0",
		Type:             network.EthernetDevice,
		IsAutoStart:      true,
		IsEnabled:        true,
		Addrs: []domainnetwork.NetAddr{{
			AddressValue: "192.168.0.0/24",
			ConfigType:   network.ConfigDHCP,
		}},
	}}, nil)

	preparedInfo := network.InterfaceInfos{{
		MACAddress:          "some:mac:address",
		InterfaceName:       "eth0",
		ParentInterfaceName: "br-eth0",
		InterfaceType:       network.EthernetDevice,
		ConfigType:          network.ConfigDHCP,
		Addresses: network.ProviderAddresses{{MachineAddress: network.MachineAddress{
			CIDR:       "192.168.0.0/24",
			ConfigType: network.ConfigDHCP,
		}}},
	}}

	s.networkService.EXPECT().AllocateContainerAddresses(
		gomock.Any(), hostInstanceID, "0/lxd/0", preparedInfo).Return(nil, errors.ContainerAddressesNotSupported)

	res, err := s.api.PrepareContainerInterfaceInfo(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: "machine-0-lxd-0",
		}},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.MachineNetworkConfigResults{
		Results: []params.MachineNetworkConfigResult{{
			Error: nil,
			Config: []params.NetworkConfig{{
				MACAddress:          "some:mac:address",
				InterfaceName:       "eth0",
				ParentInterfaceName: "br-eth0",
				InterfaceType:       "ethernet",
				ConfigType:          "dhcp",
				Addresses: []params.Address{{
					CIDR:       "192.168.0.0/24",
					ConfigType: "dhcp",
				}},
			}},
		}},
	})
}

func (s *provisionerMockSuite) TestPrepareContainerInterfaceInfoProviderAddrAllocation(c *tc.C) {
	defer s.setup(c).Finish()

	hostUUID := machinetesting.GenUUID(c)
	guestUUID := machinetesting.GenUUID(c)
	hostInstanceID := instance.Id("m0-instance-id")

	s.authorizer.EXPECT().GetAuthTag().Return(names.NewMachineTag("0"))

	expMach := s.machineService.EXPECT()
	expMach.GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return(hostUUID, nil)
	expMach.GetInstanceID(gomock.Any(), hostUUID).Return(hostInstanceID, nil)
	expMach.GetMachineUUID(gomock.Any(), coremachine.Name("0/lxd/0")).Return(guestUUID, nil)

	s.networkService.EXPECT().DevicesForGuest(gomock.Any(), hostUUID, guestUUID).Return([]domainnetwork.NetInterface{{
		MACAddress:       new("some:mac:address"),
		Name:             "eth0",
		ParentDeviceName: "br-eth0",
		Type:             network.EthernetDevice,
		IsAutoStart:      true,
		IsEnabled:        true,
		Addrs: []domainnetwork.NetAddr{{
			AddressValue: "192.168.0.0/24",
			ConfigType:   network.ConfigStatic,
		}},
	}}, nil)

	preparedInfo := network.InterfaceInfos{{
		MACAddress:          "some:mac:address",
		InterfaceName:       "eth0",
		ParentInterfaceName: "br-eth0",
		InterfaceType:       network.EthernetDevice,
		ConfigType:          network.ConfigStatic,
		Addresses: network.ProviderAddresses{{MachineAddress: network.MachineAddress{
			CIDR:       "192.168.0.0/24",
			ConfigType: network.ConfigStatic,
		}}},
	}}

	allocatedInfo := network.InterfaceInfos{{
		MACAddress:          "some:other:mac:address",
		InterfaceName:       "eth0",
		ParentInterfaceName: "br-eth0",
		InterfaceType:       network.EthernetDevice,
		ConfigType:          network.ConfigStatic,
		Addresses: network.ProviderAddresses{{MachineAddress: network.MachineAddress{
			Value:      "192.168.0.6",
			CIDR:       "192.168.0.0/24",
			ConfigType: network.ConfigStatic,
		}}},
	}}

	s.networkService.EXPECT().AllocateContainerAddresses(
		gomock.Any(), hostInstanceID, "0/lxd/0", preparedInfo).Return(allocatedInfo, nil)

	res, err := s.api.PrepareContainerInterfaceInfo(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: "machine-0-lxd-0",
		}},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.MachineNetworkConfigResults{
		Results: []params.MachineNetworkConfigResult{{
			Error: nil,
			Config: []params.NetworkConfig{{
				MACAddress:          "some:other:mac:address",
				InterfaceName:       "eth0",
				ParentInterfaceName: "br-eth0",
				InterfaceType:       "ethernet",
				ConfigType:          "static",
				Addresses: []params.Address{{
					Value:      "192.168.0.6",
					CIDR:       "192.168.0.0/24",
					ConfigType: "static",
				}},
			}},
		}},
	})
}

func (s *provisionerMockSuite) TestStatusSuccess(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.statusService.EXPECT().GetMachineStatus(gomock.Any(), coremachine.Name("0")).Return(status.StatusInfo{
		Status:  status.Active,
		Message: "blah",
		Data:    map[string]any{"foo": "bar"},
		Since:   &time.Time{},
	}, nil)

	result, err := s.api.Status(c.Context(), params.Entities{Entities: []params.Entity{{
		Tag: "machine-0",
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{{
			Status: status.Active.String(),
			Info:   "blah",
			Data:   map[string]any{"foo": "bar"},
			Since:  &time.Time{},
		}},
	})
}

func (s *provisionerMockSuite) TestStatusNotFound(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.statusService.EXPECT().GetMachineStatus(gomock.Any(), coremachine.Name("0")).Return(status.StatusInfo{}, machineerrors.MachineNotFound)

	result, err := s.api.Status(c.Context(), params.Entities{Entities: []params.Entity{{
		Tag: "machine-0",
	}}})

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *provisionerMockSuite) TestStatusInvalidTags(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	result, err := s.api.Status(c.Context(), params.Entities{Entities: []params.Entity{
		{Tag: "application-unknown"},
		{Tag: "invalid-tag"},
		{Tag: "unit-missing-1"},
		{Tag: ""},
		{Tag: "42"},
	}})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StatusResults{Results: []params.StatusResult{
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
	}})
}

func (s *provisionerMockSuite) TestSetStatus(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	now := s.clock.Now()

	s.statusService.EXPECT().SetMachineStatus(gomock.Any(), coremachine.Name("0"), status.StatusInfo{
		Status:  status.Error,
		Message: "blah",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	}).Return(nil)

	result, err := s.api.SetStatus(c.Context(), params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    "machine-0",
			Status: status.Error.String(),
			Info:   "blah",
			Data:   map[string]any{"foo": "bar"},
		}}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}}})
}

func (s *provisionerMockSuite) TestSetStatusMachineNotFound(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	now := s.clock.Now()

	s.statusService.EXPECT().SetMachineStatus(gomock.Any(), coremachine.Name("0"), status.StatusInfo{
		Status:  status.Error,
		Message: "blah",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	}).Return(machineerrors.MachineNotFound)

	result, err := s.api.SetStatus(c.Context(), params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    "machine-0",
			Status: status.Error.String(),
			Info:   "blah",
			Data:   map[string]any{"foo": "bar"},
		}}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *provisionerMockSuite) TestSetStatusInvalidTags(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	result, err := s.api.SetStatus(c.Context(), params.SetStatus{Entities: []params.EntityStatusArgs{
		{Tag: "application-unknown"},
		{Tag: "invalid-tag"},
		{Tag: "unit-missing-1"},
		{Tag: ""},
		{Tag: "42"},
	}})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
	}})
}

func (s *provisionerMockSuite) TestInstanceStatusSuccess(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.statusService.EXPECT().GetInstanceStatus(gomock.Any(), coremachine.Name("0")).Return(status.StatusInfo{
		Status:  status.Active,
		Message: "blah",
		Data:    map[string]any{"foo": "bar"},
		Since:   &time.Time{},
	}, nil)

	result, err := s.api.InstanceStatus(c.Context(), params.Entities{Entities: []params.Entity{{
		Tag: "machine-0",
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{{
			Status: status.Active.String(),
			Info:   "blah",
			Data:   map[string]any{"foo": "bar"},
			Since:  &time.Time{},
		}},
	})
}

func (s *provisionerMockSuite) TestInstanceStatusNotFound(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.statusService.EXPECT().GetInstanceStatus(gomock.Any(), coremachine.Name("0")).Return(status.StatusInfo{}, machineerrors.MachineNotFound)

	result, err := s.api.InstanceStatus(c.Context(), params.Entities{Entities: []params.Entity{{
		Tag: "machine-0",
	}}})

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *provisionerMockSuite) TestInstanceStatusInvalidTags(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	result, err := s.api.InstanceStatus(c.Context(), params.Entities{Entities: []params.Entity{
		{Tag: "application-unknown"},
		{Tag: "invalid-tag"},
		{Tag: "unit-missing-1"},
		{Tag: ""},
		{Tag: "42"},
	}})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StatusResults{Results: []params.StatusResult{
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
	}})
}

func (s *provisionerMockSuite) TestSetInstanceStatusSuccess(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	now := s.clock.Now()
	s.statusService.EXPECT().SetInstanceStatus(gomock.Any(), coremachine.Name("0"), status.StatusInfo{
		Status:  status.Active,
		Message: "blah",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	}).Return(nil)

	result, err := s.api.SetInstanceStatus(c.Context(), params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    "machine-0",
			Status: status.Active.String(),
			Info:   "blah",
			Data:   map[string]any{"foo": "bar"},
		}}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}}})
}

func (s *provisionerMockSuite) TestSetInstanceStatusToError(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	now := s.clock.Now()

	s.statusService.EXPECT().SetInstanceStatus(gomock.Any(), coremachine.Name("0"), status.StatusInfo{
		Status:  status.ProvisioningError,
		Message: "blah",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	}).Return(nil)
	s.statusService.EXPECT().SetMachineStatus(gomock.Any(), coremachine.Name("0"), status.StatusInfo{
		Status:  status.Error,
		Message: "blah",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	}).Return(nil)

	result, err := s.api.SetInstanceStatus(c.Context(), params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    "machine-0",
			Status: status.ProvisioningError.String(),
			Info:   "blah",
			Data:   map[string]any{"foo": "bar"},
		}}},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}}})
}

func (s *provisionerMockSuite) TestSetInstanceStatusNotFound(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	now := s.clock.Now()

	s.statusService.EXPECT().SetInstanceStatus(gomock.Any(), coremachine.Name("0"), status.StatusInfo{
		Status:  status.Active,
		Message: "blah",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	}).Return(machineerrors.MachineNotFound)

	result, err := s.api.SetInstanceStatus(c.Context(), params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    "machine-0",
			Status: status.Active.String(),
			Info:   "blah",
			Data:   map[string]any{"foo": "bar"},
		}}},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *provisionerMockSuite) TestSetInstanceStatusInvalidTags(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	result, err := s.api.SetInstanceStatus(c.Context(), params.SetStatus{Entities: []params.EntityStatusArgs{
		{Tag: "application-unknown"},
		{Tag: "invalid-tag"},
		{Tag: "unit-missing-1"},
		{Tag: ""},
		{Tag: "42"},
	}})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
		{Error: apiservertesting.ErrUnauthorized},
	}})
}

func (s *provisionerMockSuite) TestMarkMachinesForRemoval(c *tc.C) {
	defer s.setup(c).Finish()

	machineName := coremachine.Name("1")
	machineUUID := machinetesting.GenUUID(c)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machineName).Return(machineUUID, nil)
	s.removalService.EXPECT().MarkInstanceAsDead(gomock.Any(), machineUUID).Return(nil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
	}}
	result, err := s.api.MarkMachinesForRemoval(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})
}

func (s *provisionerMockSuite) TestMarkMachinesForRemovalNotFound(c *tc.C) {
	defer s.setup(c).Finish()

	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("1")).Return("", machineerrors.MachineNotFound)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
	}}
	result, err := s.api.MarkMachinesForRemoval(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *provisionerMockSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = testclock.NewClock(time.Now())
	s.authorizer = facademocks.NewMockAuthorizer(ctrl)

	s.applicationService = NewMockApplicationService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.statusService = NewMockStatusService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.removalService = NewMockRemovalService(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.provisioningService = NewMockProvisioningService(ctrl)

	// ControllerConfig is fetched once per ProvisioningInfo call (before the loop).
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).
		Return(coretesting.FakeControllerConfig(), nil).AnyTimes()

	s.api = &ProvisionerAPI{
		applicationService:      s.applicationService,
		machineService:          s.machineService,
		statusService:           s.statusService,
		networkService:          s.networkService,
		removalService:          s.removalService,
		controllerConfigService: s.controllerConfigService,
		provisioningService:     s.provisioningService,

		clock:  s.clock,
		logger: loggertesting.WrapCheckLog(c),

		authorizer: s.authorizer,
		getAuthFunc: func(context.Context) (common.AuthFunc, error) {
			return func(tag names.Tag) bool {
				return true
			}, nil
		},
	}

	c.Cleanup(func() {
		s.applicationService = nil
		s.machineService = nil
		s.statusService = nil
		s.networkService = nil
		s.removalService = nil
		s.controllerConfigService = nil
		s.provisioningService = nil
		s.authorizer = nil
		s.api = nil
	})

	return ctrl
}

func TestWithControllerSuite(t *testing.T) {
	tc.Run(t, &withControllerSuite{})
}

type withControllerSuite struct {
	apiAddressAccessor      *MockAPIAddressAccessor
	controllerConfigService *MockControllerConfigService
}

func (s *withControllerSuite) TestAPIAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	addrs := []string{"0.1.2.3:1234"}
	s.apiAddressAccessor.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(addrs, nil)
	provisioner := &ProvisionerAPI{APIAddresser: common.NewAPIAddresser(s.apiAddressAccessor, nil)}

	// Act
	result, err := provisioner.APIAddresses(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsResult{
		Result: []string{"0.1.2.3:1234"},
	})
}

func (s *withControllerSuite) TestCACert(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(coretesting.FakeControllerConfig(), nil)
	provisioner := &ProvisionerAPI{controllerConfigService: s.controllerConfigService}

	// Act
	result, err := provisioner.CACert(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.BytesResult{
		Result: []byte(coretesting.CACert),
	})
}

func (s *withControllerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.apiAddressAccessor = NewMockAPIAddressAccessor(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)

	c.Cleanup(func() {
		s.apiAddressAccessor = nil
		s.controllerConfigService = nil
	})

	return ctrl
}

// TestProvisioningInfoErrorContinues verifies that when GetProvisioningInfo
// fails for one machine in a multi-entity request, it records the error and
// continues processing subsequent machines (the continue statement).
func (s *provisionerMockSuite) TestProvisioningInfoErrorContinues(c *tc.C) {
	defer s.setup(c).Finish()

	// Machine-0: provisioning service returns MachineNotFound.
	s.provisioningService.EXPECT().GetProvisioningInfo(gomock.Any(), coremachine.Name("0"), false, gomock.Any()).
		Return(domainprovisioner.ProvisioningInfo{}, machineerrors.MachineNotFound)

	// Machine-1: provisioning service returns a different error.
	s.provisioningService.EXPECT().GetProvisioningInfo(gomock.Any(), coremachine.Name("1"), false, gomock.Any()).
		Return(domainprovisioner.ProvisioningInfo{}, internalerrors.New("some internal error"))

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-1"},
	}}
	result, err := s.api.ProvisioningInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 2)

	// Machine-0: should have a not-found error with correct code.
	c.Check(result.Results[0].Error, tc.Not(tc.IsNil))
	c.Check(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
	c.Check(result.Results[0].Result, tc.IsNil)

	// Machine-1: should have an error too, but processing was not skipped.
	c.Check(result.Results[1].Error, tc.Not(tc.IsNil))
	c.Check(result.Results[1].Error.Code, tc.Not(tc.Equals), params.CodeNotFound)
	c.Check(result.Results[1].Result, tc.IsNil)
}

// TestProvisioningInfoPermissionDenied verifies that an unparseable tag or
// access denial results in ErrPerm and does not panic.
func (s *provisionerMockSuite) TestProvisioningInfoPermissionDenied(c *tc.C) {
	defer s.setup(c).Finish()

	// Override the auth function to deny access to machine-0.
	s.api.getAuthFunc = func(context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag.Id() != "0"
		}, nil
	}

	// Machine-1 is allowed but GetProvisioningInfo returns MachineNotFound.
	s.provisioningService.EXPECT().GetProvisioningInfo(gomock.Any(), coremachine.Name("1"), false, gomock.Any()).
		Return(domainprovisioner.ProvisioningInfo{}, machineerrors.MachineNotFound)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"}, // denied by auth
		{Tag: "machine-1"}, // allowed but not found
	}}
	result, err := s.api.ProvisioningInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 2)

	// Machine-0: permission denied.
	c.Check(result.Results[0].Error, tc.Not(tc.IsNil))
	c.Check(result.Results[0].Error.Message, tc.Equals, "permission denied")
	c.Check(result.Results[0].Result, tc.IsNil)

	// Machine-1: not found error (proves we continued past machine-0).
	c.Check(result.Results[1].Error, tc.Not(tc.IsNil))
	c.Check(result.Results[1].Error.Code, tc.Equals, params.CodeNotFound)
	c.Check(result.Results[1].Result, tc.IsNil)
}

// TestProvisioningInfoBasicSuccess verifies a basic successful call to
// ProvisioningInfo returns the expected result with base, jobs, and tags.
func (s *provisionerMockSuite) TestProvisioningInfoBasicSuccess(c *tc.C) {
	defer s.setup(c).Finish()

	s.provisioningService.EXPECT().GetProvisioningInfo(gomock.Any(), coremachine.Name("0"), false, gomock.Any()).
		Return(domainprovisioner.ProvisioningInfo{
			Base:             corebase.MakeDefaultBase("ubuntu", "22.04"),
			Jobs:             []coremodel.MachineJob{coremodel.JobHostUnits},
			EndpointBindings: map[string]string{},
			ControllerConfig: map[string]any{"controller-uuid": "ctrl-uuid"},
		}, nil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
	}}
	result, err := s.api.ProvisioningInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.IsNil)
	c.Assert(result.Results[0].Result, tc.Not(tc.IsNil))

	info := result.Results[0].Result
	c.Check(info.Base, tc.DeepEquals, params.Base{Name: "ubuntu", Channel: "22.04/stable"})
	c.Check(info.Jobs, tc.DeepEquals, []coremodel.MachineJob{coremodel.JobHostUnits})
	c.Check(info.ControllerConfig, tc.Not(tc.IsNil))
	c.Check(info.EndpointBindings, tc.DeepEquals, map[string]string{})
}

// TestProvisioningInfoWithStorage verifies that volume params are correctly
// populated in the provisioning info.
func (s *provisionerMockSuite) TestProvisioningInfoWithStorage(c *tc.C) {
	defer s.setup(c).Finish()

	s.provisioningService.EXPECT().GetProvisioningInfo(gomock.Any(), coremachine.Name("0"), false, gomock.Any()).
		Return(domainprovisioner.ProvisioningInfo{
			Base: corebase.MakeDefaultBase("ubuntu", "22.04"),
			Jobs: []coremodel.MachineJob{coremodel.JobHostUnits},
			Volumes: []domainprovisioner.VolumeParams{{
				VolumeID:   "0",
				Provider:   "ebs",
				SizeMiB:    1024,
				Tags:       map[string]string{"env": "test"},
				Attributes: map[string]any{"iops": "3000"},
				Attachment: &domainprovisioner.VolumeAttachmentParams{
					VolumeID:  "0",
					MachineID: "0",
					Provider:  "ebs",
				},
			}},
			ControllerConfig: map[string]any{"controller-uuid": "ctrl-uuid"},
		}, nil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
	}}
	result, err := s.api.ProvisioningInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.IsNil)
	c.Assert(result.Results[0].Result, tc.Not(tc.IsNil))

	info := result.Results[0].Result
	c.Assert(info.Volumes, tc.HasLen, 1)
	c.Check(info.Volumes[0].VolumeTag, tc.Equals, "volume-0")
	c.Check(info.Volumes[0].SizeMiB, tc.Equals, uint64(1024))
	c.Check(info.Volumes[0].Provider, tc.Equals, "ebs")
	c.Check(info.Volumes[0].Tags, tc.DeepEquals, map[string]string{"env": "test"})
	c.Check(info.Volumes[0].Attributes, tc.DeepEquals, map[string]any{"iops": "3000"})
	c.Check(info.Volumes[0].Attachment, tc.Not(tc.IsNil))
	c.Check(info.Volumes[0].Attachment.MachineTag, tc.Equals, "machine-0")
}

// TestProvisioningInfoWithRootDisk verifies that root disk params are
// populated when root-disk-source constraint is set.
func (s *provisionerMockSuite) TestProvisioningInfoWithRootDisk(c *tc.C) {
	defer s.setup(c).Finish()

	s.provisioningService.EXPECT().GetProvisioningInfo(gomock.Any(), coremachine.Name("0"), false, gomock.Any()).
		Return(domainprovisioner.ProvisioningInfo{
			Base: corebase.MakeDefaultBase("ubuntu", "22.04"),
			Jobs: []coremodel.MachineJob{coremodel.JobHostUnits},
			RootDisk: &domainprovisioner.VolumeParams{
				Provider:   "ebs",
				Attributes: map[string]any{"iops": "3000"},
			},
			ControllerConfig: map[string]any{"controller-uuid": "ctrl-uuid"},
		}, nil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
	}}
	result, err := s.api.ProvisioningInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.IsNil)
	c.Assert(result.Results[0].Result, tc.Not(tc.IsNil))

	info := result.Results[0].Result
	c.Assert(info.RootDisk, tc.Not(tc.IsNil))
	c.Check(info.RootDisk.Provider, tc.Equals, "ebs")
	c.Check(info.RootDisk.Attributes, tc.DeepEquals, map[string]any{"iops": "3000"})
}

// TestProvisioningInfoCloudInitUserData verifies that cloud-init user data
// from the model config is passed through to provisioning info.
func (s *provisionerMockSuite) TestProvisioningInfoCloudInitUserData(c *tc.C) {
	defer s.setup(c).Finish()

	cloudInitData := map[string]any{
		"packages":        []any{"python3-pip", "curl"},
		"package_upgrade": true,
	}

	s.provisioningService.EXPECT().GetProvisioningInfo(gomock.Any(), coremachine.Name("0"), false, gomock.Any()).
		Return(domainprovisioner.ProvisioningInfo{
			Base:              corebase.MakeDefaultBase("ubuntu", "22.04"),
			Jobs:              []coremodel.MachineJob{coremodel.JobHostUnits},
			CloudInitUserData: cloudInitData,
			ControllerConfig:  map[string]any{"controller-uuid": "ctrl-uuid"},
		}, nil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
	}}
	result, err := s.api.ProvisioningInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results[0].Error, tc.IsNil)
	c.Assert(result.Results[0].Result, tc.Not(tc.IsNil))

	c.Check(result.Results[0].Result.CloudInitUserData, tc.DeepEquals, cloudInitData)
}

// TestProvisioningInfoWithEndpointBindings verifies that endpoint bindings
// are correctly mapped to the API result.
func (s *provisionerMockSuite) TestProvisioningInfoWithEndpointBindings(c *tc.C) {
	defer s.setup(c).Finish()

	s.provisioningService.EXPECT().GetProvisioningInfo(gomock.Any(), coremachine.Name("0"), false, gomock.Any()).
		Return(domainprovisioner.ProvisioningInfo{
			Base: corebase.MakeDefaultBase("ubuntu", "22.04"),
			Jobs: []coremodel.MachineJob{coremodel.JobHostUnits},
			EndpointBindings: map[string]string{
				"":    "provider-space-1",
				"web": "provider-space-1",
			},
			SpaceSubnets: map[string][]string{
				"myspace": {"subnet-0"},
			},
			SubnetAZs: map[string][]string{
				"subnet-0": {"zone-a"},
			},
			ControllerConfig: map[string]any{"controller-uuid": "ctrl-uuid"},
		}, nil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
	}}
	result, err := s.api.ProvisioningInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results[0].Error, tc.IsNil)
	c.Assert(result.Results[0].Result, tc.Not(tc.IsNil))

	info := result.Results[0].Result
	c.Check(info.EndpointBindings, tc.DeepEquals, map[string]string{
		"":    "provider-space-1",
		"web": "provider-space-1",
	})
}

// TestProvisioningInfoWithNetworkTopology verifies that network topology
// data (space subnets and subnet AZs) is correctly mapped.
func (s *provisionerMockSuite) TestProvisioningInfoWithNetworkTopology(c *tc.C) {
	defer s.setup(c).Finish()

	s.provisioningService.EXPECT().GetProvisioningInfo(gomock.Any(), coremachine.Name("0"), false, gomock.Any()).
		Return(domainprovisioner.ProvisioningInfo{
			Base: corebase.MakeDefaultBase("ubuntu", "22.04"),
			Jobs: []coremodel.MachineJob{coremodel.JobHostUnits},
			SpaceSubnets: map[string][]string{
				"space1": {"subnet-0"},
				"space2": {"subnet-1"},
			},
			SubnetAZs: map[string][]string{
				"subnet-0": {"zone-a"},
				"subnet-1": {"zone-b"},
			},
			ControllerConfig: map[string]any{"controller-uuid": "ctrl-uuid"},
		}, nil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
	}}
	result, err := s.api.ProvisioningInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results[0].Error, tc.IsNil)
	c.Assert(result.Results[0].Result, tc.Not(tc.IsNil))

	topo := result.Results[0].Result.ProvisioningNetworkTopology
	c.Check(topo.SubnetAZs, tc.DeepEquals, map[string][]string{
		"subnet-0": {"zone-a"},
		"subnet-1": {"zone-b"},
	})
	c.Check(topo.SpaceSubnets["space1"], tc.DeepEquals, []string{"subnet-0"})
	c.Check(topo.SpaceSubnets["space2"], tc.DeepEquals, []string{"subnet-1"})
}

// TestProvisioningInfoServiceError verifies that an error from the
// provisioning service is mapped to the per-machine result error.
func (s *provisionerMockSuite) TestProvisioningInfoServiceError(c *tc.C) {
	defer s.setup(c).Finish()

	s.provisioningService.EXPECT().GetProvisioningInfo(gomock.Any(), coremachine.Name("99"), false, gomock.Any()).
		Return(domainprovisioner.ProvisioningInfo{}, machineerrors.MachineNotFound)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-99"},
	}}
	result, err := s.api.ProvisioningInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.Not(tc.IsNil))
	c.Check(result.Results[0].Result, tc.IsNil)
}

// TestProvisioningInfoWithPlacement verifies that the placement directive
// from the machine is included in provisioning info.
func (s *provisionerMockSuite) TestProvisioningInfoWithPlacement(c *tc.C) {
	defer s.setup(c).Finish()

	placement := "zone=us-east-1a"
	s.provisioningService.EXPECT().GetProvisioningInfo(gomock.Any(), coremachine.Name("0"), false, gomock.Any()).
		Return(domainprovisioner.ProvisioningInfo{
			Base:               corebase.MakeDefaultBase("ubuntu", "22.04"),
			Jobs:               []coremodel.MachineJob{coremodel.JobHostUnits},
			PlacementDirective: &placement,
			ControllerConfig:   map[string]any{"controller-uuid": "ctrl-uuid"},
		}, nil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
	}}
	result, err := s.api.ProvisioningInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results[0].Error, tc.IsNil)
	c.Assert(result.Results[0].Result, tc.Not(tc.IsNil))
	c.Check(result.Results[0].Result.Placement, tc.Equals, placement)
}

// TestProvisioningInfoWithConstraints verifies that constraints from the
// machine are included in provisioning info.
func (s *provisionerMockSuite) TestProvisioningInfoWithConstraints(c *tc.C) {
	defer s.setup(c).Finish()

	cons := constraints.MustParse("cores=4 mem=8G")
	s.provisioningService.EXPECT().GetProvisioningInfo(gomock.Any(), coremachine.Name("0"), false, gomock.Any()).
		Return(domainprovisioner.ProvisioningInfo{
			Base:             corebase.MakeDefaultBase("ubuntu", "22.04"),
			Jobs:             []coremodel.MachineJob{coremodel.JobHostUnits},
			Constraints:      cons,
			ControllerConfig: map[string]any{"controller-uuid": "ctrl-uuid"},
		}, nil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
	}}
	result, err := s.api.ProvisioningInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results[0].Error, tc.IsNil)
	c.Assert(result.Results[0].Result, tc.Not(tc.IsNil))
	c.Check(result.Results[0].Result.Constraints, tc.DeepEquals, cons)
}

// TestProvisioningInfoPermissionsMultipleMachines verifies that when a machine
// agent authenticates, it can only access its own machine and its containers.
// Other machines and non-machine tags are denied.
func (s *provisionerMockSuite) TestProvisioningInfoPermissionsMultipleMachines(c *tc.C) {
	defer s.setup(c).Finish()

	// Only machine-0 and its containers are accessible.
	s.api.getAuthFunc = func(context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			machineTag, ok := tag.(names.MachineTag)
			if !ok {
				return false
			}
			return machineTag.Id() == "0" || machineTag.Id() == "0/lxd/0"
		}, nil
	}

	s.provisioningService.EXPECT().GetProvisioningInfo(gomock.Any(), coremachine.Name("0"), false, gomock.Any()).
		Return(domainprovisioner.ProvisioningInfo{
			Base:             corebase.MakeDefaultBase("ubuntu", "22.04"),
			Jobs:             []coremodel.MachineJob{coremodel.JobHostUnits},
			ControllerConfig: map[string]any{"controller-uuid": "ctrl-uuid"},
		}, nil)

	s.provisioningService.EXPECT().GetProvisioningInfo(gomock.Any(), coremachine.Name("0/lxd/0"), false, gomock.Any()).
		Return(domainprovisioner.ProvisioningInfo{}, machineerrors.MachineNotFound)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},       // allowed
		{Tag: "machine-0-lxd-0"}, // allowed (container)
		{Tag: "machine-42"},      // denied
		{Tag: "machine-1"},       // denied
	}}
	result, err := s.api.ProvisioningInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 4)

	// Machine-0: success.
	c.Check(result.Results[0].Error, tc.IsNil)
	c.Check(result.Results[0].Result, tc.Not(tc.IsNil))

	// Machine-0/lxd/0: not found (container doesn't exist but auth passes).
	c.Check(result.Results[1].Error, tc.Not(tc.IsNil))

	// Machine-42: permission denied.
	c.Check(result.Results[2].Error, tc.Not(tc.IsNil))
	c.Check(result.Results[2].Error.Message, tc.Equals, "permission denied")

	// Machine-1: permission denied.
	c.Check(result.Results[3].Error, tc.Not(tc.IsNil))
	c.Check(result.Results[3].Error.Message, tc.Equals, "permission denied")
}

// TestProvisioningInfoInvalidTag verifies that a non-machine tag returns
// a permission error.
func (s *provisionerMockSuite) TestProvisioningInfoInvalidTag(c *tc.C) {
	defer s.setup(c).Finish()

	args := params.Entities{Entities: []params.Entity{
		{Tag: "application-foo"},
	}}
	result, err := s.api.ProvisioningInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.Not(tc.IsNil))
	c.Check(result.Results[0].Error.Message, tc.Equals, "permission denied")
}
