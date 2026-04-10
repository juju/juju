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
	corerelation "github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	domainnetwork "github.com/juju/juju/domain/network"
	networkinternal "github.com/juju/juju/domain/network/internal"
	relationerrors "github.com/juju/juju/domain/relation/errors"
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
	stateNetworkInfo := []networkinternal.EndpointNetworkInfo{
		endpointNetworkInfo("db", []string{"192.168.1.10/24"},
			unitAddress("192.168.1.10", "192.168.1.0/24", "eth0",
				"aa:bb:cc:dd:ee:ff", corenetwork.ScopeCloudLocal,
				corenetwork.EthernetDevice),
		),
		endpointNetworkInfo("server", []string{"10.0.0.10/24"},
			unitAddress("10.0.0.10", "10.0.0.0/24", "eth1",
				"ff:ee:dd:cc:bb:aa", corenetwork.ScopeCloudLocal,
				corenetwork.EthernetDevice),
		),
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().IsCaasUnit(gomock.Any(), unitUUID.String()).Return(false, nil)
	s.st.EXPECT().GetUnitEgressSubnets(gomock.Any(), unitUUID.String()).Return([]string{"192.168.1.0/24"}, nil)
	s.st.EXPECT().GetUnitEndpointNetworkInfo(
		gomock.Any(), unitUUID.String(), endpointNames,
	).Return(stateNetworkInfo, nil)

	service := NewProviderService(s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c))
	infos, err := service.GetUnitEndpointNetworks(c.Context(), unitName, endpointNames)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.DeepEquals, expectedInfos)
}

func (s *infoSuite) TestGetUnitEndpointNetworksSortsIngressAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")
	endpointNames := []string{"db"}

	stateNetworkInfo := []networkinternal.EndpointNetworkInfo{
		endpointNetworkInfo("db", []string{"10.0.0.9", "10.0.1.9"},
			unitAddress("10.0.1.9", "10.0.1.0/24", "eth0",
				"aa:bb:cc:dd:ee:f0", corenetwork.ScopeCloudLocal,
				corenetwork.EthernetDevice),
			unitAddress("10.0.0.9", "10.0.0.0/24", "eth1",
				"aa:bb:cc:dd:ee:f1", corenetwork.ScopeCloudLocal,
				corenetwork.EthernetDevice),
			unitAddress("10.0.0.2", "10.0.0.0/24", "veth0",
				"ff:ee:dd:cc:bb:aa", corenetwork.ScopeCloudLocal,
				corenetwork.VirtualEthernetDevice),
			unitAddress("127.0.0.1", "127.0.0.0/8", "lo",
				"00:00:00:00:00:00", corenetwork.ScopeMachineLocal,
				corenetwork.LoopbackDevice),
		),
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().IsCaasUnit(gomock.Any(), unitUUID.String()).Return(false, nil)
	s.st.EXPECT().GetUnitEgressSubnets(gomock.Any(), unitUUID.String()).Return([]string{"10.0.0.0/24"}, nil)
	s.st.EXPECT().GetUnitEndpointNetworkInfo(
		gomock.Any(), unitUUID.String(), endpointNames,
	).Return(stateNetworkInfo, nil)

	service := NewProviderService(s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c))
	infos, err := service.GetUnitEndpointNetworks(c.Context(), unitName, endpointNames)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.HasLen, 1)
	c.Assert(infos[0].IngressAddresses, tc.DeepEquals, []string{"10.0.0.9", "10.0.1.9"})
	c.Assert(infos[0].EgressSubnets, tc.DeepEquals, []string{"10.0.0.0/24"})
}

