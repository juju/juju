// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/containermanager"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/domain/network/internal"
	internalerrors "github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalnetwork "github.com/juju/juju/internal/network"
	"github.com/juju/juju/internal/testhelpers"
)

type containerSuite struct {
	testhelpers.IsolationSuite

	st                     *MockState
	providerWithNetworking *MockProviderWithNetworking

	hostUUID  machine.UUID
	guestUUID machine.UUID
	guestName machine.Name
	nodeUUID  string

	svc *ProviderService
}

func TestContainerSuite(t *testing.T) {
	tc.Run(t, &containerSuite{})
}

func (s *containerSuite) TestDevicesToBridgeConflictingSpaceConstraints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(
		[]internal.SpaceName{},
		[]internal.SpaceName{{
			UUID: "negative-space-uuid",
			Name: "negative-space",
		}},
		nil,
	)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(
		[]internal.SpaceName{{
			UUID: "negative-space-uuid",
			Name: "negative-space",
		}},
		nil,
	)

	_, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIs, errors.SpaceRequirementConflict)
}

func (s *containerSuite) TestDevicesToBridgeSpaceReqsSatisfiedByBridge(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	spaceUUID := "positive-space-uuid"
	spaces := []internal.SpaceName{{
		UUID: spaceUUID,
		Name: "positive-space",
	}}

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(spaces, nil, nil)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(nil, nil)
	exp.GetMachineNetNodeUUID(c.Context(), s.hostUUID.String()).Return(s.nodeUUID, nil)
	// A bridge in the space means that connectivity is satisfied.
	exp.NICsInSpaces(c.Context(), s.nodeUUID).Return(map[string][]network.NetInterface{
		spaceUUID: {{
			Name: "br-not-default-lxd",
			Type: corenetwork.BridgeDevice,
		}},
	}, nil)
	exp.GetContainerNetworkingMethod(c.Context()).Return(containermanager.NetworkingMethodProvider.String(), nil)

	nics, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(nics, tc.HasLen, 0)
}

func (s *containerSuite) TestDevicesToBridgeSpaceReqsSatisfiedByOVS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	spaceUUID := "positive-space-uuid"
	spaces := []internal.SpaceName{{
		UUID: spaceUUID,
		Name: "positive-space",
	}}

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(spaces, nil, nil)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(nil, nil)
	exp.GetMachineNetNodeUUID(c.Context(), s.hostUUID.String()).Return(s.nodeUUID, nil)
	// An OVS device in the space means that connectivity is satisfied.
	exp.NICsInSpaces(c.Context(), s.nodeUUID).Return(map[string][]network.NetInterface{
		spaceUUID: {{
			Name:            "ovs-1",
			Type:            corenetwork.EthernetDevice,
			VirtualPortType: corenetwork.OvsPort,
		}},
	}, nil)
	exp.GetContainerNetworkingMethod(c.Context()).Return(containermanager.NetworkingMethodProvider.String(), nil)

	nics, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(nics, tc.HasLen, 0)
}

func (s *containerSuite) TestDevicesToBridgeSpaceReqsUnsatisfiable(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	spaceUUID := "positive-space-uuid"
	spaces := []internal.SpaceName{{
		UUID: spaceUUID,
		Name: "positive-space",
	}}

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(spaces, nil, nil)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(nil, nil)
	exp.GetMachineNetNodeUUID(c.Context(), s.hostUUID.String()).Return(s.nodeUUID, nil)
	// No devices in the space means the host can't
	// accommodate the guest space requirements.
	exp.NICsInSpaces(c.Context(), s.nodeUUID).Return(nil, nil)
	exp.GetContainerNetworkingMethod(c.Context()).Return(containermanager.NetworkingMethodProvider.String(), nil)

	_, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIs, errors.SpaceRequirementsUnsatisfiable)
}

func (s *containerSuite) TestDevicesToBridgeDeviceSatisfiesSpaces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	spaceUUID := "positive-space-uuid"
	spaces := []internal.SpaceName{{
		UUID: spaceUUID,
		Name: "positive-space",
	}}

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(spaces, nil, nil)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(nil, nil)
	exp.GetMachineNetNodeUUID(c.Context(), s.hostUUID.String()).Return(s.nodeUUID, nil)
	// An ethernet device in the space can be bridged to satisfy the guest's
	// requirements. Loopback devices are not considered, nor are devices
	// not connected to the space(s) we need.
	exp.NICsInSpaces(c.Context(), s.nodeUUID).Return(map[string][]network.NetInterface{
		"another-space-uuid": {
			{
				Name: "br-not-default-lxd",
				Type: corenetwork.BridgeDevice,
			},
		},
		spaceUUID: {
			{
				Name: "lo",
				Type: corenetwork.LoopbackDevice,
			},
			{
				Name:       "eth0",
				Type:       corenetwork.EthernetDevice,
				MACAddress: new("some-mac-address"),
			},
		},
	}, nil)
	exp.GetContainerNetworkingMethod(c.Context()).Return(containermanager.NetworkingMethodProvider.String(), nil)

	nics, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	c.Check(nics[0], tc.DeepEquals, network.DeviceToBridge{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
		MACAddress: "some-mac-address",
	})
}

