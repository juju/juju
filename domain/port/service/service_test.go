// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	domain "github.com/juju/juju/domain"
	"github.com/juju/juju/domain/port"
	domaintesting "github.com/juju/juju/domain/testing"
)

type serviceSuite struct {
	st *MockState
}

var _ = gc.Suite(&serviceSuite{})

const (
	unitUUID    = "unit-uuid"
	machineUUID = "machine-uuid"
	appUUID     = "app-uuid"
)

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)
	s.st.EXPECT().RunAtomic(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(ctx domain.AtomicContext) error) error {
		return fn(domaintesting.NewAtomicContext(ctx))
	}).AnyTimes()

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

	srv := NewService(s.st)
	res, err := srv.GetUnitOpenedPorts(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, grp)
}

func (s *serviceSuite) TestGetMachineOpenedPorts(c *gc.C) {
	defer s.setupMocks(c).Finish()

	grp := map[string]network.GroupedPortRanges{
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

	srv := NewService(s.st)
	res, err := srv.GetMachineOpenedPorts(context.Background(), machineUUID)
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

	expected := map[string]network.GroupedPortRanges{
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

	srv := NewService(s.st)
	res, err := srv.GetApplicationOpenedPorts(context.Background(), appUUID)
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

	srv := NewService(s.st)
	res, err := srv.GetApplicationOpenedPortsByEndpoint(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.DeepEquals, expected)
}

func (s serviceSuite) TestGetApplicationOpenedPortsByEndpointOverlap(c *gc.C) {
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

	srv := NewService(s.st)
	res, err := srv.GetApplicationOpenedPortsByEndpoint(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.DeepEquals, expected)
}

func (s *serviceSuite) TestUpdateUnitPorts(c *gc.C) {
	defer s.setupMocks(c).Finish()

	endpoints := []port.Endpoint{
		{UUID: "wildcard-uuid", Endpoint: WildcardEndpoint},
		{UUID: "ep1-uuid", Endpoint: "ep1"},
		{UUID: "ep2-uuid", Endpoint: "ep2"},
	}

	currentPorts := map[string][]port.PortRangeUUID{
		"ep1": {
			{UUID: "port-range-uuid-1", PortRange: network.MustParsePortRange("22/tcp")},
			{UUID: "port-range-uuid-2", PortRange: network.MustParsePortRange("23/tcp")},
		},
	}

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

	s.st.EXPECT().GetColocatedOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetUnitOpenedPortsUUID(domaintesting.IsAtomicContextChecker, unitUUID).Return(currentPorts, nil)
	s.st.EXPECT().GetEndpoints(domaintesting.IsAtomicContextChecker, unitUUID).Return(endpoints, nil)
	s.st.EXPECT().AddOpenedPorts(domaintesting.IsAtomicContextChecker, network.GroupedPortRanges{
		"ep1-uuid": {
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("443/tcp"),
		},
		"ep2-uuid": {
			network.MustParsePortRange("8000-9000/udp"),
		},
	}).Return(nil)
	s.st.EXPECT().RemoveOpenedPorts(domaintesting.IsAtomicContextChecker, []string{"port-range-uuid-1"}).Return(nil)

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsNoChanges(c *gc.C) {
	srv := NewService(nil)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{"ep1": {}}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsClosePortsNotOpen(c *gc.C) {
	defer s.setupMocks(c).Finish()

	endpoints := []port.Endpoint{
		{UUID: "wildcard-uuid", Endpoint: WildcardEndpoint},
		{UUID: "ep1-uuid", Endpoint: "ep1"},
	}

	currentPorts := map[string][]port.PortRangeUUID{}

	openPorts := network.GroupedPortRanges{}
	closePorts := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("22/tcp"),
		},
	}

	s.st.EXPECT().GetColocatedOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetUnitOpenedPortsUUID(domaintesting.IsAtomicContextChecker, unitUUID).Return(currentPorts, nil)
	s.st.EXPECT().GetEndpoints(domaintesting.IsAtomicContextChecker, unitUUID).Return(endpoints, nil)

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsOpenedPortsAlreadyOpen(c *gc.C) {
	defer s.setupMocks(c).Finish()

	endpoints := []port.Endpoint{
		{UUID: "wildcard-uuid", Endpoint: WildcardEndpoint},
		{UUID: "ep1-uuid", Endpoint: "ep1"},
	}

	currentPorts := map[string][]port.PortRangeUUID{
		"ep1": {
			{UUID: "port-range-uuid-1", PortRange: network.MustParsePortRange("80/tcp")},
		},
	}

	openPorts := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("443/tcp"),
		},
	}
	closePorts := network.GroupedPortRanges{}

	s.st.EXPECT().GetColocatedOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetUnitOpenedPortsUUID(domaintesting.IsAtomicContextChecker, unitUUID).Return(currentPorts, nil)
	s.st.EXPECT().GetEndpoints(domaintesting.IsAtomicContextChecker, unitUUID).Return(endpoints, nil)
	s.st.EXPECT().AddOpenedPorts(domaintesting.IsAtomicContextChecker, network.GroupedPortRanges{
		"ep1-uuid": {
			network.MustParsePortRange("443/tcp"),
		},
	}).Return(nil)

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsSameRangeAcrossEndpoints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	endpoints := []port.Endpoint{
		{UUID: "wildcard-uuid", Endpoint: WildcardEndpoint},
		{UUID: "ep1-uuid", Endpoint: "ep1"},
		{UUID: "ep2-uuid", Endpoint: "ep2"},
		{UUID: "ep3-uuid", Endpoint: "ep3"},
	}

	currentPorts := map[string][]port.PortRangeUUID{}

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

	s.st.EXPECT().GetColocatedOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetUnitOpenedPortsUUID(domaintesting.IsAtomicContextChecker, unitUUID).Return(currentPorts, nil)
	s.st.EXPECT().GetEndpoints(domaintesting.IsAtomicContextChecker, unitUUID).Return(endpoints, nil)
	s.st.EXPECT().AddOpenedPorts(domaintesting.IsAtomicContextChecker, network.GroupedPortRanges{
		"ep1-uuid": {
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("443/tcp"),
		},
		"ep2-uuid": {
			network.MustParsePortRange("80/tcp"),
		},
		"ep3-uuid": {
			network.MustParsePortRange("80/tcp"),
		},
	}).Return(nil)

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsConflict(c *gc.C) {
	srv := NewService(nil)

	err := srv.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
		"ep2": {
			network.MustParsePortRange("150-250/tcp"),
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIs, port.ErrPortRangeConflict)

	err = srv.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}, network.GroupedPortRanges{
		"ep2": {
			network.MustParsePortRange("150-250/tcp"),
		},
	})
	c.Assert(err, jc.ErrorIs, port.ErrPortRangeConflict)

	err = srv.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{
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

	s.st.EXPECT().GetColocatedOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID).Return([]network.PortRange{
		network.MustParsePortRange("150-250/tcp"),
	}, nil)

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, network.GroupedPortRanges{})

	c.Assert(err, jc.ErrorIs, port.ErrPortRangeConflict)
}

