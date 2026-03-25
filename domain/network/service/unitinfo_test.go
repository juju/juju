// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/collections/transform"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	corenetwork "github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	domainnetwork "github.com/juju/juju/domain/network"
	networkinternal "github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type infoSuite struct {
	testhelpers.IsolationSuite

	st                         *MockState
	providerWithNetworking     *MockProviderWithNetworking
	networkProviderGetter      func(context.Context) (ProviderWithNetworking, error)
	notSupportedProviderGetter func(context.Context) (ProviderWithNetworking, error)
	genericErrorProviderGetter func(context.Context) (ProviderWithNetworking, error)
}

func TestInfoSuite(t *testing.T) {
	tc.Run(t, &infoSuite{})
}

func (s *infoSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.st = NewMockState(ctrl)
	s.providerWithNetworking = NewMockProviderWithNetworking(ctrl)
	s.networkProviderGetter = func(context.Context) (ProviderWithNetworking, error) {
		return s.providerWithNetworking, nil
	}
	s.notSupportedProviderGetter = func(context.Context) (ProviderWithNetworking, error) {
		return nil, errors.Errorf("provider %w", coreerrors.NotSupported)
	}
	s.genericErrorProviderGetter = func(context.Context) (ProviderWithNetworking, error) {
		return nil, errors.New("boom")
	}
	return ctrl
}

func (s *infoSuite) TestGetUnitEndpointNetworks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")
	endpointNames := []string{"db", "server"}

	expectedInfos := []domainnetwork.UnitNetwork{
		{
			EndpointName: "db",
			DeviceInfos: []domainnetwork.DeviceInfo{
				{
					Name:       "eth0",
					MACAddress: "aa:bb:cc:dd:ee:ff",
					Addresses: []domainnetwork.AddressInfo{
						{
							Hostname: "192.168.1.10",
							Value:    "192.168.1.10",
							CIDR:     "192.168.1.0/24",
						},
					},
				},
			},
			IngressAddresses: []string{"192.168.1.10"},
			EgressSubnets:    []string{"192.168.1.0/24"},
		},
		{
			EndpointName: "server",
			DeviceInfos: []domainnetwork.DeviceInfo{
				{
					Name:       "eth1",
					MACAddress: "ff:ee:dd:cc:bb:aa",
					Addresses: []domainnetwork.AddressInfo{
						{
							Hostname: "10.0.0.10",
							Value:    "10.0.0.10",
							CIDR:     "10.0.0.0/24",
						},
					},
				},
			},
			IngressAddresses: []string{"10.0.0.10"},
			EgressSubnets:    []string{"192.168.1.0/24"},
		},
	}
	stateAddresses := []networkinternal.EndpointAddresses{
		{
			EndpointName: "db",
			Addresses: []networkinternal.UnitAddress{
				unitAddress("192.168.1.10", "192.168.1.0/24", "eth0",
					"aa:bb:cc:dd:ee:ff", corenetwork.ScopeCloudLocal,
					corenetwork.EthernetDevice),
			},
		},
		{
			EndpointName: "server",
			Addresses: []networkinternal.UnitAddress{
				unitAddress("10.0.0.10", "10.0.0.0/24", "eth1",
					"ff:ee:dd:cc:bb:aa", corenetwork.ScopeCloudLocal,
					corenetwork.EthernetDevice),
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().IsCaasUnit(gomock.Any(), unitUUID.String()).Return(false, nil)
	s.st.EXPECT().GetUnitEgressSubnets(gomock.Any(), unitUUID.String()).Return([]string{"192.168.1.0/24"}, nil)
	s.st.EXPECT().GetUnitEndpointNetworkAddresses(
		gomock.Any(), unitUUID.String(), endpointNames,
	).Return(stateAddresses, nil)

	service := NewProviderService(s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c))
	infos, err := service.GetUnitEndpointNetworks(c.Context(), unitName, endpointNames)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.DeepEquals, expectedInfos)
}

func (s *infoSuite) TestGetUnitEndpointNetworksUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return("", errors.New("unit not found"))

	service := NewProviderService(s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c))
	_, err := service.GetUnitEndpointNetworks(c.Context(), unitName, []string{"db"})
	c.Assert(err, tc.ErrorMatches, "unit not found")
}