func (s *containerSuite) TestDevicesToBridgeMultipleReqsMultipleDevsSatisfySpaces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	spaceOne := "one-space-uuid"
	consSpaces := []internal.SpaceName{{
		UUID: spaceOne,
		Name: "one-space",
	}}

	spaceTwo := "two-space-uuid"
	boundSpaces := []internal.SpaceName{{
		UUID: spaceTwo,
		Name: "two-space",
	}}

	negativeSpaces := []internal.SpaceName{{
		UUID: "negative-space-uuid",
		Name: "negative-space",
	}}

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(consSpaces, negativeSpaces, nil)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(boundSpaces, nil)
	exp.GetMachineNetNodeUUID(c.Context(), s.hostUUID.String()).Return(s.nodeUUID, nil)
	// An ethernet device in the space can be bridged to satisfy one space
	// requirement and an existing bridge satisfies the other.
	exp.NICsInSpaces(c.Context(), s.nodeUUID).Return(map[string][]network.NetInterface{
		spaceOne: {{
			Name:       "eth0",
			Type:       corenetwork.EthernetDevice,
			MACAddress: new("some-mac-address"),
		}},
		spaceTwo: {{
			Name: "br-not-default-lxd",
			Type: corenetwork.BridgeDevice,
		}},
	}, nil)
	exp.GetContainerNetworkingMethod(c.Context()).Return(containermanager.NetworkingMethodProvider.String(), nil)

	nics, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	c.Check(nics[0], tc.DeepEquals, network.DeviceToBridge{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
		MACAddress: "some-mac-address",
	})
}

func (s *containerSuite) TestDevicesToBridgeLocalBridgeReqsUnsatisfiable(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	spaceUUID := "positive-space-uuid"
	spaces := []internal.SpaceName{{
		UUID: spaceUUID,
		Name: "positive-space",
	}}

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(spaces, nil, nil)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(nil, nil)
	exp.GetMachineNetNodeUUID(c.Context(), s.hostUUID.String()).Return(s.nodeUUID, nil)
	// The default LXD bridge cannot satisfy space requirements when the
	// container networking method is "provider".
	exp.NICsInSpaces(c.Context(), s.nodeUUID).Return(map[string][]network.NetInterface{
		spaceUUID: {{
			Name: internalnetwork.DefaultLXDBridge,
			Type: corenetwork.BridgeDevice,
		}},
	}, nil)
	exp.GetContainerNetworkingMethod(c.Context()).Return(containermanager.NetworkingMethodProvider.String(), nil)

	_, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIs, errors.SpaceRequirementsUnsatisfiable)
}

func (s *containerSuite) TestDevicesToBridgeLocalMethodDefaultBridgeNoSpace(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// The default LXD bridge is not associated with any space, because its
	// subnet is not registered with Juju. With the "local" container
	// networking method it still satisfies the space requirements, so the
	// device in the required space must not be selected for bridging.
	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"": {{
				Name: internalnetwork.DefaultLXDBridge,
				Type: corenetwork.BridgeDevice,
			}},
			"positive-space-uuid": {{
				Name:       "eth0",
				Type:       corenetwork.EthernetDevice,
				MACAddress: new("some-mac-address"),
			}},
		},
		containermanager.NetworkingMethodLocal.String(),
	)

	nics, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(nics, tc.HasLen, 0)
}

func (s *containerSuite) TestDevicesToBridgeLocalMethodDefaultBridgeNotObserved(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// There is a bridgeable device in the space, but the default LXD bridge
	// has not been observed on the host. Host devices are never bridged when
	// using local networking, so the requirements are unsatisfiable.
	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"positive-space-uuid": {{
				Name:       "eth0",
				Type:       corenetwork.EthernetDevice,
				MACAddress: new("some-mac-address"),
			}},
		},
		containermanager.NetworkingMethodLocal.String(),
	)

	_, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIs, errors.SpaceRequirementsUnsatisfiable)
}

func (s *containerSuite) TestDevicesToBridgeLocalMethodDefaultBridgeInSpace(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// The default LXD bridge satisfies the space requirement when the
	// container networking method is "local".
	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"positive-space-uuid": {{
				Name: internalnetwork.DefaultLXDBridge,
				Type: corenetwork.BridgeDevice,
			}},
		},
		containermanager.NetworkingMethodLocal.String(),
	)

	nics, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(nics, tc.HasLen, 0)
}

func (s *containerSuite) TestDevicesToBridgeLocalMethodInSpaceBridgeNoDefaultBridge(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// The default LXD bridge has not been observed on the host, but a
	// bridge in the required space satisfies the requirement, so no host
	// devices are selected for bridging and no error is reported.
	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"positive-space-uuid": {{
				Name: "br-eth0",
				Type: corenetwork.BridgeDevice,
			}},
		},
		containermanager.NetworkingMethodLocal.String(),
	)

	nics, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(nics, tc.HasLen, 0)
}

func (s *containerSuite) TestDevicesToBridgeLocalMethodMixedInSpaceAndUnsatisfiableSpaces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// One required space has an in-space bridge; the other has no observed
	// devices, and the default LXD bridge has not been observed on the
	// host. The error must name only the unsatisfiable space: it is the
	// message operators see while waiting for lxdbr0 to be reported, and
	// the space satisfied by its in-space bridge is not actionable.
	s.expectContainerNetworking(c,
		[]internal.SpaceName{
			{UUID: "alpha-space-uuid", Name: "alpha-space"},
			{UUID: "beta-space-uuid", Name: "beta-space"},
		},
		map[string][]network.NetInterface{
			"alpha-space-uuid": {{
				Name: "br-eth0",
				Type: corenetwork.BridgeDevice,
			}},
			"beta-space-uuid": nil,
		},
		containermanager.NetworkingMethodLocal.String(),
	)

	_, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIs, errors.SpaceRequirementsUnsatisfiable)
	c.Check(err, tc.ErrorMatches, ".*space\\(s\\) \\[beta-space-uuid\\]")
	c.Check(err, tc.Not(tc.ErrorMatches), ".*alpha-space-uuid.*")
}

