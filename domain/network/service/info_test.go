// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreunit "github.com/juju/juju/core/unit"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type infoSuite struct {
	testhelpers.IsolationSuite

	st *MockState
}

func TestInfoSuite(t *testing.T) {
	tc.Run(t, &infoSuite{})
}

func (s *infoSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.st = NewMockState(ctrl)
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
							Hostname: "mysql-0",
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
							Hostname: "mysql-0-server",
							Value:    "10.0.0.10",
							CIDR:     "10.0.0.0/24",
						},
					},
				},
			},
			IngressAddresses: []string{"10.0.0.10"},
			EgressSubnets:    []string{"10.0.0.0/24"},
		},
	}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().GetUnitEndpointNetworks(gomock.Any(), unitUUID.String(), endpointNames).Return(expectedInfos, nil)

	service := NewService(s.st, loggertesting.WrapCheckLog(c))
	infos, err := service.GetUnitEndpointNetworks(c.Context(), unitName, endpointNames)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(infos, tc.DeepEquals, expectedInfos)
}

func (s *infoSuite) TestGetUnitEndpointNetworksUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return("", errors.New("unit not found"))

	service := NewService(s.st, loggertesting.WrapCheckLog(c))
	_, err := service.GetUnitEndpointNetworks(c.Context(), unitName, []string{"db"})
	c.Assert(err, tc.ErrorMatches, "unit not found")
}

func (s *infoSuite) TestGetUnitEndpointNetworksStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("mysql/0")
	unitUUID := coreunit.UUID("unit-uuid-123")
	endpointNames := []string{"db"}

	s.st.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.st.EXPECT().GetUnitEndpointNetworks(gomock.Any(), unitUUID.String(), endpointNames).Return(nil,
		errors.New("state error"))

	service := NewService(s.st, loggertesting.WrapCheckLog(c))
	_, err := service.GetUnitEndpointNetworks(c.Context(), unitName, endpointNames)
	c.Assert(err, tc.ErrorMatches, "getting unit endpoint networks: state error")
}