func (s *infoSuite) TestGetUnitEndpointNetworksFallsBackToModelEgressSubnets(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")
	endpointNames := []string{"db"}
	stateNetworkInfo := []networkinternal.EndpointNetworkInfo{
		endpointNetworkInfo("db", []string{"192.168.1.10"},
			unitAddress("192.168.1.10", "192.168.1.0/24", "eth0",
				"aa:bb:cc:dd:ee:ff", corenetwork.ScopeCloudLocal,
				corenetwork.EthernetDevice),
		),
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().GetUnitEgressSubnets(gomock.Any(), unitUUID.String()).Return(nil, nil)
	s.st.EXPECT().GetModelEgressSubnets(gomock.Any()).Return([]string{"203.0.113.0/24"}, nil)
	s.st.EXPECT().IsCaasUnit(gomock.Any(), unitUUID.String()).Return(false, nil)
	s.st.EXPECT().GetUnitEndpointNetworkInfo(
		gomock.Any(), unitUUID.String(), endpointNames,
	).Return(stateNetworkInfo, nil)

	service := NewProviderService(
		s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c),
	)
	infos, err := service.GetUnitEndpointNetworks(c.Context(), unitName, endpointNames)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.HasLen, 1)
	c.Check(infos[0].EgressSubnets, tc.DeepEquals, []string{"203.0.113.0/24"})
}

func (s *infoSuite) TestGetUnitEndpointNetworksFallsBackToPublicEgressSubnets(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")
	endpointNames := []string{"db"}
	stateNetworkInfo := []networkinternal.EndpointNetworkInfo{
		endpointNetworkInfo("db", []string{"192.168.1.10"},
			unitAddress("192.168.1.10", "192.168.1.0/24", "eth0",
				"aa:bb:cc:dd:ee:ff", corenetwork.ScopeCloudLocal,
				corenetwork.EthernetDevice),
		),
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().GetUnitEgressSubnets(gomock.Any(), unitUUID.String()).Return(nil, nil)
	s.st.EXPECT().GetModelEgressSubnets(gomock.Any()).Return([]string{}, nil)
	s.st.EXPECT().GetUnitPublicAddressForEgress(
		gomock.Any(), unitUUID.String(),
	).Return("198.51.100.10/24", nil)
	s.st.EXPECT().IsCaasUnit(gomock.Any(), unitUUID.String()).Return(false, nil)
	s.st.EXPECT().GetUnitEndpointNetworkInfo(
		gomock.Any(), unitUUID.String(), endpointNames,
	).Return(stateNetworkInfo, nil)

	service := NewProviderService(
		s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c),
	)
	infos, err := service.GetUnitEndpointNetworks(c.Context(), unitName, endpointNames)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.HasLen, 1)
	c.Check(infos[0].EgressSubnets, tc.DeepEquals, []string{"198.51.100.10/32"})
}

func (s *infoSuite) TestGetUnitRelationNetwork(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")
	relationUUID := tc.Must(c, corerelation.NewUUID)
	endpointName := "db"
	stateNetworkInfo := []networkinternal.EndpointNetworkInfo{
		endpointNetworkInfo(endpointName, []string{"192.168.1.10"},
			unitAddress("192.168.1.10", "192.168.1.0/24", "eth0",
				"aa:bb:cc:dd:ee:ff", corenetwork.ScopeCloudLocal,
				corenetwork.EthernetDevice),
		),
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().GetUnitRelationEndpointName(
		gomock.Any(), unitUUID.String(), relationUUID.String(),
	).Return(endpointName, nil)
	s.st.EXPECT().GetRelationEgressSubnets(
		gomock.Any(), relationUUID.String(),
	).Return([]string{"192.168.1.0/24"}, nil)
	s.st.EXPECT().IsCaasUnit(gomock.Any(), unitUUID.String()).Return(false, nil)
	s.st.EXPECT().GetUnitEndpointNetworkInfo(
		gomock.Any(), unitUUID.String(), []string{endpointName},
	).Return(stateNetworkInfo, nil)

	service := NewProviderService(
		s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c),
	)
	infoMap, err := service.GetUnitRelationNetwork(
		c.Context(), unitName, []corerelation.UUID{relationUUID},
	)
	c.Assert(err, tc.ErrorIsNil)
	info, ok := infoMap[relationUUID]
	c.Assert(ok, tc.IsTrue)
	c.Check(info.EndpointName, tc.Equals, endpointName)
	c.Check(info.IngressAddresses, tc.DeepEquals, []string{"192.168.1.10"})
	c.Check(info.EgressSubnets, tc.DeepEquals, []string{"192.168.1.0/24"})
}