func (s *containerSuite) TestDevicesToBridgeLocalMethodNegativeConstraintNotEnforced(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// The default LXD bridge is observed in a negatively constrained
	// space. Local networking does not enforce negative space
	// constraints: the bridge satisfies the guest's positive requirement
	// regardless, and no host devices are selected for bridging.
	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		[]internal.SpaceName{{UUID: "negative-space-uuid", Name: "negative-space"}},
		nil,
	)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(nil, nil)
	exp.GetMachineNetNodeUUID(c.Context(), s.hostUUID.String()).Return(s.nodeUUID, nil)
	exp.NICsInSpaces(c.Context(), s.nodeUUID).Return(map[string][]network.NetInterface{
		"negative-space-uuid": {{
			Name: internalnetwork.DefaultLXDBridge,
			Type: corenetwork.BridgeDevice,
		}},
	}, nil)
	exp.GetContainerNetworkingMethod(c.Context()).Return(containermanager.NetworkingMethodLocal.String(), nil)

	nics, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(nics, tc.HasLen, 0)
}

func (s *containerSuite) TestDevicesToBridgeNetworkingMethodError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	methodErr := internalerrors.New("boom")

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(nil, nil, nil)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(nil, nil)
	exp.GetMachineNetNodeUUID(c.Context(), s.hostUUID.String()).Return(s.nodeUUID, nil)
	exp.NICsInSpaces(c.Context(), s.nodeUUID).Return(nil, nil)
	exp.GetContainerNetworkingMethod(c.Context()).Return("", methodErr)

	_, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Check(err, tc.ErrorIs, methodErr)
}

// TestDevicesToBridgeAutoMethodNoContainerAddressSupport covers the
// default configuration: the unset "auto" method resolved against a
// provider that cannot allocate container addresses (e.g. EC2). The model
// uses local networking, so the default LXD bridge satisfies all space
// requirements and no host devices are selected for bridging.
func (s *containerSuite) TestDevicesToBridgeAutoMethodNoContainerAddressSupport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// The default LXD bridge is observed, but its subnet is not
	// registered with Juju, so it is reported in no space.
	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"": {{
				Name: internalnetwork.DefaultLXDBridge,
				Type: corenetwork.BridgeDevice,
			}},
			"positive-space-uuid": {{
				Name: "eth0",
				Type: corenetwork.EthernetDevice,
			}},
		},
		"",
	)
	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(false)

	toBridge, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(toBridge, tc.HasLen, 0)
}

// TestDevicesToBridgeAutoMethodNoSupportDoesNotBridgeHostDevice ensures
// that with the unset "auto" method on a provider without container
// address support, a missing default LXD bridge is never compensated for
// by bridging host devices: the space requirements are reported
// unsatisfiable so the caller retries, giving the host agent time to
// report the bridge.
func (s *containerSuite) TestDevicesToBridgeAutoMethodNoSupportDoesNotBridgeHostDevice(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"positive-space-uuid": {{
				Name: "eth0",
				Type: corenetwork.EthernetDevice,
			}},
		},
		"",
	)
	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(false)

	_, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIs, errors.SpaceRequirementsUnsatisfiable)
}

// TestDevicesToBridgeAutoMethodContainerAddressSupport ensures that the
// unset "auto" method on a provider that supports allocating container
// addresses uses provider networking, bridging host devices as required.
func (s *containerSuite) TestDevicesToBridgeAutoMethodContainerAddressSupport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"positive-space-uuid": {{
				Name:       "eth0",
				Type:       corenetwork.EthernetDevice,
				MACAddress: new("aa:bb:cc:dd:ee:ff"),
			}},
		},
		"",
	)
	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(true)

	toBridge, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(toBridge, tc.HasLen, 1)
	c.Check(toBridge[0].DeviceName, tc.Equals, "eth0")
	c.Check(toBridge[0].BridgeName, tc.Equals, "br-eth0")
	c.Check(toBridge[0].MACAddress, tc.Equals, "aa:bb:cc:dd:ee:ff")
}

// TestDevicesToBridgeAutoMethodProviderNotSupported ensures the unset
// "auto" method resolves to local networking when the provider does not
// implement the networking capability at all.
func (s *containerSuite) TestDevicesToBridgeAutoMethodProviderNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// Replace the provider getter with one whose provider does not
	// support the networking capability.
	s.svc = NewProviderService(
		s.st,
		func(ctx context.Context) (ProviderWithNetworking, error) {
			return nil, internalerrors.Errorf(
				"provider type %T %w", ProviderWithNetworking(nil), coreerrors.NotSupported,
			)
		},
		nil, // No provider with zones needed for this suite.
		loggertesting.WrapCheckLog(c),
	)

	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"": {{
				Name: internalnetwork.DefaultLXDBridge,
				Type: corenetwork.BridgeDevice,
			}},
		},
		"",
	)

	toBridge, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(toBridge, tc.HasLen, 0)
}

