// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	controllernode "github.com/juju/juju/domain/controllernode"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	state *MockState
}

var _ = tc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}

func (s *serviceSuite) TestUpdateExternalControllerSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CurateNodes(gomock.Any(), []string{"3", "4"}, []string{"1"})

	err := NewService(s.state, loggertesting.WrapCheckLog(c)).CurateNodes(c.Context(), []string{"3", "4"}, []string{"1"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateDqliteNode(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().UpdateDqliteNode(gomock.Any(), "0", uint64(12345), "192.168.5.60")

	err := NewService(s.state, loggertesting.WrapCheckLog(c)).UpdateDqliteNode(c.Context(), "0", 12345, "192.168.5.60")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestIsModelKnownToController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	knownID := "known"
	fakeID := "fake"

	exp := s.state.EXPECT()
	gomock.InOrder(
		exp.SelectDatabaseNamespace(gomock.Any(), fakeID).Return("", controllernodeerrors.NotFound),
		exp.SelectDatabaseNamespace(gomock.Any(), knownID).Return(knownID, nil),
	)

	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	known, err := svc.IsKnownDatabaseNamespace(c.Context(), fakeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(known, tc.IsFalse)

	known, err = svc.IsKnownDatabaseNamespace(c.Context(), knownID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(known, tc.IsTrue)
}

func (s *serviceSuite) TestSetControllerNodeAgentVersionSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	controllerID := "1"
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("1.2.3"),
		Arch:   corearch.ARM64,
	}

	s.state.EXPECT().SetRunningAgentBinaryVersion(gomock.Any(), controllerID, ver).Return(nil)

	svc := NewService(s.state, loggertesting.WrapCheckLog(c))
	err := svc.SetControllerNodeReportedAgentVersion(
		c.Context(),
		controllerID,
		ver,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetControllerNodeAgentVersionNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	controllerID := "1"

	ver := coreagentbinary.Version{
		Number: semversion.Zero,
	}
	err := svc.SetControllerNodeReportedAgentVersion(
		c.Context(),
		controllerID,
		ver,
	)
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	ver = coreagentbinary.Version{
		Number: semversion.MustParse("1.2.3"),
		Arch:   corearch.UnsupportedArches[0],
	}
	err = svc.SetControllerNodeReportedAgentVersion(
		c.Context(),
		controllerID,
		ver,
	)
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestSetControllerNodeAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	controllerID := "1"
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("1.2.3"),
		Arch:   corearch.ARM64,
	}

	s.state.EXPECT().SetRunningAgentBinaryVersion(gomock.Any(), controllerID, ver).Return(controllernodeerrors.NotFound)

	err := svc.SetControllerNodeReportedAgentVersion(
		c.Context(),
		controllerID,
		ver,
	)
	c.Assert(err, tc.ErrorIs, controllernodeerrors.NotFound)
}

func (s *serviceSuite) TestIsControllerNode(c *tc.C) {
	defer s.setupMocks(c).Finish()

	controllerID := "1"
	fakeID := "fake"

	exp := s.state.EXPECT()
	gomock.InOrder(
		exp.IsControllerNode(gomock.Any(), fakeID).Return(false, controllernodeerrors.NotFound),
		exp.IsControllerNode(gomock.Any(), controllerID).Return(true, nil),
	)

	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	is, err := svc.IsControllerNode(c.Context(), fakeID)
	c.Assert(err, tc.ErrorIs, controllernodeerrors.NotFound)
	c.Check(is, tc.IsFalse)

	is, err = svc.IsControllerNode(c.Context(), controllerID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(is, tc.IsTrue)
}

func (s *serviceSuite) TestIsControllerNodeNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	controllerID := ""

	is, err := svc.IsControllerNode(c.Context(), controllerID)
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Check(is, tc.IsFalse)
}

func (s *serviceSuite) TestSetAPIAddressesStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	controllerID := "1"
	s.state.EXPECT().SetAPIAddresses(gomock.Any(), controllerID, gomock.Any()).Return(internalerrors.New("boom"))

	err := svc.SetAPIAddresses(c.Context(), controllerID, network.SpaceHostPorts{{}}, &network.SpaceInfo{})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestSetAPIAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	controllerID := "1"

	controllerApiAddrs := []controllernode.APIAddress{
		{
			Address: "10.0.0.1:17070",
			IsAgent: true,
		},
		{
			Address: "10.0.0.2:17070",
			IsAgent: false,
		},
	}
	s.state.EXPECT().SetAPIAddresses(gomock.Any(), controllerID, controllerApiAddrs).Return(nil)

	addrs := network.SpaceHostPorts{
		{
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.1",
				},
				SpaceID: "space0-uuid",
			},
			NetPort: network.NetPort(17070),
		},
		{
			// This address is in a different space.
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.2",
				},
				SpaceID: "space1-uuid",
			},
			NetPort: network.NetPort(17070),
		},
	}
	err := svc.SetAPIAddresses(c.Context(), controllerID, addrs, &network.SpaceInfo{
		ID:   "space0-uuid",
		Name: "space0",
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetAPIAddressesNilMgmtSpace(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	controllerID := "1"

	controllerApiAddrs := []controllernode.APIAddress{
		{
			Address: "10.0.0.1:17070",
			IsAgent: true,
		},
		{
			Address: "10.0.0.2:17070",
			IsAgent: true,
		},
	}
	s.state.EXPECT().SetAPIAddresses(gomock.Any(), controllerID, controllerApiAddrs).Return(nil)

	addrs := network.SpaceHostPorts{
		{
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.1",
				},
				SpaceID: "space0-uuid",
			},
			NetPort: network.NetPort(17070),
		},
		{
			// This address is in a different space.
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.2",
				},
				SpaceID: "space1-uuid",
			},
			NetPort: network.NetPort(17070),
		},
	}
	err := svc.SetAPIAddresses(c.Context(), controllerID, addrs, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetAPIAddressesAllAddrsFilteredAgents(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	controllerID := "1"

	controllerApiAddrs := []controllernode.APIAddress{
		{
			Address: "10.0.0.1:17070",
			IsAgent: true,
		},
		{
			Address: "10.0.0.2:17070",
			IsAgent: true,
		},
	}
	s.state.EXPECT().SetAPIAddresses(gomock.Any(), controllerID, controllerApiAddrs).Return(nil)

	addrs := network.SpaceHostPorts{
		{
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.1",
				},
				SpaceID: "space1-uuid",
			},
			NetPort: network.NetPort(17070),
		},
		{
			// This address is in a different space.
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.2",
				},
				SpaceID: "space2-uuid",
			},
			NetPort: network.NetPort(17070),
		},
	}
	err := svc.SetAPIAddresses(c.Context(), controllerID, addrs, &network.SpaceInfo{
		ID: "space0-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetAPIAddressesNotAllAddrsFilteredAgents(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	controllerID := "1"

	controllerApiAddrs := []controllernode.APIAddress{
		{
			Address: "10.0.0.1:17070",
			IsAgent: false,
		},
		{
			Address: "10.0.0.2:17070",
			IsAgent: true,
		},
	}
	s.state.EXPECT().SetAPIAddresses(gomock.Any(), controllerID, controllerApiAddrs).Return(nil)

	addrs := network.SpaceHostPorts{
		{
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.1",
				},
				SpaceID: "space1-uuid",
			},
			NetPort: network.NetPort(17070),
		},
		{
			// This address is in a different space.
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.2",
				},
				SpaceID: "space0-uuid",
			},
			NetPort: network.NetPort(17070),
		},
	}
	err := svc.SetAPIAddresses(c.Context(), controllerID, addrs, &network.SpaceInfo{
		ID: "space0-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestGetControllerIDs(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	s.state.EXPECT().GetControllerIDs(gomock.Any()).Return([]string{"1", "2"}, nil)

	controllerIDs, err := svc.GetControllerIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerIDs, tc.HasLen, 2)
	c.Check(controllerIDs, tc.DeepEquals, []string{"1", "2"})
}
