// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	watcher "github.com/juju/juju/core/watcher"
	eventsource "github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/core/watcher/watchertest"
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

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	c.Cleanup(func() {
		s.state = nil
	})

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
			Scope:   network.ScopePublic,
		},
		{
			Address: "10.0.0.2:17070",
			IsAgent: false,
			Scope:   network.ScopePublic,
		},
	}
	s.state.EXPECT().SetAPIAddresses(gomock.Any(), controllerID, controllerApiAddrs).Return(nil)

	addrs := network.SpaceHostPorts{
		{
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.1",
					Scope: network.ScopePublic,
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
					Scope: network.ScopePublic,
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
			Scope:   network.ScopeCloudLocal,
		},
		{
			Address: "10.0.0.2:17070",
			IsAgent: true,
			Scope:   network.ScopeCloudLocal,
		},
	}
	s.state.EXPECT().SetAPIAddresses(gomock.Any(), controllerID, controllerApiAddrs).Return(nil)

	addrs := network.SpaceHostPorts{
		{
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.1",
					Scope: network.ScopeCloudLocal,
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
					Scope: network.ScopeCloudLocal,
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
			Scope:   network.ScopeCloudLocal,
		},
		{
			Address: "10.0.0.2:17070",
			IsAgent: true,
			Scope:   network.ScopeCloudLocal,
		},
	}
	s.state.EXPECT().SetAPIAddresses(gomock.Any(), controllerID, controllerApiAddrs).Return(nil)

	addrs := network.SpaceHostPorts{
		{
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.1",
					Scope: network.ScopeCloudLocal,
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
					Scope: network.ScopeCloudLocal,
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
			Scope:   network.ScopePublic,
		},
		{
			Address: "10.0.0.2:17070",
			IsAgent: true,
			Scope:   network.ScopeCloudLocal,
		},
	}
	s.state.EXPECT().SetAPIAddresses(gomock.Any(), controllerID, controllerApiAddrs).Return(nil)

	addrs := network.SpaceHostPorts{
		{
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.1",
					Scope: network.ScopePublic,
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
					Scope: network.ScopeCloudLocal,
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

func (s *serviceSuite) TestGetControllerIDsError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	s.state.EXPECT().GetControllerIDs(gomock.Any()).Return(nil, internalerrors.Errorf("boom"))

	_, err := svc.GetControllerIDs(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestAllAPIAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	s.state.EXPECT().GetAPIAddresses(gomock.Any(), "1").Return([]string{
		"10.0.0.1:17070",
		"10.0.0.2:17070",
	}, nil)

	apiAddrs, err := svc.GetAPIAddresses(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(apiAddrs, tc.DeepEquals, []string{
		"10.0.0.1:17070",
		"10.0.0.2:17070",
	})
}

func (s *serviceSuite) TestGetAPIAddressesError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	s.state.EXPECT().GetAPIAddresses(gomock.Any(), "1").Return(nil, internalerrors.Errorf("boom"))

	_, err := svc.GetAPIAddresses(c.Context(), "1")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestGetAPIAddressesForAgents(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	s.state.EXPECT().GetAPIAddressesForAgents(gomock.Any(), "1").Return([]string{
		"10.0.0.1:17070",
		"10.0.0.2:17070",
	}, nil)

	apiAddrs, err := svc.GetAPIAddressesForAgents(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(apiAddrs, tc.DeepEquals, []string{
		"10.0.0.1:17070",
		"10.0.0.2:17070",
	})
}

func (s *serviceSuite) TestGetAPIAddressesForAgentsError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	s.state.EXPECT().GetAPIAddressesForAgents(gomock.Any(), "1").Return(nil, internalerrors.Errorf("boom"))

	_, err := svc.GetAPIAddressesForAgents(c.Context(), "1")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestGetAllAPIAddressesForAgents(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	s.state.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(map[string][]string{
		"1": {
			"10.0.0.1:17070",
		},
		"2": {
			"10.0.0.2:17070",
		},
	}, nil)

	apiAddrs, err := svc.GetAllAPIAddressesForAgents(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(apiAddrs, tc.DeepEquals, map[string][]string{
		"1": {
			"10.0.0.1:17070",
		},
		"2": {
			"10.0.0.2:17070",
		},
	})
}

func (s *serviceSuite) TestGetAllAPIAddressesForAgentsError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	s.state.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(nil, internalerrors.Errorf("boom"))

	_, err := svc.GetAllAPIAddressesForAgents(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestGetAllAPIAddressesForAgentsInPreferredOrder(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	// Arrange
	args := []controllernode.APIAddresses{
		{
			{
				Address: "10.0.0.1:17070",
				Scope:   network.ScopeCloudLocal,
			}, { // This address not in result, machine local.
				Address: "10.0.0.2:17070",
				Scope:   network.ScopeMachineLocal,
			},
		}, {
			{
				Address: "10.0.0.43:17070",
				Scope:   network.ScopePublic,
			}, {
				Address: "10.0.0.7:17070",
				Scope:   network.ScopeCloudLocal,
			},
		},
	}
	s.state.EXPECT().GetAllAPIAddressesWithScopeForAgents(gomock.Any()).Return(args, nil)

	// Act
	apiAddrs, err := svc.GetAllAPIAddressesForAgentsInPreferredOrder(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(apiAddrs, tc.DeepEquals, []string{"10.0.0.1:17070", "10.0.0.7:17070", "10.0.0.43:17070"})
}

func (s *serviceSuite) TestGetAllAPIAddressesForAgentsInPreferredOrderError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	s.state.EXPECT().GetAllAPIAddressesWithScopeForAgents(gomock.Any()).Return(nil, internalerrors.Errorf("boom"))

	_, err := svc.GetAllAPIAddressesForAgentsInPreferredOrder(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestGetAllNoProxyAPIAddressesForAgents(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	// Arrange: out of sorted order ip addresses for no proxy
	// method.
	args := []controllernode.APIAddresses{
		{
			{ // This address should be ignored
				Address: "42.1.2.4:17070",
				Scope:   network.ScopeMachineLocal,
			}, {
				Address: "10.0.0.7:17070",
				Scope:   network.ScopeCloudLocal,
			},
		}, {
			{
				Address: "10.0.0.1:17070",
				Scope:   network.ScopeCloudLocal,
			}, { // This address should be ignored
				Address: "42.1.2.34:17070",
				Scope:   network.ScopeMachineLocal,
			},
		},
	}
	s.state.EXPECT().GetAllAPIAddressesWithScopeForAgents(gomock.Any()).Return(args, nil)

	// Act
	apiAddrs, err := svc.GetAllNoProxyAPIAddressesForAgents(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(apiAddrs, tc.DeepEquals, "10.0.0.1,10.0.0.7")
}

func (s *serviceSuite) TestGetAllNoProxyAPIAddressesForAgentsError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	s.state.EXPECT().GetAllAPIAddressesWithScopeForAgents(gomock.Any()).Return(nil, internalerrors.Errorf("boom"))

	_, err := svc.GetAllNoProxyAPIAddressesForAgents(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestGetAllAPIAddressesForClients(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	// Arrange
	args := []controllernode.APIAddresses{
		{
			{
				Address: "10.0.0.1:17070",
				IsAgent: true,
				Scope:   network.ScopeCloudLocal,
			}, {
				Address: "10.0.0.2:17070",
				IsAgent: false,
				Scope:   network.ScopePublic,
			},
		}, {
			{
				Address: "10.0.0.34:17070",
				IsAgent: true,
				Scope:   network.ScopePublic,
			}, { // This address shouldn't appear.
				Address: "10.0.0.9:17070",
				IsAgent: false,
				Scope:   network.ScopeMachineLocal,
			},
		},
	}
	s.state.EXPECT().GetAllAPIAddressesWithScopeForClients(gomock.Any()).Return(args, nil)

	// Act
	apiAddrs, err := svc.GetAllAPIAddressesForClients(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(apiAddrs, tc.DeepEquals, []string{"10.0.0.2:17070", "10.0.0.1:17070", "10.0.0.34:17070"})
}

func (s *serviceSuite) TestGetAllAPIAddressesForClientsError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	s.state.EXPECT().GetAllAPIAddressesWithScopeForClients(gomock.Any()).Return(nil, internalerrors.Errorf("boom"))

	_, err := svc.GetAllAPIAddressesForClients(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestGetAllCloudLocalAPIAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	returnAddrs := []string{"9.3.5.2:17070", "3.4.2.5:17070"}
	s.state.EXPECT().GetAllCloudLocalAPIAddresses(gomock.Any()).Return(returnAddrs, nil)

	// Act
	obtainedAddrs, err := svc.GetAllCloudLocalAPIAddresses(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedAddrs, tc.DeepEquals, []string{"9.3.5.2", "3.4.2.5"})
}

func (s *serviceSuite) TestGetAllCloudLocalAPIAddressesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.state.EXPECT().GetAllCloudLocalAPIAddresses(gomock.Any()).Return(nil, internalerrors.Errorf("boom"))
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))

	// Act
	_, err := svc.GetAllCloudLocalAPIAddresses(c.Context())

	// Assert
	c.Assert(err, tc.ErrorMatches, "boom")
}

type watchableServiceSuite struct {
	testhelpers.IsolationSuite

	state          *MockState
	watcherFactory *MockWatcherFactory
}

func TestWatchableServiceSuite(t *testing.T) {
	tc.Run(t, &watchableServiceSuite{})
}

func (s *watchableServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	c.Cleanup(func() {
		s.state = nil
		s.watcherFactory = nil
	})

	return ctrl
}

func (s *watchableServiceSuite) TestWatchControllerNode(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var namespace string

	ch := make(chan struct{}, 1)
	s.watcherFactory.EXPECT().NewNotifyWatcher(gomock.Any()).DoAndReturn(func(fo eventsource.FilterOption, _ ...eventsource.FilterOption) (watcher.Watcher[struct{}], error) {
		namespace = fo.Namespace()

		watcher := watchertest.NewMockNotifyWatcher(ch)
		return watcher, nil
	})

	s.state.EXPECT().NamespaceForWatchControllerNodes().Return("controller-nodes")

	svc := NewWatchableService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	watcher, err := svc.WatchControllerNodes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, watcher)

	c.Assert(namespace, tc.Equals, "controller-nodes")
}

func (s *watchableServiceSuite) TestWatchControllerNodeError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.watcherFactory.EXPECT().NewNotifyWatcher(gomock.Any()).DoAndReturn(func(fo eventsource.FilterOption, _ ...eventsource.FilterOption) (watcher.Watcher[struct{}], error) {
		return nil, internalerrors.Errorf("boom")
	})

	s.state.EXPECT().NamespaceForWatchControllerNodes().Return("controller-nodes")

	svc := NewWatchableService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
	_, err := svc.WatchControllerNodes(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}