// TestDevicesToBridgeNetworkingMethodNotValid ensures an unknown raw
// model config value is rejected rather than silently treated as
// provider networking.
func (s *containerSuite) TestDevicesToBridgeNetworkingMethodNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	s.expectContainerNetworking(c, nil, nil, "bogus")

	_, err := s.svc.DevicesToBridge(c.Context(), s.hostUUID, s.guestUUID)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *containerSuite) TestNetworkConfigForGuestBridgeFoundNoContainerAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	ctx := c.Context()

	spaceUUID := "positive-space-uuid"
	spaces := []internal.SpaceName{{
		UUID: spaceUUID,
		Name: "positive-space",
	}}
	bridgeName := "br-eth0"
	cidr := "10.10.10.0/24"

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(ctx, s.guestUUID.String()).Return(spaces, nil, nil)
	exp.GetMachineAppBindings(ctx, s.guestUUID.String()).Return(nil, nil)
	exp.GetMachineNetNodeUUID(ctx, s.hostUUID.String()).Return(s.nodeUUID, nil)
	// A bridge in the space means that connectivity is satisfied.
	exp.NICsInSpaces(ctx, s.nodeUUID).Return(map[string][]network.NetInterface{
		spaceUUID: {
			{
				Name: bridgeName,
				Type: corenetwork.BridgeDevice,
			},
		},
	}, nil)
	exp.GetContainerNetworkingMethod(ctx).Return(containermanager.NetworkingMethodProvider.String(), nil)
	exp.GetSubnetCIDRForDevice(ctx, s.nodeUUID, bridgeName, spaceUUID).Return(cidr, nil)

	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(false)

	nics, err := s.svc.NetworkConfigForGuest(ctx, s.hostUUID, s.guestUUID, s.guestName)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	nic := nics[0]

	c.Check(nic.InterfaceName, tc.Equals, "eth0")
	c.Check(nic.MACAddress, tc.Not(tc.Equals), "")
	c.Check(nic.InterfaceType, tc.Equals, corenetwork.EthernetDevice)
	c.Check(nic.ParentInterfaceName, tc.Equals, bridgeName)
	c.Check(nic.Disabled, tc.IsFalse)
	c.Check(nic.NoAutoStart, tc.IsFalse)

	c.Assert(nic.Addresses, tc.HasLen, 1)
	c.Check(nic.Addresses[0].CIDR, tc.Equals, cidr)
	c.Check(nic.ConfigType, tc.Equals, corenetwork.ConfigDHCP)
	c.Check(nic.Addresses[0].ConfigType, tc.Equals, corenetwork.ConfigDHCP)
}

func (s *containerSuite) TestNetworkConfigForGuestBridgeFoundContainerAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	ctx := c.Context()

	spaceUUID := "positive-space-uuid"
	spaces := []internal.SpaceName{{
		UUID: spaceUUID,
		Name: "positive-space",
	}}
	bridgeName := "br-eth0"
	cidr := "10.10.10.0/24"

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(ctx, s.guestUUID.String()).Return(spaces, nil, nil)
	exp.GetMachineAppBindings(ctx, s.guestUUID.String()).Return(nil, nil)
	exp.GetMachineNetNodeUUID(ctx, s.hostUUID.String()).Return(s.nodeUUID, nil)
	// A bridge in the space means that connectivity is satisfied.
	exp.NICsInSpaces(ctx, s.nodeUUID).Return(map[string][]network.NetInterface{
		spaceUUID: {
			{
				Name: bridgeName,
				Type: corenetwork.BridgeDevice,
			},
		},
	}, nil)
	exp.GetContainerNetworkingMethod(ctx).Return(containermanager.NetworkingMethodProvider.String(), nil)
	exp.GetSubnetCIDRForDevice(ctx, s.nodeUUID, bridgeName, spaceUUID).Return(cidr, nil)

	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(true)

	// The provider is asked to allocate an address for the device, which is
	// configured statically, on behalf of the guest and via the host's
	// cloud instance.
	exp.GetMachineInstanceID(ctx, s.hostUUID.String()).Return("host-instance-id", nil)

	allocated := corenetwork.InterfaceInfos{{
		InterfaceName: "eth0",
		Addresses: corenetwork.ProviderAddresses{{MachineAddress: corenetwork.MachineAddress{
			Value:      "10.10.10.6",
			CIDR:       cidr,
			ConfigType: corenetwork.ConfigStatic,
		}}},
	}}
	var prepared corenetwork.InterfaceInfos
	s.providerWithNetworking.EXPECT().AllocateContainerAddresses(
		gomock.Any(), instance.Id("host-instance-id"), s.guestName.String(), gomock.Any(),
	).DoAndReturn(func(
		_ context.Context, _ instance.Id, _ string, info corenetwork.InterfaceInfos,
	) (corenetwork.InterfaceInfos, error) {
		prepared = info
		return allocated, nil
	})

	nics, err := s.svc.NetworkConfigForGuest(ctx, s.hostUUID, s.guestUUID, s.guestName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(nics, tc.DeepEquals, allocated)

	c.Assert(prepared, tc.HasLen, 1)
	nic := prepared[0]

	c.Check(nic.InterfaceName, tc.Equals, "eth0")
	c.Check(nic.MACAddress, tc.Not(tc.Equals), "")
	c.Check(nic.InterfaceType, tc.Equals, corenetwork.EthernetDevice)
	c.Check(nic.ParentInterfaceName, tc.Equals, bridgeName)
	c.Check(nic.Disabled, tc.IsFalse)
	c.Check(nic.NoAutoStart, tc.IsFalse)

	c.Assert(nic.Addresses, tc.HasLen, 1)
	c.Check(nic.Addresses[0].CIDR, tc.Equals, cidr)
	c.Check(nic.ConfigType, tc.Equals, corenetwork.ConfigStatic)
}

// TestNetworkConfigForGuestHostNotProvisioned ensures that when the provider
// is to allocate addresses, a host without a cloud instance is reported.
func (s *containerSuite) TestNetworkConfigForGuestHostNotProvisioned(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"positive-space-uuid": {{
				Name: "br-eth0",
				Type: corenetwork.BridgeDevice,
			}},
		},
		containermanager.NetworkingMethodProvider.String(),
	)
	s.st.EXPECT().GetSubnetCIDRForDevice(c.Context(), s.nodeUUID, "br-eth0", "positive-space-uuid").
		Return("10.10.10.0/24", nil)
	s.st.EXPECT().GetMachineInstanceID(c.Context(), s.hostUUID.String()).
		Return("", errors.HostNotProvisioned)

	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(true)

	_, err := s.svc.NetworkConfigForGuest(c.Context(), s.hostUUID, s.guestUUID, s.guestName)
	c.Check(err, tc.ErrorIs, errors.HostNotProvisioned)
}