func (s *infoSuite) TestGetUnitEndpointNetworksStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")
	endpointNames := []string{"db"}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().IsCaasUnit(gomock.Any(), unitUUID.String()).Return(true, nil)
	s.st.EXPECT().GetUnitEgressSubnets(gomock.Any(), unitUUID.String()).Return([]string{"10.0.0.0/24"}, nil)
	s.st.EXPECT().GetUnitEndpointNetworkAddresses(
		gomock.Any(), unitUUID.String(), endpointNames,
	).Return(nil, errors.New("state error"))

	service := NewProviderService(s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c))
	_, err := service.GetUnitEndpointNetworks(c.Context(), unitName, endpointNames)
	c.Assert(err, tc.ErrorMatches, "getting unit endpoint addresses: state error")
}

func (s *infoSuite) TestGetUnitEndpointNetworksIsCaasUnitError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().IsCaasUnit(gomock.Any(), unitUUID.String()).Return(false, errors.New("boom"))

	service := NewProviderService(s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c))
	_, err := service.GetUnitEndpointNetworks(c.Context(), unitName, []string{"db"})
	c.Assert(err, tc.ErrorMatches, "checking if unit is caas: boom")
}

func (s *infoSuite) TestGetUnitEndpointNetworksGetUnitEgressSubnetsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")
	endpointNames := []string{"db"}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().IsCaasUnit(gomock.Any(), unitUUID.String()).Return(false, nil)
	s.st.EXPECT().GetUnitEgressSubnets(gomock.Any(), unitUUID.String()).Return(nil, errors.New("boom"))

	service := NewProviderService(s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c))
	_, err := service.GetUnitEndpointNetworks(c.Context(), unitName, endpointNames)
	c.Assert(err, tc.ErrorMatches, "getting unit egress subnets: boom")
}

func (s *infoSuite) TestGetUnitEndpointNetworksExcludesLoopbackAndVethFromIngress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")
	endpointNames := []string{"db"}
	stateAddresses := []networkinternal.EndpointAddresses{
		{
			EndpointName: "db",
			Addresses: []networkinternal.UnitAddress{
				unitAddress("10.0.0.1", "10.0.0.0/24", "eth0",
					"aa:bb:cc:dd:ee:ff", corenetwork.ScopeCloudLocal,
					corenetwork.EthernetDevice),
				unitAddress("10.0.0.2", "10.0.0.0/24", "veth0",
					"ff:ee:dd:cc:bb:aa", corenetwork.ScopeCloudLocal,
					corenetwork.VirtualEthernetDevice),
				unitAddress("127.0.0.1", "127.0.0.0/8", "lo",
					"00:00:00:00:00:00", corenetwork.ScopeMachineLocal,
					corenetwork.LoopbackDevice),
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().IsCaasUnit(gomock.Any(), unitUUID.String()).Return(false, nil)
	s.st.EXPECT().GetUnitEgressSubnets(gomock.Any(), unitUUID.String()).Return([]string{"192.168.1.0/24"}, nil)
	s.st.EXPECT().GetUnitEndpointNetworkAddresses(
		gomock.Any(), unitUUID.String(), endpointNames,
	).Return(stateAddresses, nil)

	service := NewProviderService(s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c))
	infos, err := service.GetUnitEndpointNetworks(c.Context(), unitName, endpointNames)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.HasLen, 1)
	c.Check(infos[0].EndpointName, tc.Equals, "db")
	c.Check(infos[0].IngressAddresses, tc.DeepEquals, []string{"10.0.0.1"})
	c.Check(infos[0].EgressSubnets, tc.DeepEquals, []string{"192.168.1.0/24"})

	devices := transform.SliceToMap(
		infos[0].DeviceInfos,
		func(d domainnetwork.DeviceInfo) (string, domainnetwork.DeviceInfo) {
			return d.Name, d
		},
	)
	c.Assert(devices, tc.HasLen, 2)
	c.Check(devices["eth0"].Addresses, tc.DeepEquals, []domainnetwork.AddressInfo{{
		Hostname: "10.0.0.1",
		Value:    "10.0.0.1",
		CIDR:     "10.0.0.0/24",
	}})
	c.Check(devices["veth0"].Addresses, tc.DeepEquals, []domainnetwork.AddressInfo{{
		Hostname: "10.0.0.2",
		Value:    "10.0.0.2",
		CIDR:     "10.0.0.0/24",
	}})
}