func (s *serviceSuite) TestUpdateUnitPortsClosePortConflictColocated(c *gc.C) {
	defer s.setupMocks(c).Finish()

	closePorts := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}

	s.st.EXPECT().GetColocatedOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID).Return([]network.PortRange{
		network.MustParsePortRange("150-250/tcp"),
	}, nil)

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{}, closePorts)

	c.Assert(err, jc.ErrorIs, port.ErrPortRangeConflict)
}

func (s *serviceSuite) TestUpdateUnitPortsOpenWildcard(c *gc.C) {
	defer s.setupMocks(c).Finish()

	endpoints := []port.Endpoint{
		{UUID: "wildcard-uuid", Endpoint: WildcardEndpoint},
		{UUID: "ep1-uuid", Endpoint: "ep1"},
		{UUID: "ep2-uuid", Endpoint: "ep2"},
		{UUID: "ep3-uuid", Endpoint: "ep3"},
	}

	currentPorts := map[string][]port.PortRangeUUID{
		"ep1": {
			{UUID: "port-range-uuid-1", PortRange: network.MustParsePortRange("100-200/tcp")},
		},
		"ep2": {
			{UUID: "port-range-uuid-2", PortRange: network.MustParsePortRange("100-200/tcp")},
		},
	}

	openPorts := network.GroupedPortRanges{
		WildcardEndpoint: {
			network.MustParsePortRange("100-200/tcp"),
		},
	}
	closePorts := network.GroupedPortRanges{}

	s.st.EXPECT().GetColocatedOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetUnitOpenedPortsUUID(domaintesting.IsAtomicContextChecker, unitUUID).Return(currentPorts, nil)
	s.st.EXPECT().GetEndpoints(domaintesting.IsAtomicContextChecker, unitUUID).Return(endpoints, nil)
	s.st.EXPECT().AddOpenedPorts(domaintesting.IsAtomicContextChecker, network.GroupedPortRanges{
		"wildcard-uuid": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}).Return(nil)
	s.st.EXPECT().RemoveOpenedPorts(domaintesting.IsAtomicContextChecker, []string{"port-range-uuid-1", "port-range-uuid-2"}).Return(nil)

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsOpenPortRangeOpenOnWildcard(c *gc.C) {
	defer s.setupMocks(c).Finish()

	endpoints := []port.Endpoint{
		{UUID: "wildcard-uuid", Endpoint: WildcardEndpoint},
		{UUID: "ep1-uuid", Endpoint: "ep1"},
	}

	currentPorts := map[string][]port.PortRangeUUID{
		WildcardEndpoint: {
			{UUID: "wildcard-uuid", PortRange: network.MustParsePortRange("100-200/tcp")},
		},
	}

	openPorts := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}
	closePorts := network.GroupedPortRanges{}

	s.st.EXPECT().GetColocatedOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetUnitOpenedPortsUUID(domaintesting.IsAtomicContextChecker, unitUUID).Return(currentPorts, nil)
	s.st.EXPECT().GetEndpoints(domaintesting.IsAtomicContextChecker, unitUUID).Return(endpoints, nil)

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsCloseWildcard(c *gc.C) {
	defer s.setupMocks(c).Finish()

	endpoints := []port.Endpoint{
		{UUID: "wildcard-uuid", Endpoint: WildcardEndpoint},
		{UUID: "ep1-uuid", Endpoint: "ep1"},
		{UUID: "ep2-uuid", Endpoint: "ep2"},
		{UUID: "ep3-uuid", Endpoint: "ep3"},
	}

	currentPorts := map[string][]port.PortRangeUUID{
		WildcardEndpoint: {
			{UUID: "wildcard-uuid", PortRange: network.MustParsePortRange("100-200/tcp")},
		},
		"ep1": {
			{UUID: "ep1-uuid", PortRange: network.MustParsePortRange("100-200/tcp")},
		},
		"ep2": {
			{UUID: "ep2-uuid", PortRange: network.MustParsePortRange("100-200/tcp")},
		},
	}

	openPorts := network.GroupedPortRanges{}
	closePorts := network.GroupedPortRanges{
		WildcardEndpoint: {
			network.MustParsePortRange("100-200/tcp"),
		},
	}

	s.st.EXPECT().GetColocatedOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetUnitOpenedPortsUUID(domaintesting.IsAtomicContextChecker, unitUUID).Return(currentPorts, nil)
	s.st.EXPECT().GetEndpoints(domaintesting.IsAtomicContextChecker, unitUUID).Return(endpoints, nil)
	s.st.EXPECT().RemoveOpenedPorts(domaintesting.IsAtomicContextChecker, []string{"ep1-uuid", "ep2-uuid", "wildcard-uuid"}).Return(nil)

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsClosePortRangeOpenOnWildcard(c *gc.C) {
	defer s.setupMocks(c).Finish()

	endpoints := []port.Endpoint{
		{UUID: "wildcard-uuid", Endpoint: WildcardEndpoint},
		{UUID: "ep1-uuid", Endpoint: "ep1"},
		{UUID: "ep2-uuid", Endpoint: "ep2"},
		{UUID: "ep3-uuid", Endpoint: "ep3"},
	}

	currentPorts := map[string][]port.PortRangeUUID{
		WildcardEndpoint: {
			{UUID: "wildcard-uuid", PortRange: network.MustParsePortRange("100-200/tcp")},
		},
	}

	openPorts := network.GroupedPortRanges{}
	closePorts := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}

	s.st.EXPECT().GetColocatedOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetUnitOpenedPortsUUID(domaintesting.IsAtomicContextChecker, unitUUID).Return(currentPorts, nil)
	s.st.EXPECT().GetEndpoints(domaintesting.IsAtomicContextChecker, unitUUID).Return(endpoints, nil)
	s.st.EXPECT().AddOpenedPorts(domaintesting.IsAtomicContextChecker, network.GroupedPortRanges{
		"ep2-uuid": {
			network.MustParsePortRange("100-200/tcp"),
		},
		"ep3-uuid": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}).Return(nil)
	s.st.EXPECT().RemoveOpenedPorts(domaintesting.IsAtomicContextChecker, []string{"wildcard-uuid"}).Return(nil)

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsNeedsToAddSomeEndpoints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	endpoints := []port.Endpoint{
		{UUID: "wildcard-uuid", Endpoint: WildcardEndpoint},
		{UUID: "ep1-uuid", Endpoint: "ep1"},
	}
	newEndpoints := []port.Endpoint{
		{UUID: "ep2-uuid", Endpoint: "ep2"},
		{UUID: "ep3-uuid", Endpoint: "ep3"},
	}

	currentPorts := map[string][]port.PortRangeUUID{}

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

	s.st.EXPECT().GetColocatedOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetUnitOpenedPortsUUID(domaintesting.IsAtomicContextChecker, unitUUID).Return(currentPorts, nil)
	s.st.EXPECT().GetEndpoints(domaintesting.IsAtomicContextChecker, unitUUID).Return(endpoints, nil)
	s.st.EXPECT().AddEndpoints(domaintesting.IsAtomicContextChecker, unitUUID, []string{"ep2", "ep3"}).Return(newEndpoints, nil)
	s.st.EXPECT().AddOpenedPorts(domaintesting.IsAtomicContextChecker, network.GroupedPortRanges{
		"ep1-uuid": {
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("443/tcp"),
		},
		"ep2-uuid": {
			network.MustParsePortRange("80/tcp"),
		},
		"ep3-uuid": {
			network.MustParsePortRange("80/tcp"),
		},
	}).Return(nil)

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortRangeEqualOnColocatedUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	endpoints := []port.Endpoint{
		{UUID: "wildcard-uuid", Endpoint: WildcardEndpoint},
		{UUID: "ep1-uuid", Endpoint: "ep1"},
		{UUID: "ep2-uuid", Endpoint: "ep2"},
	}

	colocatedPorts := []network.PortRange{
		network.MustParsePortRange("100-150/tcp"),
	}

	currentPorts := map[string][]port.PortRangeUUID{}

	openPorts := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-150/tcp"),
		},
	}

	s.st.EXPECT().GetColocatedOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID).Return(colocatedPorts, nil)
	s.st.EXPECT().GetUnitOpenedPortsUUID(domaintesting.IsAtomicContextChecker, unitUUID).Return(currentPorts, nil)
	s.st.EXPECT().GetEndpoints(domaintesting.IsAtomicContextChecker, unitUUID).Return(endpoints, nil)
	s.st.EXPECT().AddOpenedPorts(domaintesting.IsAtomicContextChecker, network.GroupedPortRanges{
		"ep1-uuid": {
			network.MustParsePortRange("100-150/tcp"),
		},
	}).Return(nil)

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateUnitPortRangesConflictsColocated(c *gc.C) {
	defer s.setupMocks(c).Finish()

	colocatedPorts := []network.PortRange{
		network.MustParsePortRange("100-150/tcp"),
	}

	openPorts := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100/tcp"),
		},
	}

	s.st.EXPECT().GetColocatedOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID).Return(colocatedPorts, nil)

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIs, port.ErrPortRangeConflict)
}