// TestNetworkConfigForGuestProviderAllocationError ensures that a failure
// to allocate addresses is reported.
func (s *containerSuite) TestNetworkConfigForGuestProviderAllocationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"positive-space-uuid": {{
				Name: "br-eth0",
				Type: corenetwork.BridgeDevice,
			}},
		},
		containermanager.NetworkingMethodProvider.String(),
	)
	s.st.EXPECT().GetSubnetCIDRForDevice(c.Context(), s.nodeUUID, "br-eth0", "positive-space-uuid").
		Return("10.10.10.0/24", nil)
	s.st.EXPECT().GetMachineInstanceID(c.Context(), s.hostUUID.String()).Return("host-instance-id", nil)

	allocErr := internalerrors.New("boom")
	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(true)
	s.providerWithNetworking.EXPECT().AllocateContainerAddresses(
		gomock.Any(), instance.Id("host-instance-id"), s.guestName.String(), gomock.Any(),
	).Return(nil, allocErr)

	_, err := s.svc.NetworkConfigForGuest(c.Context(), s.hostUUID, s.guestUUID, s.guestName)
	c.Check(err, tc.ErrorIs, allocErr)
}

// TestNetworkConfigForGuestLocalMethodAddressCapableNoProviderAllocation
// covers explicit "local" container networking on a provider that supports
// allocating container addresses. The device for the default LXD bridge is
// configured for DHCP, and the provider is not asked to allocate an address:
// neither the provider's allocation nor the host's cloud instance are
// consulted.
func (s *containerSuite) TestNetworkConfigForGuestLocalMethodAddressCapableNoProviderAllocation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"": {{
				Name: internalnetwork.DefaultLXDBridge,
				Type: corenetwork.BridgeDevice,
			}},
		},
		containermanager.NetworkingMethodLocal.String(),
	)

	// There are deliberately no expectations for GetMachineInstanceID or the
	// provider's AllocateContainerAddresses: unexpected calls fail the test.
	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(true)

	nics, err := s.svc.NetworkConfigForGuest(c.Context(), s.hostUUID, s.guestUUID, s.guestName)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	nic := nics[0]

	c.Check(nic.ParentInterfaceName, tc.Equals, internalnetwork.DefaultLXDBridge)
	c.Check(nic.ConfigType, tc.Equals, corenetwork.ConfigDHCP)
	c.Assert(nic.Addresses, tc.HasLen, 1)
	c.Check(nic.Addresses[0].CIDR, tc.Equals, "")
	c.Check(nic.Addresses[0].ConfigType, tc.Equals, corenetwork.ConfigDHCP)
}

// TestNetworkConfigForGuestNoDevicesNoProviderAllocation ensures that a guest
// with no space requirements, and so no devices, does not cause the provider
// to be asked to allocate addresses, even if it is able to.
func (s *containerSuite) TestNetworkConfigForGuestNoDevicesNoProviderAllocation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	s.expectContainerNetworking(c, nil, nil, containermanager.NetworkingMethodProvider.String())
	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(true)

	nics, err := s.svc.NetworkConfigForGuest(c.Context(), s.hostUUID, s.guestUUID, s.guestName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(nics, tc.HasLen, 0)
}

func (s *containerSuite) TestNetworkConfigForGuestNoBridgeFoundError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	ctx := c.Context()

	spaceUUID := "positive-space-uuid"
	spaces := []internal.SpaceName{{
		UUID: spaceUUID,
		Name: "positive-space",
	}}

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(ctx, s.guestUUID.String()).Return(spaces, nil, nil)
	exp.GetMachineAppBindings(ctx, s.guestUUID.String()).Return(nil, nil)
	exp.GetMachineNetNodeUUID(ctx, s.hostUUID.String()).Return(s.nodeUUID, nil)
	// No bridge means that the guest's space requirements are not satisfied.
	exp.NICsInSpaces(ctx, s.nodeUUID).Return(map[string][]network.NetInterface{
		spaceUUID: {
			{
				Name: "eth0",
				Type: corenetwork.EthernetDevice,
			},
		},
	}, nil)
	exp.GetContainerNetworkingMethod(ctx).Return(containermanager.NetworkingMethodProvider.String(), nil)

	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(true)

	_, err := s.svc.NetworkConfigForGuest(ctx, s.hostUUID, s.guestUUID, s.guestName)
	c.Assert(err, tc.ErrorIs, errors.SpaceRequirementsUnsatisfiable)
}

func (s *containerSuite) TestNetworkConfigForGuestLocalMethodDefaultBridgeNoSpace(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	bridgeMTU := int64(1500)

	// The default LXD bridge is not associated with any space, because its
	// subnet is not registered with Juju. With the "local" container
	// networking method it is used as the parent for the guest device, and
	// no CIDR lookup is performed for it.
	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"": {{
				Name: internalnetwork.DefaultLXDBridge,
				Type: corenetwork.BridgeDevice,
				MTU:  &bridgeMTU,
			}},
			"positive-space-uuid": {{
				Name: "eth0",
				Type: corenetwork.EthernetDevice,
			}},
		},
		containermanager.NetworkingMethodLocal.String(),
	)

	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(false)

	nics, err := s.svc.NetworkConfigForGuest(c.Context(), s.hostUUID, s.guestUUID, s.guestName)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	nic := nics[0]

	c.Check(nic.InterfaceName, tc.Equals, "eth0")
	c.Check(nic.MACAddress, tc.Not(tc.Equals), "")
	c.Check(nic.InterfaceType, tc.Equals, corenetwork.EthernetDevice)
	c.Check(nic.ParentInterfaceName, tc.Equals, internalnetwork.DefaultLXDBridge)
	c.Check(nic.MTU, tc.Equals, int(bridgeMTU))
	c.Check(nic.Disabled, tc.IsFalse)
	c.Check(nic.NoAutoStart, tc.IsFalse)

	c.Assert(nic.Addresses, tc.HasLen, 1)
	c.Check(nic.Addresses[0].CIDR, tc.Equals, "")
	c.Check(nic.ConfigType, tc.Equals, corenetwork.ConfigDHCP)
	c.Check(nic.Addresses[0].ConfigType, tc.Equals, corenetwork.ConfigDHCP)
}

