// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/port"
	porterrors "github.com/juju/juju/domain/port/errors"
)

type serviceSuite struct {
	st  *MockState
	srv *Service
}

var _ = tc.Suite(&serviceSuite{})

const (
	unitUUID    coreunit.UUID      = "unit-uuid"
	machineUUID string             = "machine-uuid"
	appUUID     coreapplication.ID = "app-uuid"
)

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)
	s.srv = &Service{st: s.st}

	return ctrl
}

func (s *serviceSuite) TestGetUnitOpenedPorts(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetUnitOpenedPorts(gomock.Any(), unitUUID).Return(network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("443/tcp"),
		},
		"ep2": {
			network.MustParsePortRange("8000-9000/udp"),
		},
	}, nil)

	res, err := s.srv.GetUnitOpenedPorts(context.Background(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("443/tcp"),
		},
		"ep2": {
			network.MustParsePortRange("8000-9000/udp"),
		},
	})
}

func (s *serviceSuite) TestGetAllOpenedPorts(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetAllOpenedPorts(gomock.Any()).Return(port.UnitGroupedPortRanges{
		"unit/0": {
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("443/tcp"),
		},
		"unit/1": {
			network.MustParsePortRange("8000-9000/udp"),
		},
	}, nil)

	res, err := s.srv.GetAllOpenedPorts(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, port.UnitGroupedPortRanges{
		"unit/0": {
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("443/tcp"),
		},
		"unit/1": {
			network.MustParsePortRange("8000-9000/udp"),
		},
	})
}

func (s *serviceSuite) TestGetMachineOpenedPorts(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetMachineOpenedPorts(gomock.Any(), machineUUID).Return(map[coreunit.Name]network.GroupedPortRanges{
		"unit/1": {
			"ep1": {
				network.MustParsePortRange("80/tcp"),
				network.MustParsePortRange("443/tcp"),
			},
			"ep2": {
				network.MustParsePortRange("8000-9000/udp"),
			},
		},
		"unit/2": {
			"ep3": {
				network.MustParsePortRange("8080/tcp"),
			},
		},
	}, nil)

	res, err := s.srv.GetMachineOpenedPorts(context.Background(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, map[coreunit.Name]network.GroupedPortRanges{
		"unit/1": {
			"ep1": {
				network.MustParsePortRange("80/tcp"),
				network.MustParsePortRange("443/tcp"),
			},
			"ep2": {
				network.MustParsePortRange("8000-9000/udp"),
			},
		},
		"unit/2": {
			"ep3": {
				network.MustParsePortRange("8080/tcp"),
			},
		},
	})
}

func (s *serviceSuite) TestGetApplicationOpenedPorts(c *tc.C) {
	defer s.setupMocks(c).Finish()

	openedPorts := port.UnitEndpointPortRanges{
		{Endpoint: "ep1", UnitName: "unit/1", PortRange: network.MustParsePortRange("80/tcp")},
		{Endpoint: "ep1", UnitName: "unit/1", PortRange: network.MustParsePortRange("443/tcp")},
		{Endpoint: "ep2", UnitName: "unit/1", PortRange: network.MustParsePortRange("8000-9000/udp")},
		{Endpoint: "ep3", UnitName: "unit/2", PortRange: network.MustParsePortRange("8080/tcp")},
	}

	expected := map[coreunit.Name]network.GroupedPortRanges{
		"unit/1": {
			"ep1": {
				network.MustParsePortRange("80/tcp"),
				network.MustParsePortRange("443/tcp"),
			},
			"ep2": {
				network.MustParsePortRange("8000-9000/udp"),
			},
		},
		"unit/2": {
			"ep3": {
				network.MustParsePortRange("8080/tcp"),
			},
		},
	}

	s.st.EXPECT().GetApplicationOpenedPorts(gomock.Any(), appUUID).Return(openedPorts, nil)

	res, err := s.srv.GetApplicationOpenedPorts(context.Background(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, expected)
}

func (s *serviceSuite) TestGetApplicationOpenedPortsByEndpoint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	openedPorts := port.UnitEndpointPortRanges{
		{Endpoint: "ep1", UnitName: "unit/1", PortRange: network.MustParsePortRange("80/tcp")},
		{Endpoint: "ep1", UnitName: "unit/1", PortRange: network.MustParsePortRange("443/tcp")},
		{Endpoint: "ep1", UnitName: "unit/2", PortRange: network.MustParsePortRange("8080/tcp")},
		{Endpoint: "ep2", UnitName: "unit/1", PortRange: network.MustParsePortRange("8000-8005/udp")},
	}

	s.st.EXPECT().GetApplicationOpenedPorts(gomock.Any(), appUUID).Return(openedPorts, nil)

	expected := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("443/tcp"),
			network.MustParsePortRange("8080/tcp"),
		},
		"ep2": {
			network.MustParsePortRange("8000/udp"),
			network.MustParsePortRange("8001/udp"),
			network.MustParsePortRange("8002/udp"),
			network.MustParsePortRange("8003/udp"),
			network.MustParsePortRange("8004/udp"),
			network.MustParsePortRange("8005/udp"),
		},
	}

	res, err := s.srv.GetApplicationOpenedPortsByEndpoint(context.Background(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, expected)
}