func (s *infoSuite) TestGetUnitRelationNetworkFallsBackToModelEgressSubnets(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")
	relationUUID := tc.Must(c, corerelation.NewUUID)
	endpointName := "db"
	stateNetworkInfo := []networkinternal.EndpointNetworkInfo{
		endpointNetworkInfo(endpointName, []string{"192.168.1.10"},
			unitAddress("192.168.1.10", "192.168.1.0/24", "eth0",
				"aa:bb:cc:dd:ee:ff", corenetwork.ScopeCloudLocal,
				corenetwork.EthernetDevice),
		),
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().GetUnitRelationEndpointName(
		gomock.Any(), unitUUID.String(), relationUUID.String(),
	).Return(endpointName, nil)
	s.st.EXPECT().GetRelationEgressSubnets(
		gomock.Any(), relationUUID.String(),
	).Return(nil, nil)
	s.st.EXPECT().GetModelEgressSubnets(gomock.Any()).Return([]string{"203.0.113.0/24"}, nil)
	s.st.EXPECT().IsCaasUnit(gomock.Any(), unitUUID.String()).Return(false, nil)
	s.st.EXPECT().GetUnitEndpointNetworkInfo(
		gomock.Any(), unitUUID.String(), []string{endpointName},
	).Return(stateNetworkInfo, nil)

	service := NewProviderService(
		s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c),
	)
	infoMap, err := service.GetUnitRelationNetwork(
		c.Context(), unitName, []corerelation.UUID{relationUUID},
	)
	c.Assert(err, tc.ErrorIsNil)
	info, ok := infoMap[relationUUID]
	c.Assert(ok, tc.IsTrue)
	c.Check(info.EgressSubnets, tc.DeepEquals, []string{"203.0.113.0/24"})
}

func (s *infoSuite) TestGetUnitRelationNetworkFallsBackToPublicEgressSubnets(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")
	relationUUID := tc.Must(c, corerelation.NewUUID)
	endpointName := "db"
	stateNetworkInfo := []networkinternal.EndpointNetworkInfo{
		endpointNetworkInfo(endpointName, []string{"192.168.1.10"},
			unitAddress("192.168.1.10", "192.168.1.0/24", "eth0",
				"aa:bb:cc:dd:ee:ff", corenetwork.ScopeCloudLocal,
				corenetwork.EthernetDevice),
		),
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().GetUnitRelationEndpointName(
		gomock.Any(), unitUUID.String(), relationUUID.String(),
	).Return(endpointName, nil)
	s.st.EXPECT().GetRelationEgressSubnets(
		gomock.Any(), relationUUID.String(),
	).Return(nil, nil)
	s.st.EXPECT().GetModelEgressSubnets(gomock.Any()).Return([]string{}, nil)
	s.st.EXPECT().GetUnitPublicAddressForEgress(
		gomock.Any(), unitUUID.String(),
	).Return("198.51.100.10/24", nil)
	s.st.EXPECT().IsCaasUnit(gomock.Any(), unitUUID.String()).Return(false, nil)
	s.st.EXPECT().GetUnitEndpointNetworkInfo(
		gomock.Any(), unitUUID.String(), []string{endpointName},
	).Return(stateNetworkInfo, nil)

	service := NewProviderService(
		s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c),
	)
	infoMap, err := service.GetUnitRelationNetwork(
		c.Context(), unitName, []corerelation.UUID{relationUUID},
	)
	c.Assert(err, tc.ErrorIsNil)
	info, ok := infoMap[relationUUID]
	c.Assert(ok, tc.IsTrue)
	c.Check(info.EgressSubnets, tc.DeepEquals, []string{"198.51.100.10/32"})
}