func (s *containerSuite) TestNetworkConfigForGuestLocalMethodDefaultBridgeNotObserved(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// There is no in-space bridge and the default LXD bridge has not been
	// observed on the host, so the guest's space requirements are not
	// satisfied.
	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"positive-space-uuid": {{
				Name: "eth0",
				Type: corenetwork.EthernetDevice,
			}},
		},
		containermanager.NetworkingMethodLocal.String(),
	)

	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(false)

	_, err := s.svc.NetworkConfigForGuest(c.Context(), s.hostUUID, s.guestUUID, s.guestName)
	c.Assert(err, tc.ErrorIs, errors.SpaceRequirementsUnsatisfiable)
}

func (s *containerSuite) TestNetworkConfigForGuestLocalMethodMultipleSpacesSingleBridge(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// Both spaces lack in-space bridges. The default LXD bridge satisfies
	// them both, but only a single guest device is created for it.
	s.expectContainerNetworking(c,
		[]internal.SpaceName{
			{UUID: "one-space-uuid", Name: "one-space"},
			{UUID: "two-space-uuid", Name: "two-space"},
		},
		map[string][]network.NetInterface{
			"": {{
				Name: internalnetwork.DefaultLXDBridge,
				Type: corenetwork.BridgeDevice,
			}},
		},
		containermanager.NetworkingMethodLocal.String(),
	)

	// Even when the provider supports container addresses, the device on
	// the default LXD bridge uses DHCP, since the bridge subnet is not
	// registered with Juju.
	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(true)

	nics, err := s.svc.NetworkConfigForGuest(c.Context(), s.hostUUID, s.guestUUID, s.guestName)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	nic := nics[0]

	c.Check(nic.ParentInterfaceName, tc.Equals, internalnetwork.DefaultLXDBridge)
	c.Assert(nic.Addresses, tc.HasLen, 1)
	c.Check(nic.Addresses[0].CIDR, tc.Equals, "")
	c.Check(nic.ConfigType, tc.Equals, corenetwork.ConfigDHCP)
	c.Check(nic.Addresses[0].ConfigType, tc.Equals, corenetwork.ConfigDHCP)
}

func (s *containerSuite) TestNetworkConfigForGuestLocalMethodMixedBridges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// One space is served by an existing bridge; the other falls back to
	// the default LXD bridge with DHCP. The spaces are listed in reverse
	// name order, to verify that requirements are satisfied in sorted
	// space name order.
	s.expectContainerNetworking(c,
		[]internal.SpaceName{
			{UUID: "two-space-uuid", Name: "two-space"},
			{UUID: "one-space-uuid", Name: "one-space"},
		},
		map[string][]network.NetInterface{
			"one-space-uuid": {{
				Name: "br-eth0",
				Type: corenetwork.BridgeDevice,
			}},
			"": {{
				Name: internalnetwork.DefaultLXDBridge,
				Type: corenetwork.BridgeDevice,
			}},
		},
		containermanager.NetworkingMethodLocal.String(),
	)
	s.st.EXPECT().GetSubnetCIDRForDevice(c.Context(), s.nodeUUID, "br-eth0", "one-space-uuid").
		Return("10.10.10.0/24", nil)

	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(false)

	nics, err := s.svc.NetworkConfigForGuest(c.Context(), s.hostUUID, s.guestUUID, s.guestName)
	c.Assert(err, tc.ErrorIsNil)

	// Devices are appended in the order of the required spaces: the device
	// for the bridged space first, then the device parented to the default
	// LXD bridge.
	c.Assert(nics, tc.HasLen, 2)
	bridgedDev, localDev := nics[0], nics[1]

	c.Check(bridgedDev.ParentInterfaceName, tc.Equals, "br-eth0")
	c.Assert(bridgedDev.Addresses, tc.HasLen, 1)
	c.Check(bridgedDev.Addresses[0].CIDR, tc.Equals, "10.10.10.0/24")
	c.Check(bridgedDev.ConfigType, tc.Equals, corenetwork.ConfigDHCP)
	c.Check(bridgedDev.Addresses[0].ConfigType, tc.Equals, corenetwork.ConfigDHCP)

	c.Check(localDev.ParentInterfaceName, tc.Equals, internalnetwork.DefaultLXDBridge)
	c.Assert(localDev.Addresses, tc.HasLen, 1)
	c.Check(localDev.Addresses[0].CIDR, tc.Equals, "")
	c.Check(localDev.ConfigType, tc.Equals, corenetwork.ConfigDHCP)
	c.Check(localDev.Addresses[0].ConfigType, tc.Equals, corenetwork.ConfigDHCP)
}

