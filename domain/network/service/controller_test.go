// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type controllerNetworkSuite struct{}

func TestControllerNetworkSuite(t *testing.T) { tc.Run(t, &controllerNetworkSuite{}) }

func (*controllerNetworkSuite) TestPeerManagementSpaceAndClientEligibility(c *tc.C) {
	st := NewMockState(gomock.NewController(c))
	public := controllerCandidate("192.0.2.1", network.ScopePublic, "public", domainnetwork.DeviceTypeEthernet)
	management := controllerCandidate("10.0.0.1", network.ScopeCloudLocal, "management", domainnetwork.DeviceTypeVeth)
	st.EXPECT().GetControllerUnitNetwork(gomock.Any(), "controller/0").Return(domainnetwork.ControllerAPIAddresses{public, management}, nil).Times(3)
	st.EXPECT().GetModelType(gomock.Any()).Return(model.IAAS, nil).Times(3)
	st.EXPECT().GetSpaceByName(gomock.Any(), network.SpaceName("management")).Return(&network.SpaceInfo{ID: "management"}, nil)
	svc := NewService(st, loggertesting.WrapCheckLog(c))
	peers, err := svc.GetControllerPeerAddresses(c.Context(), "controller/0", "management")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(peers, tc.DeepEquals, network.SpaceAddresses{management.SpaceAddress})
	// Management-space configuration must not add otherwise unselected VETH
	// addresses to the client selection.
	clients, err := svc.GetControllerClientAddresses(c.Context(), []unit.Name{"controller/0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(clients.ByUnit["controller/0"], tc.DeepEquals, network.SpaceAddresses{public.SpaceAddress})
	peers, err = svc.GetControllerPeerAddresses(c.Context(), "controller/0", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(peers, tc.DeepEquals, network.SpaceAddresses{public.SpaceAddress})
}

func (*controllerNetworkSuite) TestClientAddressesRetainEligibleManagementAddresses(c *tc.C) {
	st := NewMockState(gomock.NewController(c))
	public := controllerCandidate("192.0.2.1", network.ScopePublic, "public", domainnetwork.DeviceTypeEthernet)
	management := controllerCandidate("10.0.0.1", network.ScopeCloudLocal, "management", domainnetwork.DeviceTypeEthernet)
	st.EXPECT().GetControllerUnitNetwork(gomock.Any(), "controller/0").Return(domainnetwork.ControllerAPIAddresses{public, management}, nil)
	st.EXPECT().GetModelType(gomock.Any()).Return(model.IAAS, nil)
	// No space lookup is required for either eligible address.
	addresses, err := NewService(st, loggertesting.WrapCheckLog(c)).GetControllerClientAddresses(c.Context(), []unit.Name{"controller/0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses.ByUnit["controller/0"], tc.DeepEquals, network.SpaceAddresses{public.SpaceAddress, management.SpaceAddress})
}

func (*controllerNetworkSuite) TestManagementSpaceFallback(c *tc.C) {
	st := NewMockState(gomock.NewController(c))
	address := controllerCandidate("10.0.0.1", network.ScopeCloudLocal, "other", domainnetwork.DeviceTypeVeth)
	st.EXPECT().GetControllerUnitNetwork(gomock.Any(), "controller/0").Return(domainnetwork.ControllerAPIAddresses{address}, nil)
	st.EXPECT().GetModelType(gomock.Any()).Return(model.IAAS, nil)
	st.EXPECT().GetSpaceByName(gomock.Any(), network.SpaceName("management")).Return(&network.SpaceInfo{ID: "management"}, nil)
	addresses, err := NewService(st, loggertesting.WrapCheckLog(c)).GetControllerPeerAddresses(c.Context(), "controller/0", "management")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses, tc.DeepEquals, network.SpaceAddresses{address.SpaceAddress})
}

func (*controllerNetworkSuite) TestPodIPAndDNSIgnoreManagementSpace(c *tc.C) {
	st := NewMockState(gomock.NewController(c))
	ip := controllerCandidate("10.0.0.1", network.ScopeMachineLocal, network.AlphaSpaceId, domainnetwork.DeviceTypeEthernet)
	dns := controllerCandidate("controller-0.example.test", network.ScopeCloudLocal, network.AlphaSpaceId, domainnetwork.DeviceTypeUnknown)
	st.EXPECT().GetControllerUnitNetwork(gomock.Any(), "controller/0").Return(domainnetwork.ControllerAPIAddresses{ip, dns}, nil).Times(2)
	st.EXPECT().GetModelType(gomock.Any()).Return(model.CAAS, nil).Times(2)
	st.EXPECT().GetSpaceByName(gomock.Any(), gomock.Any()).Times(0)
	svc := NewService(st, loggertesting.WrapCheckLog(c))
	addresses, err := svc.GetControllerPeerAddresses(c.Context(), "controller/0", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses, tc.DeepEquals, network.SpaceAddresses{dns.SpaceAddress, ip.SpaceAddress})
	withManagementSpace, err := svc.GetControllerPeerAddresses(c.Context(), "controller/0", "management")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(withManagementSpace, tc.DeepEquals, addresses)
}

func (*controllerNetworkSuite) TestUnsuitableAddresses(c *tc.C) {
	st := NewMockState(gomock.NewController(c))
	var candidates domainnetwork.ControllerAPIAddresses
	for _, value := range []string{"127.0.0.1", "::1", "169.254.1.1", "fe80::1", "0.0.0.0", "224.0.0.1"} {
		candidates = append(candidates, controllerCandidate(value, network.ScopeCloudLocal, network.AlphaSpaceId, domainnetwork.DeviceTypeEthernet))
	}
	candidates = append(candidates, controllerCandidate("10.0.0.1", network.ScopeMachineLocal, network.AlphaSpaceId, domainnetwork.DeviceTypeEthernet))
	st.EXPECT().GetControllerUnitNetwork(gomock.Any(), "controller/0").Return(candidates, nil)
	st.EXPECT().GetModelType(gomock.Any()).Return(model.IAAS, nil)
	addresses, err := NewService(st, loggertesting.WrapCheckLog(c)).GetControllerPeerAddresses(c.Context(), "controller/0", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses, tc.HasLen, 0)
}

func (*controllerNetworkSuite) TestErrorsAreNotEmptySnapshots(c *tc.C) {
	st := NewMockState(gomock.NewController(c))
	svc := NewService(st, loggertesting.WrapCheckLog(c))
	_, err := svc.GetControllerPeerAddresses(c.Context(), "postgresql/0", "")
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
	_, err = svc.GetControllerPeerAddresses(c.Context(), unit.Name("invalid"), "")
	c.Check(err, tc.ErrorIs, unit.InvalidUnitName)
	st.EXPECT().GetControllerUnitNetwork(gomock.Any(), "controller/0").Return(nil, applicationerrors.UnitIsDead)
	_, err = svc.GetControllerPeerAddresses(c.Context(), "controller/0", "")
	c.Check(err, tc.ErrorIs, applicationerrors.UnitIsDead)
	st.EXPECT().GetControllerUnitNetwork(gomock.Any(), "controller/0").Return(nil, nil)
	st.EXPECT().GetModelType(gomock.Any()).Return(model.IAAS, nil)
	st.EXPECT().GetSpaceByName(gomock.Any(), network.SpaceName("missing")).Return(nil, networkerrors.SpaceNotFound)
	_, err = svc.GetControllerPeerAddresses(c.Context(), "controller/0", "missing")
	c.Check(err, tc.ErrorIs, networkerrors.SpaceNotFound)
}

func (*controllerNetworkSuite) TestModelTypeReadFailure(c *tc.C) {
	st := NewMockState(gomock.NewController(c))
	address := controllerCandidate("192.0.2.1", network.ScopePublic, network.AlphaSpaceId, domainnetwork.DeviceTypeEthernet)
	st.EXPECT().GetControllerUnitNetwork(gomock.Any(), "controller/0").Return(domainnetwork.ControllerAPIAddresses{address}, nil)
	failure := errors.New("model type unavailable")
	st.EXPECT().GetModelType(gomock.Any()).Return(model.ModelType(""), failure).Times(3)
	svc := NewService(st, loggertesting.WrapCheckLog(c))
	addresses, err := svc.GetControllerPeerAddresses(c.Context(), "controller/0", "")
	c.Check(err, tc.ErrorIs, failure)
	c.Check(addresses, tc.IsNil)
	selection, err := svc.GetControllerClientAddresses(c.Context(), []unit.Name{"controller/0"})
	c.Check(err, tc.ErrorIs, failure)
	c.Check(selection, tc.DeepEquals, domainnetwork.ControllerAddressSelection{})
	selection, err = svc.GetControllerAgentAddresses(c.Context(), []unit.Name{"controller/0"}, "")
	c.Check(err, tc.ErrorIs, failure)
	c.Check(selection, tc.DeepEquals, domainnetwork.ControllerAddressSelection{})
}

func (*controllerNetworkSuite) TestDiscoveryKeepsControllerIdentityAndManagementPolicy(c *tc.C) {
	st := NewMockState(gomock.NewController(c))
	public := controllerCandidate("192.0.2.1", network.ScopePublic, "public", domainnetwork.DeviceTypeEthernet)
	management := controllerCandidate("10.0.0.1", network.ScopeCloudLocal, "management", domainnetwork.DeviceTypeVeth)
	fallback := controllerCandidate("192.0.2.2", network.ScopePublic, "public", domainnetwork.DeviceTypeVeth)
	st.EXPECT().GetModelType(gomock.Any()).Return(model.IAAS, nil).Times(2)
	st.EXPECT().GetControllerUnitNetwork(gomock.Any(), "controller/0").Return(domainnetwork.ControllerAPIAddresses{public, management}, nil).Times(2)
	st.EXPECT().GetControllerUnitNetwork(gomock.Any(), "controller/1").Return(domainnetwork.ControllerAPIAddresses{fallback}, nil).Times(2)
	st.EXPECT().GetSpaceByName(gomock.Any(), network.SpaceName("management")).Return(&network.SpaceInfo{ID: "management"}, nil).Times(2)
	svc := NewService(st, loggertesting.WrapCheckLog(c))
	names := []unit.Name{"controller/0", "controller/1"}
	agents, err := svc.GetControllerAgentAddresses(c.Context(), names, "management")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(agents.Shared, tc.HasLen, 0)
	c.Check(agents.ByUnit, tc.DeepEquals, map[unit.Name]network.SpaceAddresses{
		"controller/0": {management.SpaceAddress},
		"controller/1": {fallback.SpaceAddress},
	})
	clients, err := svc.GetControllerClientAddresses(c.Context(), names)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(clients.Shared, tc.HasLen, 0)
	c.Check(clients.ByUnit, tc.DeepEquals, map[unit.Name]network.SpaceAddresses{
		"controller/0": {public.SpaceAddress},
		"controller/1": {fallback.SpaceAddress},
	})
}

func (*controllerNetworkSuite) TestServiceDiscoveryPreferenceAndFallback(c *tc.C) {
	public := controllerCandidate("api.example.test", network.ScopePublic, network.AlphaSpaceId, domainnetwork.DeviceTypeUnknown)
	private := controllerCandidate("10.0.0.1", network.ScopeCloudLocal, network.AlphaSpaceId, domainnetwork.DeviceTypeUnknown)
	for _, test := range []struct {
		candidates domainnetwork.ControllerAPIAddresses
		clients    network.SpaceAddresses
		agents     network.SpaceAddresses
	}{
		{domainnetwork.ControllerAPIAddresses{private, public}, network.SpaceAddresses{public.SpaceAddress, private.SpaceAddress}, network.SpaceAddresses{private.SpaceAddress, public.SpaceAddress}},
		{domainnetwork.ControllerAPIAddresses{private}, network.SpaceAddresses{private.SpaceAddress}, network.SpaceAddresses{private.SpaceAddress}},
		{domainnetwork.ControllerAPIAddresses{public}, network.SpaceAddresses{public.SpaceAddress}, network.SpaceAddresses{public.SpaceAddress}},
		{nil, network.SpaceAddresses{}, network.SpaceAddresses{}},
	} {
		st := NewMockState(gomock.NewController(c))
		st.EXPECT().GetModelType(gomock.Any()).Return(model.CAAS, nil).Times(2)
		st.EXPECT().GetControllerServiceAddresses(gomock.Any()).Return(test.candidates, nil).Times(2)
		svc := NewService(st, loggertesting.WrapCheckLog(c))
		// No unit read or management-space lookup is needed for Service
		// discovery, even with empty membership or an unavailable space.
		clients, err := svc.GetControllerClientAddresses(c.Context(), nil)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(clients.ByUnit, tc.HasLen, 0)
		c.Check(clients.Shared, tc.DeepEquals, test.clients)
		agents, err := svc.GetControllerAgentAddresses(c.Context(), []unit.Name{"controller/0"}, "missing")
		c.Assert(err, tc.ErrorIsNil)
		c.Check(agents.ByUnit, tc.HasLen, 0)
		c.Check(agents.Shared, tc.DeepEquals, test.agents)
	}
}

func (*controllerNetworkSuite) TestServiceDiscoveryExcludesUnsuitableAddresses(c *tc.C) {
	st := NewMockState(gomock.NewController(c))
	var candidates domainnetwork.ControllerAPIAddresses
	for _, value := range []string{"127.0.0.1", "::1", "169.254.1.1", "fe80::1", "0.0.0.0", "::", "224.0.0.1", "ff02::1"} {
		candidates = append(candidates, controllerCandidate(value, network.ScopeCloudLocal, network.AlphaSpaceId, domainnetwork.DeviceTypeUnknown))
	}
	candidates = append(candidates, controllerCandidate("10.0.0.1", network.ScopeMachineLocal, network.AlphaSpaceId, domainnetwork.DeviceTypeUnknown))
	st.EXPECT().GetModelType(gomock.Any()).Return(model.CAAS, nil).Times(2)
	st.EXPECT().GetControllerServiceAddresses(gomock.Any()).Return(candidates, nil).Times(2)
	svc := NewService(st, loggertesting.WrapCheckLog(c))
	clients, err := svc.GetControllerClientAddresses(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(clients.Shared, tc.HasLen, 0)
	agents, err := svc.GetControllerAgentAddresses(c.Context(), nil, "")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(agents.Shared, tc.HasLen, 0)
}

func (*controllerNetworkSuite) TestDiscoveryReadFailureDiscardsPartialSelection(c *tc.C) {
	st := NewMockState(gomock.NewController(c))
	st.EXPECT().GetModelType(gomock.Any()).Return(model.IAAS, nil).Times(2)
	st.EXPECT().GetControllerUnitNetwork(gomock.Any(), "controller/0").Return(domainnetwork.ControllerAPIAddresses{
		controllerCandidate("10.0.0.1", network.ScopeCloudLocal, network.AlphaSpaceId, domainnetwork.DeviceTypeEthernet),
	}, nil).Times(2)
	st.EXPECT().GetControllerUnitNetwork(gomock.Any(), "controller/1").Return(nil, applicationerrors.UnitIsDead).Times(2)
	svc := NewService(st, loggertesting.WrapCheckLog(c))
	names := []unit.Name{"controller/0", "controller/1"}
	clients, err := svc.GetControllerClientAddresses(c.Context(), names)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitIsDead)
	c.Check(clients, tc.DeepEquals, domainnetwork.ControllerAddressSelection{})
	agents, err := svc.GetControllerAgentAddresses(c.Context(), names, "")
	c.Check(err, tc.ErrorIs, applicationerrors.UnitIsDead)
	c.Check(agents, tc.DeepEquals, domainnetwork.ControllerAddressSelection{})
}

func (*controllerNetworkSuite) TestServiceReadFailureIsNotEmptySelection(c *tc.C) {
	st := NewMockState(gomock.NewController(c))
	failure := errors.New("service addresses unavailable")
	st.EXPECT().GetModelType(gomock.Any()).Return(model.CAAS, nil).Times(2)
	st.EXPECT().GetControllerServiceAddresses(gomock.Any()).Return(nil, failure).Times(2)
	svc := NewService(st, loggertesting.WrapCheckLog(c))
	clients, err := svc.GetControllerClientAddresses(c.Context(), nil)
	c.Check(err, tc.ErrorIs, failure)
	c.Check(clients, tc.DeepEquals, domainnetwork.ControllerAddressSelection{})
	agents, err := svc.GetControllerAgentAddresses(c.Context(), nil, "")
	c.Check(err, tc.ErrorIs, failure)
	c.Check(agents, tc.DeepEquals, domainnetwork.ControllerAddressSelection{})
}

func (*controllerNetworkSuite) TestDiscoveryValidatesMembership(c *tc.C) {
	st := NewMockState(gomock.NewController(c))
	svc := NewService(st, loggertesting.WrapCheckLog(c))
	for _, test := range []struct {
		name unit.Name
		err  error
	}{
		{"invalid", unit.InvalidUnitName},
		{"postgresql/0", applicationerrors.UnitNotFound},
	} {
		_, err := svc.GetControllerClientAddresses(c.Context(), []unit.Name{test.name})
		c.Check(err, tc.ErrorIs, test.err)
		_, err = svc.GetControllerAgentAddresses(c.Context(), []unit.Name{test.name}, "")
		c.Check(err, tc.ErrorIs, test.err)
	}
}

func controllerCandidate(value string, scope network.Scope, space network.SpaceUUID, device domainnetwork.DeviceType) domainnetwork.ControllerAPIAddress {
	return domainnetwork.ControllerAPIAddress{
		SpaceAddress: network.SpaceAddress{MachineAddress: network.NewMachineAddress(value, network.WithScope(scope)), SpaceID: space},
		DeviceType:   device,
	}
}
