// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	domain "github.com/juju/juju/domain"
	"github.com/juju/juju/domain/port"
	domaintesting "github.com/juju/juju/domain/testing"
)

type serviceSuite struct {
	st  *MockState
	srv *Service
}

var _ = gc.Suite(&serviceSuite{})

const (
	unitUUID    coreunit.UUID      = "unit-uuid"
	machineUUID string             = "machine-uuid"
	appUUID     coreapplication.ID = "app-uuid"
)

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)
	s.st.EXPECT().RunAtomic(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(ctx domain.AtomicContext) error) error {
		return fn(domaintesting.NewAtomicContext(ctx))
	}).AnyTimes()

	s.srv = &Service{st: s.st}

	return ctrl
}

func (s *serviceSuite) TestGetUnitOpenedPorts(c *gc.C) {
	defer s.setupMocks(c).Finish()

	grp := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("443/tcp"),
		},
		"ep2": {
			network.MustParsePortRange("8000-9000/udp"),
		},
	}

	s.st.EXPECT().GetUnitOpenedPorts(gomock.Any(), unitUUID).Return(grp, nil)

	res, err := s.srv.GetUnitOpenedPorts(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, grp)
}

func (s *serviceSuite) TestGetMachineOpenedPorts(c *gc.C) {
	defer s.setupMocks(c).Finish()

	grp := map[coreunit.UUID]network.GroupedPortRanges{
		"unit-uuid-1": {
			"ep1": {
				network.MustParsePortRange("80/tcp"),
				network.MustParsePortRange("443/tcp"),
			},
			"ep2": {
				network.MustParsePortRange("8000-9000/udp"),
			},
		},
		"unit-uuid-2": {
			"ep3": {
				network.MustParsePortRange("8080/tcp"),
			},
		},
	}

	s.st.EXPECT().GetMachineOpenedPorts(gomock.Any(), machineUUID).Return(grp, nil)

	res, err := s.srv.GetMachineOpenedPorts(context.Background(), machineUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, grp)
}

func (s *serviceSuite) TestGetApplicationOpenedPorts(c *gc.C) {
	defer s.setupMocks(c).Finish()

	openedPorts := port.UnitEndpointPortRanges{
		{Endpoint: "ep1", UnitUUID: "unit-uuid-1", PortRange: network.MustParsePortRange("80/tcp")},
		{Endpoint: "ep1", UnitUUID: "unit-uuid-1", PortRange: network.MustParsePortRange("443/tcp")},
		{Endpoint: "ep2", UnitUUID: "unit-uuid-1", PortRange: network.MustParsePortRange("8000-9000/udp")},
		{Endpoint: "ep3", UnitUUID: "unit-uuid-2", PortRange: network.MustParsePortRange("8080/tcp")},
	}

	expected := map[coreunit.UUID]network.GroupedPortRanges{
		"unit-uuid-1": {
			"ep1": {
				network.MustParsePortRange("80/tcp"),
				network.MustParsePortRange("443/tcp"),
			},
			"ep2": {
				network.MustParsePortRange("8000-9000/udp"),
			},
		},
		"unit-uuid-2": {
			"ep3": {
				network.MustParsePortRange("8080/tcp"),
			},
		},
	}

	s.st.EXPECT().GetApplicationOpenedPorts(gomock.Any(), appUUID).Return(openedPorts, nil)

	res, err := s.srv.GetApplicationOpenedPorts(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, expected)
}

func (s *serviceSuite) TestGetApplicationOpenedPortsByEndpoint(c *gc.C) {
	defer s.setupMocks(c).Finish()

	openedPorts := port.UnitEndpointPortRanges{
		{Endpoint: "ep1", UnitUUID: "unit-uuid-1", PortRange: network.MustParsePortRange("80/tcp")},
		{Endpoint: "ep1", UnitUUID: "unit-uuid-1", PortRange: network.MustParsePortRange("443/tcp")},
		{Endpoint: "ep1", UnitUUID: "unit-uuid-2", PortRange: network.MustParsePortRange("8080/tcp")},
		{Endpoint: "ep2", UnitUUID: "unit-uuid-1", PortRange: network.MustParsePortRange("8000-8005/udp")},
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.DeepEquals, expected)
}