func (s *containerSuite) TestNetworkConfigForGuestLocalMethodDefaultBridgeInSpace(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// The default LXD bridge is observed in the required space, because
	// its subnet is registered with Juju. The in-space bridge is used for
	// the guest device, and its registered CIDR is looked up, rather than
	// falling back to the local-networking bridge with DHCP.
	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"positive-space-uuid": {{
				Name: internalnetwork.DefaultLXDBridge,
				Type: corenetwork.BridgeDevice,
			}},
		},
		containermanager.NetworkingMethodLocal.String(),
	)
	s.st.EXPECT().GetSubnetCIDRForDevice(c.Context(), s.nodeUUID, internalnetwork.DefaultLXDBridge, "positive-space-uuid").
		Return("10.0.0.0/24", nil)

	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(false)

	nics, err := s.svc.NetworkConfigForGuest(c.Context(), s.hostUUID, s.guestUUID, s.guestName)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	nic := nics[0]

	c.Check(nic.InterfaceName, tc.Equals, "eth0")
	c.Check(nic.ParentInterfaceName, tc.Equals, internalnetwork.DefaultLXDBridge)
	c.Assert(nic.Addresses, tc.HasLen, 1)
	c.Check(nic.Addresses[0].CIDR, tc.Equals, "10.0.0.0/24")
	c.Check(nic.ConfigType, tc.Equals, corenetwork.ConfigDHCP)
	c.Check(nic.Addresses[0].ConfigType, tc.Equals, corenetwork.ConfigDHCP)
}

// TestNetworkConfigForGuestLocalMethodInSpaceBridgeAddressCapable ensures that
// with local networking on a provider that supports allocating container
// addresses, in-space bridge devices are configured for DHCP rather than
// static addressing: the container's networking is provided by the host
// machine, and the provider is not expected to allocate an address.
func (s *containerSuite) TestNetworkConfigForGuestLocalMethodInSpaceBridgeAddressCapable(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"positive-space-uuid": {{
				Name: "br-eth0",
				Type: corenetwork.BridgeDevice,
			}},
		},
		containermanager.NetworkingMethodLocal.String(),
	)
	s.st.EXPECT().GetSubnetCIDRForDevice(c.Context(), s.nodeUUID, "br-eth0", "positive-space-uuid").
		Return("10.10.10.0/24", nil)

	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(true)

	nics, err := s.svc.NetworkConfigForGuest(c.Context(), s.hostUUID, s.guestUUID, s.guestName)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	nic := nics[0]

	c.Check(nic.InterfaceName, tc.Equals, "eth0")
	c.Check(nic.ParentInterfaceName, tc.Equals, "br-eth0")
	c.Assert(nic.Addresses, tc.HasLen, 1)
	c.Check(nic.Addresses[0].CIDR, tc.Equals, "10.10.10.0/24")
	c.Check(nic.ConfigType, tc.Equals, corenetwork.ConfigDHCP)
	c.Check(nic.Addresses[0].ConfigType, tc.Equals, corenetwork.ConfigDHCP)
}

func (s *containerSuite) TestNetworkConfigForGuestLocalMethodNegativeConstraintNotEnforced(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// The default LXD bridge is observed in a negatively constrained
	// space, and the guest's positive requirement is on a space in which
	// no bridge is observed. Local networking does not enforce negative
	// space constraints: the default LXD bridge still satisfies the
	// positive requirement, parented with DHCP addressing.
	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		[]internal.SpaceName{{UUID: "negative-space-uuid", Name: "negative-space"}},
		nil,
	)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(nil, nil)
	exp.GetMachineNetNodeUUID(c.Context(), s.hostUUID.String()).Return(s.nodeUUID, nil)
	exp.NICsInSpaces(c.Context(), s.nodeUUID).Return(map[string][]network.NetInterface{
		"negative-space-uuid": {{
			Name: internalnetwork.DefaultLXDBridge,
			Type: corenetwork.BridgeDevice,
		}},
	}, nil)
	exp.GetContainerNetworkingMethod(c.Context()).Return(containermanager.NetworkingMethodLocal.String(), nil)

	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(false)

	nics, err := s.svc.NetworkConfigForGuest(c.Context(), s.hostUUID, s.guestUUID, s.guestName)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	nic := nics[0]

	c.Check(nic.InterfaceName, tc.Equals, "eth0")
	c.Check(nic.ParentInterfaceName, tc.Equals, internalnetwork.DefaultLXDBridge)
	c.Assert(nic.Addresses, tc.HasLen, 1)
	c.Check(nic.ConfigType, tc.Equals, corenetwork.ConfigDHCP)
	c.Check(nic.Addresses[0].ConfigType, tc.Equals, corenetwork.ConfigDHCP)
}

func (s *containerSuite) TestNetworkConfigForGuestLocalMethodInSpaceBridgeNoDefaultBridge(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// The default LXD bridge has not been observed on the host, but the
	// bridge in the required space is used for the guest device, and the
	// "not observed" error path is not taken.
	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"positive-space-uuid": {{
				Name: "br-eth0",
				Type: corenetwork.BridgeDevice,
			}},
		},
		containermanager.NetworkingMethodLocal.String(),
	)
	s.st.EXPECT().GetSubnetCIDRForDevice(c.Context(), s.nodeUUID, "br-eth0", "positive-space-uuid").
		Return("10.10.10.0/24", nil)

	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(false)

	nics, err := s.svc.NetworkConfigForGuest(c.Context(), s.hostUUID, s.guestUUID, s.guestName)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	nic := nics[0]

	c.Check(nic.ParentInterfaceName, tc.Equals, "br-eth0")
	c.Assert(nic.Addresses, tc.HasLen, 1)
	c.Check(nic.Addresses[0].CIDR, tc.Equals, "10.10.10.0/24")
}