func (s *infoSuite) TestGetUnitRelationNetworkRelationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")
	relationUUID := tc.Must(c, corerelation.NewUUID)

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().GetUnitRelationEndpointName(
		gomock.Any(), unitUUID.String(), relationUUID.String(),
	).Return("", relationerrors.RelationNotFound)

	service := NewProviderService(
		s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c),
	)
	_, err := service.GetUnitRelationNetwork(c.Context(), unitName, []corerelation.UUID{relationUUID})
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *infoSuite) TestGetUnitRelationNetworkGetRelationEgressSubnetsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")
	relationUUID := tc.Must(c, corerelation.NewUUID)
	endpointName := "db"

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().GetUnitRelationEndpointName(
		gomock.Any(), unitUUID.String(), relationUUID.String(),
	).Return(endpointName, nil)
	s.st.EXPECT().GetRelationEgressSubnets(
		gomock.Any(), relationUUID.String(),
	).Return(nil, errors.New("boom"))

	service := NewProviderService(
		s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c),
	)
	_, err := service.GetUnitRelationNetwork(c.Context(), unitName, []corerelation.UUID{relationUUID})
	c.Assert(
		err,
		tc.ErrorMatches,
		`getting egress subnets for relation ".*": boom`,
	)
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
	s.st.EXPECT().GetUnitEndpointNetworkInfo(
		gomock.Any(), unitUUID.String(), endpointNames,
	).Return(nil, errors.New("state error"))

	service := NewProviderService(s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c))
	_, err := service.GetUnitEndpointNetworks(c.Context(), unitName, endpointNames)
	c.Assert(err, tc.ErrorMatches, "getting unit endpoint network info: state error")
}

func (s *infoSuite) TestGetUnitEndpointNetworksIsCaasUnitError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().GetUnitEgressSubnets(
		gomock.Any(), unitUUID.String(),
	).Return([]string{"10.0.0.0/24"}, nil)
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
	s.st.EXPECT().GetUnitEgressSubnets(gomock.Any(), unitUUID.String()).Return(nil, errors.New("boom"))

	service := NewProviderService(s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c))
	_, err := service.GetUnitEndpointNetworks(c.Context(), unitName, endpointNames)
	c.Assert(err, tc.ErrorMatches, "getting unit egress subnets: boom")
}

