// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	domain "github.com/juju/juju/domain"
)

type serviceSuite struct {
	st *MockState
}

var _ = gc.Suite(&serviceSuite{})

const unitUUID = "unit-uuid"

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)
	s.st.EXPECT().RunAtomic(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(ctx domain.AtomicContext) error) error {
		return fn(nil)
	}).AnyTimes()

	return ctrl
}

func (s *serviceSuite) TestGetOpenedPorts(c *gc.C) {
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

	s.st.EXPECT().GetOpenedPorts(gomock.Any(), unitUUID).Return(grp, nil)

	srv := NewService(s.st)
	res, err := srv.GetOpenedPorts(context.Background(), unitUUID)
	c.Assert(err, gc.IsNil)
	c.Assert(res, gc.DeepEquals, grp)
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

	s.st.EXPECT().GetOpenedEndpointPorts(gomock.Any(), unitUUID, WildcardEndpoint).Return([]network.PortRange{}, nil)
	s.st.EXPECT().UpdateUnitPorts(gomock.Any(), unitUUID, openPorts, closePorts).Return(nil)

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
	c.Assert(err, gc.IsNil)
}

func (s *serviceSuite) TestUpdateUnitPortsNoChanges(c *gc.C) {
	srv := NewService(nil)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{"ep1": {}}, network.GroupedPortRanges{})
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

	s.st.EXPECT().GetOpenedEndpointPorts(gomock.Any(), unitUUID, WildcardEndpoint).Return([]network.PortRange{}, nil)
	s.st.EXPECT().UpdateUnitPorts(gomock.Any(), unitUUID, openPorts, closePorts).Return(nil)

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
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	err = srv.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}, network.GroupedPortRanges{
		"ep2": {
			network.MustParsePortRange("150-250/tcp"),
		},
	})
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	err = srv.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
			network.MustParsePortRange("200/tcp"),
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestUpdateUnitPortsOpenWildcard(c *gc.C) {
	defer s.setupMocks(c).Finish()

	openPorts := network.GroupedPortRanges{
		WildcardEndpoint: {
			network.MustParsePortRange("100-200/tcp"),
		},
	}
	closePorts := network.GroupedPortRanges{}

	s.st.EXPECT().GetOpenedEndpointPorts(gomock.Any(), unitUUID, WildcardEndpoint).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetEndpoints(gomock.Any(), unitUUID).Return([]string{WildcardEndpoint, "ep1", "ep2", "ep3"}, nil)
	s.st.EXPECT().UpdateUnitPorts(gomock.Any(), unitUUID, openPorts, network.GroupedPortRanges{
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

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
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

	s.st.EXPECT().GetOpenedEndpointPorts(gomock.Any(), unitUUID, WildcardEndpoint).Return([]network.PortRange{
		network.MustParsePortRange("100-200/tcp"),
	}, nil)
	s.st.EXPECT().UpdateUnitPorts(gomock.Any(), unitUUID, openPorts, network.GroupedPortRanges{}).Return(nil)

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
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

	s.st.EXPECT().GetOpenedEndpointPorts(gomock.Any(), unitUUID, WildcardEndpoint).Return([]network.PortRange{}, nil)
	s.st.EXPECT().GetEndpoints(gomock.Any(), unitUUID).Return([]string{WildcardEndpoint, "ep1", "ep2", "ep3"}, nil)
	s.st.EXPECT().UpdateUnitPorts(gomock.Any(), unitUUID, openPorts, network.GroupedPortRanges{
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

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
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

	s.st.EXPECT().GetOpenedEndpointPorts(gomock.Any(), unitUUID, WildcardEndpoint).Return([]network.PortRange{
		network.MustParsePortRange("100-200/tcp"),
	}, nil)
	s.st.EXPECT().GetEndpoints(gomock.Any(), unitUUID).Return([]string{WildcardEndpoint, "ep1", "ep2", "ep3"}, nil)
	s.st.EXPECT().UpdateUnitPorts(gomock.Any(), unitUUID, network.GroupedPortRanges{
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

	srv := NewService(s.st)
	err := srv.UpdateUnitPorts(context.Background(), unitUUID, openPorts, closePorts)
	c.Assert(err, jc.ErrorIsNil)
}
