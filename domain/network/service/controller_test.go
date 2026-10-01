// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
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
	// addresses to the external targets.
	targets, err := svc.GetControllerTargetAddresses(c.Context(), "controller/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(targets, tc.DeepEquals, network.SpaceAddresses{public.SpaceAddress})
	peers, err = svc.GetControllerPeerAddresses(c.Context(), "controller/0", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(peers, tc.DeepEquals, network.SpaceAddresses{public.SpaceAddress})
}

func (*controllerNetworkSuite) TestTargetAddressesRetainEligibleManagementAddresses(c *tc.C) {
	st := NewMockState(gomock.NewController(c))
	public := controllerCandidate("192.0.2.1", network.ScopePublic, "public", domainnetwork.DeviceTypeEthernet)
	management := controllerCandidate("10.0.0.1", network.ScopeCloudLocal, "management", domainnetwork.DeviceTypeEthernet)
	st.EXPECT().GetControllerUnitNetwork(gomock.Any(), "controller/0").Return(domainnetwork.ControllerAPIAddresses{public, management}, nil)
	st.EXPECT().GetModelType(gomock.Any()).Return(model.IAAS, nil)
	// No space lookup is required for either eligible address.
	addresses, err := NewService(st, loggertesting.WrapCheckLog(c)).GetControllerTargetAddresses(c.Context(), "controller/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses, tc.DeepEquals, network.SpaceAddresses{management.SpaceAddress, public.SpaceAddress})
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

func (*controllerNetworkSuite) TestPodIPAndDNS(c *tc.C) {
	st := NewMockState(gomock.NewController(c))
	ip := controllerCandidate("10.0.0.1", network.ScopeMachineLocal, network.AlphaSpaceId, domainnetwork.DeviceTypeEthernet)
	dns := controllerCandidate("controller-0.example.test", network.ScopeCloudLocal, network.AlphaSpaceId, domainnetwork.DeviceTypeUnknown)
	st.EXPECT().GetControllerUnitNetwork(gomock.Any(), "controller/0").Return(domainnetwork.ControllerAPIAddresses{ip, dns}, nil).Times(2)
	st.EXPECT().GetModelType(gomock.Any()).Return(model.CAAS, nil).Times(2)
	svc := NewService(st, loggertesting.WrapCheckLog(c))
	addresses, err := svc.GetControllerPeerAddresses(c.Context(), "controller/0", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses, tc.DeepEquals, network.SpaceAddresses{dns.SpaceAddress, ip.SpaceAddress})
	_, err = svc.GetControllerTargetAddresses(c.Context(), "controller/0")
	c.Check(err, tc.ErrorIs, coreerrors.NotSupported)
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
	st.EXPECT().GetControllerUnitNetwork(gomock.Any(), "controller/0").Return(domainnetwork.ControllerAPIAddresses{address}, nil).Times(2)
	failure := errors.New("model type unavailable")
	st.EXPECT().GetModelType(gomock.Any()).Return(model.ModelType(""), failure).Times(2)
	svc := NewService(st, loggertesting.WrapCheckLog(c))
	addresses, err := svc.GetControllerPeerAddresses(c.Context(), "controller/0", "")
	c.Check(err, tc.ErrorIs, failure)
	c.Check(addresses, tc.IsNil)
	addresses, err = svc.GetControllerTargetAddresses(c.Context(), "controller/0")
	c.Check(err, tc.ErrorIs, failure)
	c.Check(addresses, tc.IsNil)
}

func controllerCandidate(value string, scope network.Scope, space network.SpaceUUID, device domainnetwork.DeviceType) domainnetwork.ControllerAPIAddress {
	return domainnetwork.ControllerAPIAddress{
		SpaceAddress: network.SpaceAddress{MachineAddress: network.NewMachineAddress(value, network.WithScope(scope)), SpaceID: space},
		DeviceType:   device,
	}
}