func (s *containerSuite) TestNetworkConfigForGuestNetworkingMethodError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	methodErr := internalerrors.New("boom")

	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(nil, nil, nil)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(nil, nil)
	exp.GetMachineNetNodeUUID(c.Context(), s.hostUUID.String()).Return(s.nodeUUID, nil)
	exp.NICsInSpaces(c.Context(), s.nodeUUID).Return(nil, nil)
	exp.GetContainerNetworkingMethod(c.Context()).Return("", methodErr)

	_, err := s.svc.NetworkConfigForGuest(c.Context(), s.hostUUID, s.guestUUID, s.guestName)
	c.Check(err, tc.ErrorIs, methodErr)
}

// TestNetworkConfigForGuestAutoMethodNoContainerAddressSupport covers the
// default configuration: the unset "auto" method resolved against a
// provider that cannot allocate container addresses (e.g. EC2). The guest
// device is parented to the default LXD bridge with DHCP addressing.
func (s *containerSuite) TestNetworkConfigForGuestAutoMethodNoContainerAddressSupport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// The default LXD bridge is observed, but its subnet is not
	// registered with Juju, so it is reported in no space.
	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"": {{
				Name: internalnetwork.DefaultLXDBridge,
				Type: corenetwork.BridgeDevice,
			}},
		},
		"",
	)
	// The provider capability is consulted once: its result resolves the
	// auto method and selects the guest device addressing.
	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(false)

	nics, err := s.svc.NetworkConfigForGuest(c.Context(), s.hostUUID, s.guestUUID, s.guestName)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	nic := nics[0]

	c.Check(nic.InterfaceName, tc.Equals, "eth0")
	c.Check(nic.ParentInterfaceName, tc.Equals, internalnetwork.DefaultLXDBridge)
	c.Assert(nic.Addresses, tc.HasLen, 1)
	c.Check(nic.Addresses[0].CIDR, tc.Equals, "")
	c.Check(nic.ConfigType, tc.Equals, corenetwork.ConfigDHCP)
	c.Check(nic.Addresses[0].ConfigType, tc.Equals, corenetwork.ConfigDHCP)
}

// TestNetworkConfigForGuestAutoMethodProviderNotSupported ensures the unset
// "auto" method resolves to local networking when the provider does not
// implement the networking capability at all, and the guest device is
// parented to the default LXD bridge with DHCP addressing.
func (s *containerSuite) TestNetworkConfigForGuestAutoMethodProviderNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// Replace the provider getter with one whose provider does not
	// support the networking capability.
	s.svc = NewProviderService(
		s.st,
		func(ctx context.Context) (ProviderWithNetworking, error) {
			return nil, internalerrors.Errorf(
				"provider type %T %w", ProviderWithNetworking(nil), coreerrors.NotSupported,
			)
		},
		nil, // No provider with zones needed for this suite.
		loggertesting.WrapCheckLog(c),
	)

	s.expectContainerNetworking(c,
		[]internal.SpaceName{{UUID: "positive-space-uuid", Name: "positive-space"}},
		map[string][]network.NetInterface{
			"": {{
				Name: internalnetwork.DefaultLXDBridge,
				Type: corenetwork.BridgeDevice,
			}},
		},
		"",
	)

	nics, err := s.svc.NetworkConfigForGuest(c.Context(), s.hostUUID, s.guestUUID, s.guestName)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	nic := nics[0]

	c.Check(nic.ParentInterfaceName, tc.Equals, internalnetwork.DefaultLXDBridge)
	c.Assert(nic.Addresses, tc.HasLen, 1)
	c.Check(nic.ConfigType, tc.Equals, corenetwork.ConfigDHCP)
	c.Check(nic.Addresses[0].ConfigType, tc.Equals, corenetwork.ConfigDHCP)
}

func (s *containerSuite) TestNetworkConfigForGuestNoSpaces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// No space requirements: no guest devices are created.
	s.expectContainerNetworking(c,
		nil,
		nil,
		containermanager.NetworkingMethodLocal.String(),
	)

	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(false)

	nics, err := s.svc.NetworkConfigForGuest(c.Context(), s.hostUUID, s.guestUUID, s.guestName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(nics, tc.HasLen, 0)
}

func (s *containerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)
	s.providerWithNetworking = NewMockProviderWithNetworking(ctrl)

	c.Cleanup(func() {
		s.st = nil
		s.providerWithNetworking = nil
	})

	return ctrl
}

func (s *containerSuite) setupServiceAndMachines(c *tc.C) {
	var err error

	s.hostUUID, err = machine.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.guestUUID, err = machine.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.guestName = machine.Name("0/lxd/0")
	s.nodeUUID = "net-node-uuid"

	s.svc = NewProviderService(
		s.st,
		func(ctx context.Context) (ProviderWithNetworking, error) { return s.providerWithNetworking, nil },
		nil, // No provider with zones needed for this suite.
		loggertesting.WrapCheckLog(c),
	)
	c.Cleanup(func() { s.svc = nil })
}

// expectContainerNetworking arranges the state expectations common to
// container networking determinations: the guest's space requirements,
// the host's devices by space, and the container networking method.
func (s *containerSuite) expectContainerNetworking(
	c *tc.C,
	spaces []internal.SpaceName,
	nics map[string][]network.NetInterface,
	netMethod string,
) {
	exp := s.st.EXPECT()
	exp.GetMachineSpaceConstraints(c.Context(), s.guestUUID.String()).Return(spaces, nil, nil)
	exp.GetMachineAppBindings(c.Context(), s.guestUUID.String()).Return(nil, nil)
	exp.GetMachineNetNodeUUID(c.Context(), s.hostUUID.String()).Return(s.nodeUUID, nil)
	exp.NICsInSpaces(c.Context(), s.nodeUUID).Return(nics, nil)
	exp.GetContainerNetworkingMethod(c.Context()).Return(netMethod, nil)
}