func (s *serviceSuite) TestGetApplicationOpenedPortsByEndpointOverlap(c *gc.C) {
	defer s.setupMocks(c).Finish()

	openedPorts := port.UnitEndpointPortRanges{
		{Endpoint: "ep1", UnitUUID: "unit-uuid-1", PortRange: network.MustParsePortRange("80-85/tcp")},
		{Endpoint: "ep1", UnitUUID: "unit-uuid-2", PortRange: network.MustParsePortRange("83-88/tcp")},
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.DeepEquals, expected)
}

func (s *serviceSuite) TestSetUnitPorts(c *gc.C) {
	defer s.setupMocks(c).Finish()

	openPorts := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("443/tcp"),
		},
		"ep2": {
			network.MustParsePortRange("8000-9000/udp"),
		},
	}

	s.st.EXPECT().SetUnitPorts(gomock.Any(), "unit-name", openPorts)

	err := s.srv.SetUnitPorts(context.Background(), "unit-name", openPorts)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPorts(c *gc.C) {
	defer s.setupMocks(c).Finish()

	openPorts := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("443/tcp"),
		},
		"ep2": {
			network.MustParsePortRange("8000-9000/udp"),
		},
	}

	closePorts := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("22/tcp"),
		},
	}

	s.st.EXPECT().GetColocatedOpenedPorts(gomock.Any(), unitUUID).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetEndpointOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID, WildcardEndpoint).Return([]network.PortRange{}, nil)
	s.st.EXPECT().UpdateUnitPorts(domaintesting.IsAtomicContextChecker, unitUUID, openPorts, closePorts).Return(nil)

	err := s.srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsNoChanges(c *gc.C) {
	err := s.srv.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{"ep1": {}}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsSameRangeAcrossEndpoints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	openPorts := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("443/tcp"),
		},
		"ep2": {
			network.MustParsePortRange("80/tcp"),
		},
		"ep3": {
			network.MustParsePortRange("80/tcp"),
		},
	}
	closePorts := network.GroupedPortRanges{}

	s.st.EXPECT().GetColocatedOpenedPorts(gomock.Any(), unitUUID).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetEndpointOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID, WildcardEndpoint).Return([]network.PortRange{}, nil)
	s.st.EXPECT().UpdateUnitPorts(domaintesting.IsAtomicContextChecker, unitUUID, openPorts, closePorts).Return(nil)

	err := s.srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsConflict(c *gc.C) {

	err := s.srv.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
		"ep2": {
			network.MustParsePortRange("150-250/tcp"),
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIs, port.ErrPortRangeConflict)

	err = s.srv.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}, network.GroupedPortRanges{
		"ep2": {
			network.MustParsePortRange("150-250/tcp"),
		},
	})
	c.Assert(err, jc.ErrorIs, port.ErrPortRangeConflict)

	err = s.srv.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
			network.MustParsePortRange("200/tcp"),
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIs, port.ErrPortRangeConflict)
}

func (s *serviceSuite) TestUpdateUnitPortsOpenPortConflictColocated(c *gc.C) {
	defer s.setupMocks(c).Finish()

	openPorts := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}

	s.st.EXPECT().GetColocatedOpenedPorts(gomock.Any(), unitUUID).Return([]network.PortRange{
		network.MustParsePortRange("150-250/tcp"),
	}, nil)

	err := s.srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, network.GroupedPortRanges{})

	c.Assert(err, jc.ErrorIs, port.ErrPortRangeConflict)
}

func (s *serviceSuite) TestUpdateUnitPortsClosePortConflictColocated(c *gc.C) {
	defer s.setupMocks(c).Finish()

	closePorts := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}

	s.st.EXPECT().GetColocatedOpenedPorts(gomock.Any(), unitUUID).Return([]network.PortRange{
		network.MustParsePortRange("150-250/tcp"),
	}, nil)

	err := s.srv.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{}, closePorts)

	c.Assert(err, jc.ErrorIs, port.ErrPortRangeConflict)
}

