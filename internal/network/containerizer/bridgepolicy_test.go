// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerizer

import (
	"strconv"
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/containermanager"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	networktesting "github.com/juju/juju/core/network/testing"
)

type bridgePolicySuite struct {
	baseSuite

	containerNetworkingMethod containermanager.NetworkingMethod

	spaces corenetwork.SpaceInfos
	guest  *MockContainer
}

func TestBridgePolicySuite(t *testing.T) {
	tc.Run(t, &bridgePolicySuite{})
}

func (s *bridgePolicySuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)

	s.containerNetworkingMethod = "local"
	s.spaces = nil
}

func (s *bridgePolicySuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.guest = NewMockContainer(ctrl)
	s.guest.EXPECT().Id().Return("guest-id").AnyTimes()
	s.guest.EXPECT().ContainerType().Return(instance.LXD).AnyTimes()

	s.spaces = make(corenetwork.SpaceInfos, 4)
	s.spaces[0] = corenetwork.SpaceInfo{ID: corenetwork.AlphaSpaceId, Name: corenetwork.AlphaSpaceName}
	for _, space := range []string{"foo", "bar", "fizz"} {
		id := networktesting.GenSpaceUUID(c)
		s.spaces = append(s.spaces, corenetwork.SpaceInfo{ID: id, Name: corenetwork.SpaceName(space)})
	}
	return ctrl
}

func (s *bridgePolicySuite) setupTwoSpaces(c *tc.C) []corenetwork.SpaceUUID {
	id1 := networktesting.GenSpaceUUID(c)
	id2 := networktesting.GenSpaceUUID(c)
	s.spaces = append(s.spaces, corenetwork.SpaceInfo{
		ID:      id1,
		Name:    "somespace",
		Subnets: nil,
	})
	s.spaces = append(s.spaces, corenetwork.SpaceInfo{
		ID:      id2,
		Name:    "dmz",
		Subnets: nil,
	})
	return []corenetwork.SpaceUUID{id1, id2}
}

const (
	somespaceIndex = 0
	dmzIndex       = 1
)

func (s *bridgePolicySuite) setupMachineInTwoSpaces(c *tc.C, ctrl *gomock.Controller) []corenetwork.SpaceUUID {
	ids := s.setupTwoSpaces(c)
	s.expectNICAndBridgeWithIP(c, ctrl, "ens33", "br-ens33", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICAndBridgeWithIP(c, ctrl, "ens0p10", "br-ens0p10", ids[dmzIndex], "10.0.1.0/24")
	return ids
}

// expectAllDefaultDevices creates the loopback, lxcbr0, lxdbr0, and virbr0 devices
func (s *bridgePolicySuite) expectAllDefaultDevices(c *tc.C, ctrl *gomock.Controller) {
	// loopback
	s.expectLoopbackNIC(ctrl)
	// container.DefaultLxdBridge
	s.expectBridgeDeviceWithIP(c, ctrl, "lxdbr0", corenetwork.AlphaSpaceId, "10.0.0.0/24")
}

func (s *bridgePolicySuite) policy() *BridgePolicy {
	return &BridgePolicy{
		allSpaces:                 s.spaces,
		allSubnets:                s.baseSuite.allSubnets,
		containerNetworkingMethod: s.containerNetworkingMethod,
	}
}

func (s *bridgePolicySuite) TestDetermineContainerSpacesConstraints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.guest.EXPECT()
	exp.Constraints().Return(constraints.MustParse("spaces=foo,bar,^baz"), nil)

	obtained, err := s.policy().determineContainerSpaces(c.Context(), s.machine, s.guest)
	c.Assert(err, tc.ErrorIsNil)
	expected := corenetwork.SpaceInfos{
		*s.spaces.GetByName("foo"),
		*s.spaces.GetByName("bar"),
	}
	c.Check(obtained, tc.DeepEquals, expected)
}