func (s *infoSuite) TestGetUnitEndpointNetworksCaasUsesServiceAddressForIngress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")
	endpointNames := []string{"db"}
	stateAddresses := []networkinternal.EndpointAddresses{
		{
			EndpointName: "db",
			Addresses: []networkinternal.UnitAddress{
				unitAddress("10.0.0.1", "10.0.0.0/24", "eth0",
					"aa:bb:cc:dd:ee:ff", corenetwork.ScopeMachineLocal,
					corenetwork.EthernetDevice),
				unitAddress("10.0.0.2", "10.0.0.0/24", "eth1",
					"ff:ee:dd:cc:bb:aa", corenetwork.ScopeCloudLocal,
					corenetwork.EthernetDevice),
			},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().IsCaasUnit(gomock.Any(), unitUUID.String()).Return(true, nil)
	s.st.EXPECT().GetUnitEgressSubnets(gomock.Any(), unitUUID.String()).Return([]string{"10.0.0.0/24"}, nil)
	s.st.EXPECT().GetUnitEndpointNetworkAddresses(
		gomock.Any(), unitUUID.String(), endpointNames,
	).Return(stateAddresses, nil)

	service := NewProviderService(s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c))
	infos, err := service.GetUnitEndpointNetworks(c.Context(), unitName, endpointNames)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.HasLen, 1)
	c.Check(infos[0].IngressAddresses, tc.DeepEquals, []string{"10.0.0.2"})

	devices := transform.SliceToMap(
		infos[0].DeviceInfos,
		func(d domainnetwork.DeviceInfo) (string, domainnetwork.DeviceInfo) {
			return d.Name, d
		},
	)
	c.Assert(devices, tc.HasLen, 2)
	c.Check(devices["eth0"].Addresses, tc.DeepEquals, []domainnetwork.AddressInfo{{
		Hostname: "10.0.0.1",
		Value:    "10.0.0.1",
		CIDR:     "10.0.0.0/24",
	}})
	c.Check(devices["eth1"].Addresses, tc.HasLen, 0)
}

func (s *infoSuite) TestGetUnitEndpointNetworksNotSupportedUsesUnitAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")
	endpointNames := []string{"db", "server"}
	addresses := []networkinternal.UnitAddress{
		unitAddress("192.168.1.10", "192.168.1.0/24", "eth0",
			"aa:bb:cc:dd:ee:ff", corenetwork.ScopeCloudLocal,
			corenetwork.EthernetDevice),
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().IsCaasUnit(gomock.Any(), unitUUID.String()).Return(false, nil)
	s.st.EXPECT().GetUnitEgressSubnets(gomock.Any(), unitUUID.String()).Return([]string{"192.168.1.0/24"}, nil)
	s.st.EXPECT().GetUnitNetworkAddresses(gomock.Any(), unitUUID.String()).Return(addresses, nil)

	service := NewProviderService(s.st, s.notSupportedProviderGetter, nil, loggertesting.WrapCheckLog(c))
	infos, err := service.GetUnitEndpointNetworks(c.Context(), unitName, endpointNames)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.DeepEquals, []domainnetwork.UnitNetwork{
		{
			EndpointName: "db",
			DeviceInfos: []domainnetwork.DeviceInfo{{
				Name:       "eth0",
				MACAddress: "aa:bb:cc:dd:ee:ff",
				Addresses: []domainnetwork.AddressInfo{{
					Hostname: "192.168.1.10",
					Value:    "192.168.1.10",
					CIDR:     "192.168.1.0/24",
				}},
			}},
			IngressAddresses: []string{"192.168.1.10"},
			EgressSubnets:    []string{"192.168.1.0/24"},
		},
		{
			EndpointName: "server",
			DeviceInfos: []domainnetwork.DeviceInfo{{
				Name:       "eth0",
				MACAddress: "aa:bb:cc:dd:ee:ff",
				Addresses: []domainnetwork.AddressInfo{{
					Hostname: "192.168.1.10",
					Value:    "192.168.1.10",
					CIDR:     "192.168.1.0/24",
				}},
			}},
			IngressAddresses: []string{"192.168.1.10"},
			EgressSubnets:    []string{"192.168.1.0/24"},
		},
	})
}

func (s *infoSuite) TestGetUnitEndpointNetworksSupportsNetworkingError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)

	service := NewProviderService(s.st, s.genericErrorProviderGetter, nil, loggertesting.WrapCheckLog(c))
	_, err := service.GetUnitEndpointNetworks(c.Context(), unitName, []string{"db"})
	c.Assert(err, tc.ErrorMatches, "checking provider networking support: boom")
}

func unitAddress(
	value string,
	cidr string,
	deviceName string,
	macAddress string,
	scope corenetwork.Scope,
	deviceType corenetwork.LinkLayerDeviceType,
) networkinternal.UnitAddress {
	return networkinternal.UnitAddress{
		SpaceAddress: corenetwork.SpaceAddress{
			MachineAddress: corenetwork.MachineAddress{
				Value: value,
				Type:  corenetwork.IPv4Address,
				Scope: scope,
				CIDR:  cidr,
			},
		},
		DeviceName: deviceName,
		MACAddress: macAddress,
		DeviceType: deviceType,
	}
}