func (s *infoSuite) TestGetUnitEndpointNetworksDeprioritisesVethIngress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")
	endpointNames := []string{"db"}
	stateNetworkInfo := []networkinternal.EndpointNetworkInfo{
		endpointNetworkInfo("db", []string{"10.0.0.1", "10.0.0.2"},
			unitAddress("10.0.0.1", "10.0.0.0/24", "eth0",
				"aa:bb:cc:dd:ee:ff", corenetwork.ScopeCloudLocal,
				corenetwork.EthernetDevice),
			unitAddress("10.0.0.2", "10.0.0.0/24", "veth0",
				"ff:ee:dd:cc:bb:aa", corenetwork.ScopeCloudLocal,
				corenetwork.VirtualEthernetDevice),
			unitAddress("127.0.0.1", "127.0.0.0/8", "lo",
				"00:00:00:00:00:00", corenetwork.ScopeMachineLocal,
				corenetwork.LoopbackDevice),
		),
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().IsCaasUnit(gomock.Any(), unitUUID.String()).Return(false, nil)
	s.st.EXPECT().GetUnitEgressSubnets(gomock.Any(), unitUUID.String()).Return([]string{"192.168.1.0/24"}, nil)
	s.st.EXPECT().GetUnitEndpointNetworkInfo(
		gomock.Any(), unitUUID.String(), endpointNames,
	).Return(stateNetworkInfo, nil)

	service := NewProviderService(s.st, s.networkProviderGetter, nil, loggertesting.WrapCheckLog(c))
	infos, err := service.GetUnitEndpointNetworks(c.Context(), unitName, endpointNames)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.HasLen, 1)
	c.Check(infos[0].EndpointName, tc.Equals, "db")
	c.Check(infos[0].IngressAddresses, tc.DeepEquals, []string{"10.0.0.1", "10.0.0.2"})
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
	stateNetworkInfo := []networkinternal.EndpointNetworkInfo{
		endpointNetworkInfo("db", []string{"10.0.0.2"},
			unitAddress("10.0.0.1", "10.0.0.0/24", "eth0",
				"aa:bb:cc:dd:ee:ff", corenetwork.ScopeMachineLocal,
				corenetwork.EthernetDevice),
			unitAddress("10.0.0.2", "10.0.0.0/24", "eth1",
				"ff:ee:dd:cc:bb:aa", corenetwork.ScopeCloudLocal,
				corenetwork.EthernetDevice),
		),
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().IsCaasUnit(gomock.Any(), unitUUID.String()).Return(true, nil)
	s.st.EXPECT().GetUnitEgressSubnets(gomock.Any(), unitUUID.String()).Return([]string{"10.0.0.0/24"}, nil)
	s.st.EXPECT().GetUnitEndpointNetworkInfo(
		gomock.Any(), unitUUID.String(), endpointNames,
	).Return(stateNetworkInfo, nil)

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
	s.st.EXPECT().GetUnitNetworkInfo(
		gomock.Any(), unitUUID.String(),
	).Return(unitNetworkInfo([]string{"192.168.1.10/24"}, addresses...), nil)

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

func (s *infoSuite) TestGetUnitEndpointNetworksNotSupportedSortsIngressAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")
	endpointNames := []string{"db"}
	unitAddresses := []networkinternal.UnitAddress{
		unitAddress("10.0.1.9", "10.0.1.0/24", "eth0",
			"aa:bb:cc:dd:ee:f0", corenetwork.ScopeCloudLocal,
			corenetwork.EthernetDevice),
		unitAddress("10.0.0.9", "10.0.0.0/24", "eth1",
			"aa:bb:cc:dd:ee:f1", corenetwork.ScopeCloudLocal,
			corenetwork.EthernetDevice),
		unitAddress("10.0.0.2", "10.0.0.0/24", "veth0",
			"ff:ee:dd:cc:bb:aa", corenetwork.ScopeCloudLocal,
			corenetwork.VirtualEthernetDevice),
		unitAddress("127.0.0.1", "127.0.0.0/8", "lo",
			"00:00:00:00:00:00", corenetwork.ScopeMachineLocal,
			corenetwork.LoopbackDevice),
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().IsCaasUnit(gomock.Any(), unitUUID.String()).Return(false, nil)
	s.st.EXPECT().GetUnitEgressSubnets(gomock.Any(), unitUUID.String()).Return([]string{"10.0.0.0/24"}, nil)
	s.st.EXPECT().GetUnitNetworkInfo(
		gomock.Any(), unitUUID.String(),
	).Return(unitNetworkInfo([]string{"10.0.0.9", "10.0.1.9"}, unitAddresses...), nil)

	service := NewProviderService(s.st, s.notSupportedProviderGetter, nil, loggertesting.WrapCheckLog(c))
	infos, err := service.GetUnitEndpointNetworks(c.Context(), unitName, endpointNames)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.HasLen, 1)
	c.Assert(infos[0].IngressAddresses, tc.DeepEquals, []string{"10.0.0.9", "10.0.1.9"})
	c.Assert(infos[0].EgressSubnets, tc.DeepEquals, []string{"10.0.0.0/24"})
}

func (s *infoSuite) TestGetUnitEndpointNetworksSupportsNetworkingError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().GetUnitEgressSubnets(
		gomock.Any(), unitUUID.String(),
	).Return([]string{"10.0.0.0/24"}, nil)

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

func endpointNetworkInfo(
	endpointName string,
	ingressAddresses []string,
	addresses ...networkinternal.UnitAddress,
) networkinternal.EndpointNetworkInfo {
	return networkinternal.EndpointNetworkInfo{
		EndpointName:     endpointName,
		Addresses:        addresses,
		IngressAddresses: ingressAddresses,
	}
}

func unitNetworkInfo(
	ingressAddresses []string,
	addresses ...networkinternal.UnitAddress,
) networkinternal.UnitNetworkInfo {
	return networkinternal.UnitNetworkInfo{
		Addresses:        addresses,
		IngressAddresses: ingressAddresses,
	}
}