func (s *bridgePolicySuite) TestDetermineContainerNoSpacesConstraints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.guest.EXPECT()
	exp.Constraints().Return(constraints.MustParse(""), nil)

	obtained, err := s.policy().determineContainerSpaces(c.Context(), s.machine, s.guest)
	c.Assert(err, tc.ErrorIsNil)
	expected := corenetwork.SpaceInfos{
		*s.spaces.GetByName(corenetwork.AlphaSpaceName),
	}
	c.Check(obtained, tc.DeepEquals, expected)
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesWithProviderNetworkingAndOvsBridge(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(), nil)
	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)

	s.expectNICWithIP(c, ctrl, "ens3", corenetwork.AlphaSpaceId, "10.0.0.0/24")
	// OVS bridges appear as regular nics; however, juju detects them by
	// ovs-vsctl and sets their virtual port type to corenetwork.OvsPort
	s.expectNICWithIPAndPortType(c, ctrl, "ovsbr0", corenetwork.AlphaSpaceId, corenetwork.OvsPort, "10.0.0.0/24")

	s.expectAllDefaultDevices(c, ctrl)
	s.expectMachineAddressesDevices()

	// When using "provider" as the container networking method, the bridge
	// policy code will treat ovs devices as bridges.
	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()
	info, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.HasLen, 1)
	c.Assert(info[0].ParentInterfaceName, tc.Equals, "ovsbr0", tc.Commentf("expected container device parent to be the OVS bridge"))
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesWithLocalNetworkingAndOvsBridge(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)

	s.expectNICWithIP(c, ctrl, "ens3", corenetwork.AlphaSpaceId, "10.0.0.0/24")

	// OVS bridges appear as regular nics; however, juju detects them by
	// ovs-vsctl and sets their virtual port type to corenetwork.OvsPort
	s.expectNICWithIPAndPortType(c, ctrl, "ovsbr0", corenetwork.AlphaSpaceId, corenetwork.OvsPort, "10.0.0.0/24")

	s.expectAllDefaultDevices(c, ctrl)
	s.expectMachineAddressesDevices()

	// When using "local" as the container networking method, the bridge
	// policy code will treat ovs devices as regular NICs.
	bridgePolicy := s.policy()

	info, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.HasLen, 1)
	c.Assert(info[0].ParentInterfaceName, tc.Equals, "lxdbr0", tc.Commentf("expected container device parent to be the default lxd bridge as the container networking method is 'local'"))
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesCorrectlyPaired(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	id := networktesting.GenSpaceUUID(c)
	s.spaces = append(s.spaces, corenetwork.SpaceInfo{
		ID:      id,
		Name:    "somespace",
		Subnets: nil,
	})
	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=somespace"), nil)

	// The device names chosen and the order are very explicit. We
	// need to ensure that we have a list that does not sort well
	// alphabetically. This is because SetParentLinkLayerDevices()
	// uses a natural sort ordering and we want to verify the
	// pairing between the container's NIC name and its parent in
	// the host machine during this unit test.
	s.expectBridgeDeviceWithIP(c, ctrl, "br-eth10", id, "10.0.0.0/24")
	s.expectBridgeDeviceWithIP(c, ctrl, "br-eth1", id, "10.0.0.0/24")
	s.expectBridgeDeviceWithIP(c, ctrl, "br-eth10-100", id, "10.0.0.0/24")
	s.expectBridgeDeviceWithIP(c, ctrl, "br-eth2", id, "10.0.0.0/24")
	s.expectBridgeDeviceWithIP(c, ctrl, "br-eth0", id, "10.0.0.0/24")
	s.expectBridgeDeviceWithIP(c, ctrl, "br-eth4", id, "10.0.0.0/24")
	s.expectBridgeDeviceWithIP(c, ctrl, "br-eth3", id, "10.0.0.0/24")

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	info, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, tc.ErrorIsNil)

	expectedParents := []string{
		"br-eth0",
		"br-eth1",
		"br-eth2",
		"br-eth3",
		"br-eth4",
		"br-eth10",
		"br-eth10-100",
	}
	c.Assert(info, tc.HasLen, len(expectedParents))
	for i, dev := range info {
		c.Check(dev.InterfaceName, tc.Equals, "eth"+strconv.Itoa(i))
		c.Check(dev.InterfaceType, tc.Equals, corenetwork.EthernetDevice)
		c.Check(dev.MTU, tc.Equals, 0) // inherited from the parent device.
		c.Check(dev.MACAddress, tc.Matches, "00:16:3e(:[0-9a-f]{2}){3}")
		c.Check(dev.Disabled, tc.IsFalse)
		c.Check(dev.NoAutoStart, tc.IsFalse)
		c.Check(dev.ParentInterfaceName, tc.Equals, expectedParents[i])
	}
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesConstraintsBindOnlyOne(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=dmz"), nil)

	s.setupMachineInTwoSpaces(c, ctrl)
	s.expectMachineAddressesDevices()

	bridgePolicy := s.policy()

	info, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.HasLen, 1)
	dev := info[0]
	c.Check(dev.InterfaceName, tc.Equals, "eth0")
	c.Check(dev.InterfaceType, tc.Equals, corenetwork.EthernetDevice)
	c.Check(dev.MTU, tc.Equals, 0) // inherited from the parent device.
	c.Check(dev.MACAddress, tc.Matches, "00:16:3e(:[0-9a-f]{2}){3}")
	c.Check(dev.Disabled, tc.IsFalse)
	c.Check(dev.NoAutoStart, tc.IsFalse)
	// br-ens0p10 on the host machine is in space dmz, while br-ens33 is in space somespace
	c.Check(dev.ParentInterfaceName, tc.Equals, "br-ens0p10")
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesHostOneSpace(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)

	ids := s.setupTwoSpaces(c)
	// We set the machine to be in 'dmz'; it is in a single space.
	// Adding a container to a machine that is in a single space puts
	// that container into the same space.
	s.expectNICAndBridgeWithIP(c, ctrl, "eth0", "br-eth0", ids[dmzIndex], "10.0.0.0/24")
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(ids[dmzIndex].String()), nil)

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	info, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.HasLen, 1)
	dev := info[0]
	c.Check(dev.InterfaceName, tc.Equals, "eth0")
	c.Check(dev.InterfaceType, tc.Equals, corenetwork.EthernetDevice)
	c.Check(dev.MTU, tc.Equals, 0) // inherited from the parent device.
	c.Check(dev.MACAddress, tc.Matches, "00:16:3e(:[0-9a-f]{2}){3}")
	c.Check(dev.Disabled, tc.IsFalse)
	c.Check(dev.NoAutoStart, tc.IsFalse)
	c.Check(dev.ParentInterfaceName, tc.Equals, "br-eth0")
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesDefaultSpace(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)

	// TODO(jam): 2016-12-28 Eventually we probably want to have a
	// model-config level default-space, but for now, 'default' should not be
	// special.
	// The host machine is in both 'default' and 'dmz', and the container is
	// not requested to be in any particular space. But because we have
	// access to the 'default' space, we go ahead and use that for the
	// container.
	ids := s.setupMachineInTwoSpaces(c, ctrl)
	strIDs := transform.Slice(ids, func(s corenetwork.SpaceUUID) string { return s.String() })
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(strIDs...), nil)

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	_, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, tc.ErrorMatches, "no obvious space for container.*")
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesNoValidSpace(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)

	// The host machine will be in 2 spaces, but neither one is 'somespace',
	// thus we are unable to find a valid space to put the container in.
	ids := s.setupTwoSpaces(c)
	// Is put into the 'dmz' space
	s.expectNICAndBridgeWithIP(c, ctrl, "eth0", "br-eth0", ids[dmzIndex], "10.0.0.0/24")
	// Second bridge is in the 'db' space
	id := networktesting.GenSpaceUUID(c)
	s.spaces = append(s.spaces, corenetwork.SpaceInfo{
		ID:      id,
		Name:    "db",
		Subnets: nil,
	})
	s.expectNICAndBridgeWithIP(c, ctrl, "ens4", "br-ens4", id, "10.0.1.0/24")
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(ids[dmzIndex].String(), id.String()), nil)

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	_, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, tc.ErrorMatches, `no obvious space for container "guest-id", host machine has spaces: .*`)
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesMismatchConstraints(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Machine is in 'somespace' but container wants to be in 'dmz'
	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=dmz"), nil)
	ids := s.setupTwoSpaces(c)
	// Is put into the 'somespace' space
	s.expectNICAndBridgeWithIP(c, ctrl, "eth0", "br-eth0", ids[somespaceIndex], "10.0.0.0/24")

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	_, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Equals, `unable to find host bridge for space(s) "dmz" for container "guest-id"`)
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesMissingBridge(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Machine is in 'somespace' and 'dmz' but doesn't have a bridge for 'dmz'
	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=dmz"), nil)
	ids := s.setupTwoSpaces(c)
	s.expectNICAndBridgeWithIP(c, ctrl, "eth0", "br-eth0", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICWithIP(c, ctrl, "ens5", ids[dmzIndex], "10.0.1.0/24")

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	_, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Equals, `unable to find host bridge for space(s) "dmz" for container "guest-id"`)
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesNoDefaultNoConstraints(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)

	// The host machine will be in 2 spaces, but neither one is 'somespace',
	// thus we are unable to find a valid space to put the container in.
	ids := s.setupTwoSpaces(c)
	// In 'dmz'
	s.expectNICAndBridgeWithIP(c, ctrl, "eth0", "br-eth0", ids[dmzIndex], "10.0.0.0/24")
	// Second bridge is in the 'db' space
	id := networktesting.GenSpaceUUID(c)
	s.spaces = append(s.spaces, corenetwork.SpaceInfo{
		ID:      id,
		Name:    "db",
		Subnets: nil,
	})
	s.expectNICAndBridgeWithIP(c, ctrl, "ens4", "br-ens4", id, "10.0.1.0/24")
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(ids[dmzIndex].String(), id.String()), nil)

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	_, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, tc.ErrorMatches, `no obvious space for container "guest-id", host machine has spaces: .*`)
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesTwoDevicesOneBridged(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// The host machine has 2 devices in one space, but only one is bridged.
	// We'll only use the one that is bridged, and not complain about the other.
	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=somespace"), nil)
	ids := s.setupTwoSpaces(c)
	// In 'somespace'
	s.expectNICWithIP(c, ctrl, "eth0", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICAndBridgeWithIP(c, ctrl, "eth1", "br-eth1", ids[somespaceIndex], "10.0.0.0/24")

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	info, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.HasLen, 1)
	dev := info[0]
	c.Check(dev.InterfaceName, tc.Equals, "eth0")
	c.Check(dev.InterfaceType, tc.Equals, corenetwork.EthernetDevice)
	c.Check(dev.MTU, tc.Equals, 0) // inherited from the parent device.
	c.Check(dev.MACAddress, tc.Matches, "00:16:3e(:[0-9a-f]{2}){3}")
	c.Check(dev.Disabled, tc.IsFalse)
	c.Check(dev.NoAutoStart, tc.IsFalse)
	// br-eth1 is a valid bridge in the 'somespace' space
	c.Check(dev.ParentInterfaceName, tc.Equals, "br-eth1")
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesTwoBridgedSameSpace(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// The host machine has 2 devices and both are bridged into the desired space
	// We'll use both
	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=somespace"), nil)
	ids := s.setupTwoSpaces(c)
	// In 'somespace'
	s.expectNICAndBridgeWithIP(c, ctrl, "ens33", "br-ens33", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICAndBridgeWithIP(c, ctrl, "ens44", "br-ens44", ids[somespaceIndex], "10.0.0.0/24")

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	info, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.HasLen, 2)
	dev := info[0]
	c.Check(dev.InterfaceName, tc.Equals, "eth0")
	// br-ens33 and br-ens44 are both bridges in the 'somespace' space
	c.Check(dev.ParentInterfaceName, tc.Equals, "br-ens33")
	dev = info[1]
	c.Check(dev.InterfaceName, tc.Equals, "eth1")
	// br-ens33 and br-ens44 are both bridges in the 'somespace' space
	c.Check(dev.ParentInterfaceName, tc.Equals, "br-ens44")
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesTwoBridgesNotInSpaces(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)

	// The host machine has 2 network devices and 2 bridges, but none of them
	// are in a known space. The container also has no requested space.
	// In that case, we will use all of the unknown bridges for container
	// devices.
	s.setupTwoSpaces(c)
	s.expectNICAndBridgeWithIP(c, ctrl, "ens3", "br-ens3", corenetwork.AlphaSpaceId, "10.0.0.0/24")
	s.expectNICAndBridgeWithIP(c, ctrl, "ens4", "br-ens4", corenetwork.AlphaSpaceId, "10.0.0.0/24")
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(), nil)

	s.expectAllDefaultDevices(c, ctrl)
	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	info, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.HasLen, 2)
	dev := info[0]
	c.Check(dev.InterfaceName, tc.Equals, "eth0")
	// br-ens33 and br-ens44 are both bridges in the 'somespace' space
	c.Check(dev.ParentInterfaceName, tc.Equals, "br-ens3")
	dev = info[1]
	c.Check(dev.InterfaceName, tc.Equals, "eth1")
	// br-ens33 and br-ens44 are both bridges in the 'somespace' space
	c.Check(dev.ParentInterfaceName, tc.Equals, "br-ens4")
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesNoLocal(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// The host machine has 1 network device and only local bridges, but none of them
	// are in a known space. The container also has no requested space.
	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)
	s.setupTwoSpaces(c)
	s.expectNICWithIP(c, ctrl, "ens3", corenetwork.AlphaSpaceId, "10.0.0.0/24")
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(), nil)
	s.expectAllDefaultDevices(c, ctrl)

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	_, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Equals, `unable to find host bridge for space(s) "alpha" for container "guest-id"`)
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesUseLocal(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// The host machine has 1 network device and only local bridges, but none of them
	// are in a known space. The container also has no requested space.
	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)
	s.setupTwoSpaces(c)
	s.expectNICWithIP(c, ctrl, "ens3", corenetwork.AlphaSpaceId, "10.0.0.0/24")
	s.expectAllDefaultDevices(c, ctrl)

	s.expectMachineAddressesDevices()

	bridgePolicy := s.policy()

	info, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.HasLen, 1)
	dev := info[0]
	c.Check(dev.InterfaceName, tc.Equals, "eth0")
	c.Check(dev.ParentInterfaceName, tc.Equals, "lxdbr0")
}