func (s *serviceSuite) TestUpdateUnitPortRangeEqualOnOtherEndpoint(c *gc.C) {
	defer s.setupMocks(c).Finish()

	endpoints := []port.Endpoint{
		{UUID: "wildcard-uuid", Endpoint: WildcardEndpoint},
		{UUID: "ep1-uuid", Endpoint: "ep1"},
		{UUID: "ep2-uuid", Endpoint: "ep2"},
	}

	currentPorts := map[string][]port.PortRangeUUID{
		"ep2": {
			{UUID: "ep1-uuid", PortRange: network.MustParsePortRange("100-200/tcp")},
		},
	}

	colocatedPorts := []network.PortRange{network.MustParsePortRange("100-200/tcp")}

	openPorts := network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}

	s.st.EXPECT().GetColocatedOpenedPorts(domaintesting.IsAtomicContextChecker, unitUUID).Return(colocatedPorts, nil)
	s.st.EXPECT().GetUnitOpenedPortsUUID(domaintesting.IsAtomicContextChecker, unitUUID).Return(currentPorts, nil)
	s.st.EXPECT().GetEndpoints(domaintesting.IsAtomicContextChecker, unitUUID).Return(endpoints, nil)
	s.st.EXPECT().AddOpenedPorts(domaintesting.IsAtomicContextChecker, network.GroupedPortRanges{
		"ep1-uuid": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}).Return(nil)

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)
}