func (s *serviceSuite) TestGetApplicationOpenedPortsByEndpointOverlap(c *tc.C) {
	defer s.setupMocks(c).Finish()

	openedPorts := port.UnitEndpointPortRanges{
		{Endpoint: "ep1", UnitName: "unit-uuid-1", PortRange: network.MustParsePortRange("80-85/tcp")},
		{Endpoint: "ep1", UnitName: "unit-uuid-2", PortRange: network.MustParsePortRange("83-88/tcp")},
	}

	s.st.EXPECT().GetApplicationOpenedPorts(gomock.Any(), appUUID).Return(openedPorts, nil)

	expected := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("81/tcp"),
			network.MustParsePortRange("82/tcp"),
			network.MustParsePortRange("83/tcp"),
			network.MustParsePortRange("84/tcp"),
			network.MustParsePortRange("85/tcp"),
			network.MustParsePortRange("86/tcp"),
			network.MustParsePortRange("87/tcp"),
			network.MustParsePortRange("88/tcp"),
		},
	}

	res, err := s.srv.GetApplicationOpenedPortsByEndpoint(context.Background(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, expected)
}

func (s *serviceSuite) TestUpdateUnitPorts(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().UpdateUnitPorts(
		gomock.Any(), unitUUID,
		network.GroupedPortRanges{
			"ep1": {network.MustParsePortRange("80/tcp"), network.MustParsePortRange("443/tcp")},
			"ep2": {network.MustParsePortRange("8000-9000/tcp")},
		},
		network.GroupedPortRanges{
			"ep1": {network.MustParsePortRange("22/tcp")},
		},
	).Return(nil)

	err := s.srv.UpdateUnitPorts(
		context.Background(), unitUUID,
		network.GroupedPortRanges{
			"ep1": {network.MustParsePortRange("80/tcp"), network.MustParsePortRange("443/tcp")},
			"ep2": {network.MustParsePortRange("8000-9000/tcp")},
		},
		network.GroupedPortRanges{
			"ep1": {network.MustParsePortRange("22/tcp")},
		},
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsNoChanges(c *tc.C) {
	err := s.srv.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{"ep1": {}}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsNilOpenPorts(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().UpdateUnitPorts(
		gomock.Any(), unitUUID, nil,
		network.GroupedPortRanges{
			"ep1": {network.MustParsePortRange("22/tcp")},
		},
	).Return(nil)

	err := s.srv.UpdateUnitPorts(
		context.Background(), unitUUID, nil,
		network.GroupedPortRanges{
			"ep1": {network.MustParsePortRange("22/tcp")},
		},
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsNilClosePorts(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().UpdateUnitPorts(
		gomock.Any(), unitUUID,
		network.GroupedPortRanges{
			"ep1": {network.MustParsePortRange("80/tcp"), network.MustParsePortRange("443/tcp")},
		},
		nil,
	).Return(nil)

	err := s.srv.UpdateUnitPorts(
		context.Background(), unitUUID,
		network.GroupedPortRanges{
			"ep1": {network.MustParsePortRange("80/tcp"), network.MustParsePortRange("443/tcp")},
		},
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsNilPortMaps(c *tc.C) {
	err := s.srv.UpdateUnitPorts(context.Background(), unitUUID, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsConflict(c *tc.C) {
	err := s.srv.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
		"ep2": {
			network.MustParsePortRange("150-250/tcp"),
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIs, porterrors.PortRangeConflict)

	err = s.srv.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}, network.GroupedPortRanges{
		"ep2": {
			network.MustParsePortRange("150-250/tcp"),
		},
	})
	c.Assert(err, tc.ErrorIs, porterrors.PortRangeConflict)

	err = s.srv.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
			network.MustParsePortRange("200/tcp"),
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIs, porterrors.PortRangeConflict)
}

func (s *serviceSuite) TestGetUnitUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("unit/0")
	s.st.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)

	res, err := s.srv.GetUnitUUID(context.Background(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.Equals, unitUUID)
}
