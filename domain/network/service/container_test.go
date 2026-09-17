// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/containermanager"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/domain/network/internal"
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
	c.Assert(nics, tc.HasLen, 0)
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
	c.Assert(nics, tc.HasLen, 0)
}

func (s *containerSuite) TestDevicesForGuestBridgeFoundNoContainerAddresses(c *tc.C) {
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

	nics, err := s.svc.DevicesForGuest(ctx, s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	nic := nics[0]

	c.Check(nic.Name, tc.Equals, "eth0")
	c.Check(nic.MACAddress, tc.NotNil)
	c.Check(nic.Type, tc.Equals, corenetwork.EthernetDevice)
	c.Check(nic.ParentDeviceName, tc.Equals, bridgeName)
	c.Check(nic.IsEnabled, tc.IsTrue)
	c.Check(nic.IsAutoStart, tc.IsTrue)

	c.Assert(nic.Addrs, tc.HasLen, 1)
	c.Check(nic.Addrs[0].AddressValue, tc.Equals, cidr)
	c.Check(nic.Addrs[0].ConfigType, tc.Equals, corenetwork.ConfigDHCP)
}

func (s *containerSuite) TestDevicesForGuestBridgeFoundContainerAddresses(c *tc.C) {
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

	nics, err := s.svc.DevicesForGuest(ctx, s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	nic := nics[0]

	c.Check(nic.Name, tc.Equals, "eth0")
	c.Check(nic.MACAddress, tc.NotNil)
	c.Check(nic.Type, tc.Equals, corenetwork.EthernetDevice)
	c.Check(nic.ParentDeviceName, tc.Equals, bridgeName)
	c.Check(nic.IsEnabled, tc.IsTrue)
	c.Check(nic.IsAutoStart, tc.IsTrue)

	c.Assert(nic.Addrs, tc.HasLen, 1)
	c.Check(nic.Addrs[0].AddressValue, tc.Equals, cidr)
	c.Check(nic.Addrs[0].ConfigType, tc.Equals, corenetwork.ConfigStatic)
}

func (s *containerSuite) TestDevicesForGuestNoBridgeFoundError(c *tc.C) {
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

	_, err := s.svc.DevicesForGuest(ctx, s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIs, errors.SpaceRequirementsUnsatisfiable)
}

func (s *containerSuite) TestDevicesForGuestLocalMethodDefaultBridgeNoSpace(c *tc.C) {
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

	nics, err := s.svc.DevicesForGuest(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	nic := nics[0]

	c.Check(nic.Name, tc.Equals, "eth0")
	c.Check(nic.MACAddress, tc.NotNil)
	c.Check(nic.Type, tc.Equals, corenetwork.EthernetDevice)
	c.Check(nic.ParentDeviceName, tc.Equals, internalnetwork.DefaultLXDBridge)
	c.Check(nic.MTU, tc.DeepEquals, &bridgeMTU)
	c.Check(nic.IsEnabled, tc.IsTrue)
	c.Check(nic.IsAutoStart, tc.IsTrue)

	c.Assert(nic.Addrs, tc.HasLen, 1)
	c.Check(nic.Addrs[0].AddressValue, tc.Equals, "")
	c.Check(nic.Addrs[0].ConfigType, tc.Equals, corenetwork.ConfigDHCP)
}

func (s *containerSuite) TestDevicesForGuestLocalMethodDefaultBridgeNotObserved(c *tc.C) {
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

	_, err := s.svc.DevicesForGuest(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIs, errors.SpaceRequirementsUnsatisfiable)
}

func (s *containerSuite) TestDevicesForGuestLocalMethodMultipleSpacesSingleBridge(c *tc.C) {
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

	nics, err := s.svc.DevicesForGuest(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(nics, tc.HasLen, 1)
	nic := nics[0]

	c.Check(nic.ParentDeviceName, tc.Equals, internalnetwork.DefaultLXDBridge)
	c.Assert(nic.Addrs, tc.HasLen, 1)
	c.Check(nic.Addrs[0].AddressValue, tc.Equals, "")
	c.Check(nic.Addrs[0].ConfigType, tc.Equals, corenetwork.ConfigDHCP)
}

func (s *containerSuite) TestDevicesForGuestLocalMethodMixedBridges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupServiceAndMachines(c)

	// One space is served by an existing bridge; the other falls back to
	// the default LXD bridge with DHCP.
	s.expectContainerNetworking(c,
		[]internal.SpaceName{
			{UUID: "one-space-uuid", Name: "one-space"},
			{UUID: "two-space-uuid", Name: "two-space"},
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

	nics, err := s.svc.DevicesForGuest(c.Context(), s.hostUUID, s.guestUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Devices are appended in the order of the required spaces: the device
	// for the bridged space first, then the device parented to the default
	// LXD bridge.
	c.Assert(nics, tc.HasLen, 2)
	bridgedDev, localDev := nics[0], nics[1]

	c.Check(bridgedDev.ParentDeviceName, tc.Equals, "br-eth0")
	c.Assert(bridgedDev.Addrs, tc.HasLen, 1)
	c.Check(bridgedDev.Addrs[0].AddressValue, tc.Equals, "10.10.10.0/24")
	c.Check(bridgedDev.Addrs[0].ConfigType, tc.Equals, corenetwork.ConfigDHCP)

	c.Check(localDev.ParentDeviceName, tc.Equals, internalnetwork.DefaultLXDBridge)
	c.Assert(localDev.Addrs, tc.HasLen, 1)
	c.Check(localDev.Addrs[0].AddressValue, tc.Equals, "")
	c.Check(localDev.Addrs[0].ConfigType, tc.Equals, corenetwork.ConfigDHCP)
}

func (s *containerSuite) TestAllocateContainerAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.setupService(c)

	// Arrange
	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(true)
	hostInstance := instance.Id("juju-42fbd8-8")
	containerName := "8/lxd/7"
	input := corenetwork.InterfaceInfos{}
	expected := corenetwork.InterfaceInfos{
		{DeviceIndex: 7},
	}
	s.providerWithNetworking.EXPECT().AllocateContainerAddresses(
		gomock.Any(), hostInstance, containerName, input).Return(expected, nil)

	// Act
	obtainedInfo, err := s.svc.AllocateContainerAddresses(c.Context(), hostInstance, containerName, input)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(obtainedInfo, tc.DeepEquals, expected)
}

func (s *containerSuite) TestAllocateContainerAddressesNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.setupService(c)

	// Arrange
	hostInstance := instance.Id("juju-42fbd8-8")
	containerName := "8/lxd/7"
	input := corenetwork.InterfaceInfos{}
	s.providerWithNetworking.EXPECT().SupportsContainerAddresses().Return(false)

	// Act
	_, err := s.svc.AllocateContainerAddresses(c.Context(), hostInstance, containerName, input)

	// Assert
	c.Assert(err, tc.ErrorIs, errors.ContainerAddressesNotSupported)
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

	s.nodeUUID = "net-node-uuid"

	s.svc = NewProviderService(
		s.st,
		func(ctx context.Context) (ProviderWithNetworking, error) { return s.providerWithNetworking, nil },
		nil, // No provider with zones needed for this suite.
		loggertesting.WrapCheckLog(c),
	)
	c.Cleanup(func() { s.svc = nil })
}

func (s *containerSuite) setupService(c *tc.C) {
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