func (s *serviceSuite) TestUpdateUnitPortsOpenWildcard(c *gc.C) {
	defer s.setupMocks(c).Finish()

	openPorts := network.GroupedPortRanges{
		WildcardEndpoint: {
			network.MustParsePortRange("100-200/tcp"),
		},
	}
	closePorts := network.GroupedPortRanges{}

	s.st.EXPECT().GetColocatedOpenedPorts(gomock.Any(), unitUUID).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetEndpointOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID, WildcardEndpoint).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetEndpoints(gomock.Any(), unitUUID).Return([]string{WildcardEndpoint, "ep1", "ep2", "ep3"}, nil)
	s.st.EXPECT().UpdateUnitPorts(domaintesting.IsAtomicContextChecker, unitUUID, openPorts, network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
		"ep2": {
			network.MustParsePortRange("100-200/tcp"),
		},
		"ep3": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}).Return(nil)

	err := s.srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsOpenPortRangeOpenOnWildcard(c *gc.C) {
	defer s.setupMocks(c).Finish()

	openPorts := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}
	closePorts := network.GroupedPortRanges{}

	s.st.EXPECT().GetColocatedOpenedPorts(gomock.Any(), unitUUID).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetEndpointOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID, WildcardEndpoint).Return([]network.PortRange{
		network.MustParsePortRange("100-200/tcp"),
	}, nil)
	s.st.EXPECT().UpdateUnitPorts(domaintesting.IsAtomicContextChecker, unitUUID, openPorts, network.GroupedPortRanges{}).Return(nil)

	err := s.srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsCloseWildcard(c *gc.C) {
	defer s.setupMocks(c).Finish()

	openPorts := network.GroupedPortRanges{}
	closePorts := network.GroupedPortRanges{
		WildcardEndpoint: {
			network.MustParsePortRange("100-200/tcp"),
		},
	}

	s.st.EXPECT().GetColocatedOpenedPorts(gomock.Any(), unitUUID).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetEndpointOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID, WildcardEndpoint).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetEndpoints(gomock.Any(), unitUUID).Return([]string{WildcardEndpoint, "ep1", "ep2", "ep3"}, nil)
	s.st.EXPECT().UpdateUnitPorts(domaintesting.IsAtomicContextChecker, unitUUID, openPorts, network.GroupedPortRanges{
		WildcardEndpoint: {
			network.MustParsePortRange("100-200/tcp"),
		},
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
		"ep2": {
			network.MustParsePortRange("100-200/tcp"),
		},
		"ep3": {
			network.MustParsePortRange("100-200/tcp"),
		},
	})

	err := s.srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsClosePortRangeOpenOnWildcard(c *gc.C) {
	defer s.setupMocks(c).Finish()

	openPorts := network.GroupedPortRanges{}
	closePorts := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}

	s.st.EXPECT().GetColocatedOpenedPorts(gomock.Any(), unitUUID).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetEndpointOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID, WildcardEndpoint).Return([]network.PortRange{
		network.MustParsePortRange("100-200/tcp"),
	}, nil)
	s.st.EXPECT().GetEndpoints(gomock.Any(), unitUUID).Return([]string{WildcardEndpoint, "ep1", "ep2", "ep3"}, nil)
	s.st.EXPECT().UpdateUnitPorts(domaintesting.IsAtomicContextChecker, unitUUID, network.GroupedPortRanges{
		"ep2": {
			network.MustParsePortRange("100-200/tcp"),
		},
		"ep3": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}, network.GroupedPortRanges{
		WildcardEndpoint: {
			network.MustParsePortRange("100-200/tcp"),
		},
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}).Return(nil)

	err := s.srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
	c.Assert(err, jc.ErrorIsNil)
}
