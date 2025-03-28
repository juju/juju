// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerizer

import (
	"strconv"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/containermanager"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/network"
)

type bridgePolicySuite struct {
	baseSuite

	netBondReconfigureDelay   int
	containerNetworkingMethod containermanager.NetworkingMethod

	spaces corenetwork.SpaceInfos
	guest  *MockContainer
}

var _ = gc.Suite(&bridgePolicySuite{})

func (s *bridgePolicySuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)

	s.netBondReconfigureDelay = 13
	s.containerNetworkingMethod = "local"
	s.spaces = nil
}

func (s *bridgePolicySuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.guest = NewMockContainer(ctrl)
	s.guest.EXPECT().Id().Return("guest-id").AnyTimes()
	s.guest.EXPECT().ContainerType().Return(instance.LXD).AnyTimes()

	s.spaces = make(corenetwork.SpaceInfos, 4)
	s.spaces[0] = corenetwork.SpaceInfo{ID: corenetwork.AlphaSpaceId, Name: corenetwork.AlphaSpaceName}
	for i, space := range []string{"foo", "bar", "fizz"} {
		id := "deeadbeef" + strconv.Itoa(i)
		s.spaces = append(s.spaces, corenetwork.SpaceInfo{ID: id, Name: corenetwork.SpaceName(space)})
	}
	return ctrl
}

func (s *bridgePolicySuite) setupTwoSpaces() []string {
	id1 := strconv.Itoa(len(s.spaces))
	id2 := strconv.Itoa(len(s.spaces) + 1)
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
	return []string{id1, id2}
}

const (
	somespaceIndex = 0
	dmzIndex       = 1
)

func (s *bridgePolicySuite) setupMachineInTwoSpaces(c *gc.C, ctrl *gomock.Controller) []string {
	ids := s.setupTwoSpaces()
	s.expectNICAndBridgeWithIP(c, ctrl, "ens33", "br-ens33", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICAndBridgeWithIP(c, ctrl, "ens0p10", "br-ens0p10", ids[dmzIndex], "10.0.1.0/24")
	return ids
}

// expectAllDefaultDevices creates the loopback, lxcbr0, lxdbr0, and virbr0 devices
func (s *bridgePolicySuite) expectAllDefaultDevices(c *gc.C, ctrl *gomock.Controller) {
	// loopback
	s.expectLoopbackNIC(ctrl)
	// container.DefaultLxdBridge
	s.expectBridgeDeviceWithIP(c, ctrl, "lxdbr0", corenetwork.AlphaSpaceId, "10.0.0.0/24")
}

func (s *bridgePolicySuite) policy() *BridgePolicy {
	return &BridgePolicy{
		allSpaces:                 s.spaces,
		allSubnets:                s.baseSuite.allSubnets,
		netBondReconfigureDelay:   s.netBondReconfigureDelay,
		containerNetworkingMethod: s.containerNetworkingMethod,
	}
}

func (s *bridgePolicySuite) TestDetermineContainerSpacesConstraints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.guest.EXPECT()
	exp.Constraints().Return(constraints.MustParse("spaces=foo,bar,^baz"), nil)

	obtained, err := s.policy().determineContainerSpaces(s.machine, s.guest)
	c.Assert(err, jc.ErrorIsNil)
	expected := corenetwork.SpaceInfos{
		*s.spaces.GetByName("foo"),
		*s.spaces.GetByName("bar"),
	}
	c.Check(obtained, jc.DeepEquals, expected)
}

func (s *bridgePolicySuite) TestDetermineContainerNoSpacesConstraints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.guest.EXPECT()
	exp.Constraints().Return(constraints.MustParse(""), nil)

	obtained, err := s.policy().determineContainerSpaces(s.machine, s.guest)
	c.Assert(err, jc.ErrorIsNil)
	expected := corenetwork.SpaceInfos{
		*s.spaces.GetByName(corenetwork.AlphaSpaceName),
	}
	c.Check(obtained, jc.DeepEquals, expected)
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesWithProviderNetworkingAndOvsBridge(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.HasLen, 1)
	c.Assert(info[0].ParentInterfaceName, gc.Equals, "ovsbr0", gc.Commentf("expected container device parent to be the OVS bridge"))
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesWithLocalNetworkingAndOvsBridge(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.HasLen, 1)
	c.Assert(info[0].ParentInterfaceName, gc.Equals, "lxdbr0", gc.Commentf("expected container device parent to be the default lxd bridge as the container networking method is 'local'"))
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesCorrectlyPaired(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	id := strconv.Itoa(len(s.spaces))
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
	c.Assert(err, jc.ErrorIsNil)

	expectedParents := []string{
		"br-eth0",
		"br-eth1",
		"br-eth2",
		"br-eth3",
		"br-eth4",
		"br-eth10",
		"br-eth10-100",
	}
	c.Assert(info, gc.HasLen, len(expectedParents))
	for i, dev := range info {
		c.Check(dev.InterfaceName, gc.Equals, "eth"+strconv.Itoa(i))
		c.Check(dev.InterfaceType, gc.Equals, corenetwork.EthernetDevice)
		c.Check(dev.MTU, gc.Equals, 0) // inherited from the parent device.
		c.Check(dev.MACAddress, gc.Matches, "00:16:3e(:[0-9a-f]{2}){3}")
		c.Check(dev.Disabled, jc.IsFalse)
		c.Check(dev.NoAutoStart, jc.IsFalse)
		c.Check(dev.ParentInterfaceName, gc.Equals, expectedParents[i])
	}
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesConstraintsBindOnlyOne(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=dmz"), nil)

	s.setupMachineInTwoSpaces(c, ctrl)
	s.expectMachineAddressesDevices()

	bridgePolicy := s.policy()

	info, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.HasLen, 1)
	dev := info[0]
	c.Check(dev.InterfaceName, gc.Equals, "eth0")
	c.Check(dev.InterfaceType, gc.Equals, corenetwork.EthernetDevice)
	c.Check(dev.MTU, gc.Equals, 0) // inherited from the parent device.
	c.Check(dev.MACAddress, gc.Matches, "00:16:3e(:[0-9a-f]{2}){3}")
	c.Check(dev.Disabled, jc.IsFalse)
	c.Check(dev.NoAutoStart, jc.IsFalse)
	// br-ens0p10 on the host machine is in space dmz, while br-ens33 is in space somespace
	c.Check(dev.ParentInterfaceName, gc.Equals, "br-ens0p10")
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesHostOneSpace(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)

	ids := s.setupTwoSpaces()
	// We set the machine to be in 'dmz'; it is in a single space.
	// Adding a container to a machine that is in a single space puts
	// that container into the same space.
	s.expectNICAndBridgeWithIP(c, ctrl, "eth0", "br-eth0", ids[dmzIndex], "10.0.0.0/24")
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(ids[dmzIndex]), nil)

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	info, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.HasLen, 1)
	dev := info[0]
	c.Check(dev.InterfaceName, gc.Equals, "eth0")
	c.Check(dev.InterfaceType, gc.Equals, corenetwork.EthernetDevice)
	c.Check(dev.MTU, gc.Equals, 0) // inherited from the parent device.
	c.Check(dev.MACAddress, gc.Matches, "00:16:3e(:[0-9a-f]{2}){3}")
	c.Check(dev.Disabled, jc.IsFalse)
	c.Check(dev.NoAutoStart, jc.IsFalse)
	c.Check(dev.ParentInterfaceName, gc.Equals, "br-eth0")
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesDefaultSpace(c *gc.C) {
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
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(ids...), nil)

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	_, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, gc.ErrorMatches, "no obvious space for container.*")
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesNoValidSpace(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)

	// The host machine will be in 2 spaces, but neither one is 'somespace',
	// thus we are unable to find a valid space to put the container in.
	ids := s.setupTwoSpaces()
	// Is put into the 'dmz' space
	s.expectNICAndBridgeWithIP(c, ctrl, "eth0", "br-eth0", ids[dmzIndex], "10.0.0.0/24")
	// Second bridge is in the 'db' space
	id := strconv.Itoa(len(s.spaces))
	s.spaces = append(s.spaces, corenetwork.SpaceInfo{
		ID:      id,
		Name:    "db",
		Subnets: nil,
	})
	s.expectNICAndBridgeWithIP(c, ctrl, "ens4", "br-ens4", id, "10.0.1.0/24")
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(ids[dmzIndex], id), nil)

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	_, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, gc.ErrorMatches, `no obvious space for container "guest-id", host machine has spaces: .*`)
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesMismatchConstraints(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Machine is in 'somespace' but container wants to be in 'dmz'
	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=dmz"), nil)
	ids := s.setupTwoSpaces()
	// Is put into the 'somespace' space
	s.expectNICAndBridgeWithIP(c, ctrl, "eth0", "br-eth0", ids[somespaceIndex], "10.0.0.0/24")

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	_, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `unable to find host bridge for space(s) "dmz" for container "guest-id"`)
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesMissingBridge(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Machine is in 'somespace' and 'dmz' but doesn't have a bridge for 'dmz'
	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=dmz"), nil)
	ids := s.setupTwoSpaces()
	s.expectNICAndBridgeWithIP(c, ctrl, "eth0", "br-eth0", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICWithIP(c, ctrl, "ens5", ids[dmzIndex], "10.0.1.0/24")

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	_, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `unable to find host bridge for space(s) "dmz" for container "guest-id"`)
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesNoDefaultNoConstraints(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)

	// The host machine will be in 2 spaces, but neither one is 'somespace',
	// thus we are unable to find a valid space to put the container in.
	ids := s.setupTwoSpaces()
	// In 'dmz'
	s.expectNICAndBridgeWithIP(c, ctrl, "eth0", "br-eth0", ids[dmzIndex], "10.0.0.0/24")
	// Second bridge is in the 'db' space
	id := strconv.Itoa(len(s.spaces))
	s.spaces = append(s.spaces, corenetwork.SpaceInfo{
		ID:      id,
		Name:    "db",
		Subnets: nil,
	})
	s.expectNICAndBridgeWithIP(c, ctrl, "ens4", "br-ens4", id, "10.0.1.0/24")
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(ids[dmzIndex], id), nil)

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	_, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, gc.ErrorMatches, `no obvious space for container "guest-id", host machine has spaces: .*`)
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesTwoDevicesOneBridged(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// The host machine has 2 devices in one space, but only one is bridged.
	// We'll only use the one that is bridged, and not complain about the other.
	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=somespace"), nil)
	ids := s.setupTwoSpaces()
	// In 'somespace'
	s.expectNICWithIP(c, ctrl, "eth0", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICAndBridgeWithIP(c, ctrl, "eth1", "br-eth1", ids[somespaceIndex], "10.0.0.0/24")

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	info, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.HasLen, 1)
	dev := info[0]
	c.Check(dev.InterfaceName, gc.Equals, "eth0")
	c.Check(dev.InterfaceType, gc.Equals, corenetwork.EthernetDevice)
	c.Check(dev.MTU, gc.Equals, 0) // inherited from the parent device.
	c.Check(dev.MACAddress, gc.Matches, "00:16:3e(:[0-9a-f]{2}){3}")
	c.Check(dev.Disabled, jc.IsFalse)
	c.Check(dev.NoAutoStart, jc.IsFalse)
	// br-eth1 is a valid bridge in the 'somespace' space
	c.Check(dev.ParentInterfaceName, gc.Equals, "br-eth1")
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesTwoBridgedSameSpace(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// The host machine has 2 devices and both are bridged into the desired space
	// We'll use both
	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=somespace"), nil)
	ids := s.setupTwoSpaces()
	// In 'somespace'
	s.expectNICAndBridgeWithIP(c, ctrl, "ens33", "br-ens33", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICAndBridgeWithIP(c, ctrl, "ens44", "br-ens44", ids[somespaceIndex], "10.0.0.0/24")

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	info, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.HasLen, 2)
	dev := info[0]
	c.Check(dev.InterfaceName, gc.Equals, "eth0")
	// br-ens33 and br-ens44 are both bridges in the 'somespace' space
	c.Check(dev.ParentInterfaceName, gc.Equals, "br-ens33")
	dev = info[1]
	c.Check(dev.InterfaceName, gc.Equals, "eth1")
	// br-ens33 and br-ens44 are both bridges in the 'somespace' space
	c.Check(dev.ParentInterfaceName, gc.Equals, "br-ens44")
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesTwoBridgesNotInSpaces(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)

	// The host machine has 2 network devices and 2 bridges, but none of them
	// are in a known space. The container also has no requested space.
	// In that case, we will use all of the unknown bridges for container
	// devices.
	s.setupTwoSpaces()
	s.expectNICAndBridgeWithIP(c, ctrl, "ens3", "br-ens3", corenetwork.AlphaSpaceId, "10.0.0.0/24")
	s.expectNICAndBridgeWithIP(c, ctrl, "ens4", "br-ens4", corenetwork.AlphaSpaceId, "10.0.0.0/24")
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(), nil)

	s.expectAllDefaultDevices(c, ctrl)
	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	info, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.HasLen, 2)
	dev := info[0]
	c.Check(dev.InterfaceName, gc.Equals, "eth0")
	// br-ens33 and br-ens44 are both bridges in the 'somespace' space
	c.Check(dev.ParentInterfaceName, gc.Equals, "br-ens3")
	dev = info[1]
	c.Check(dev.InterfaceName, gc.Equals, "eth1")
	// br-ens33 and br-ens44 are both bridges in the 'somespace' space
	c.Check(dev.ParentInterfaceName, gc.Equals, "br-ens4")
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesNoLocal(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// The host machine has 1 network device and only local bridges, but none of them
	// are in a known space. The container also has no requested space.
	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)
	s.setupTwoSpaces()
	s.expectNICWithIP(c, ctrl, "ens3", corenetwork.AlphaSpaceId, "10.0.0.0/24")
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(), nil)
	s.expectAllDefaultDevices(c, ctrl)

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	_, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `unable to find host bridge for space(s) "alpha" for container "guest-id"`)
}

func (s *bridgePolicySuite) TestPopulateContainerLinkLayerDevicesUseLocal(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// The host machine has 1 network device and only local bridges, but none of them
	// are in a known space. The container also has no requested space.
	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)
	s.setupTwoSpaces()
	s.expectNICWithIP(c, ctrl, "ens3", corenetwork.AlphaSpaceId, "10.0.0.0/24")
	s.expectAllDefaultDevices(c, ctrl)

	s.expectMachineAddressesDevices()

	bridgePolicy := s.policy()

	info, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.HasLen, 1)
	dev := info[0]
	c.Check(dev.InterfaceName, gc.Equals, "eth0")
	c.Check(dev.ParentInterfaceName, gc.Equals, "lxdbr0")
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerNoneMissing(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=somespace"), nil)

	ids := s.setupTwoSpaces()
	s.expectNICAndBridgeWithIP(c, ctrl, "eth0", "br-eth0", ids[somespaceIndex], "10.0.0.0/24")

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerDefaultUnbridged(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=somespace"), nil)

	ids := s.setupTwoSpaces()
	s.expectNICWithIP(c, ctrl, "eth0", ids[somespaceIndex], "10.0.0.0/24")

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, gc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
	}})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerNoHostDevices(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=dmz,third"), nil)

	s.setupTwoSpaces()
	id := strconv.Itoa(len(s.spaces))
	s.spaces = append(s.spaces, corenetwork.SpaceInfo{
		ID:      id,
		Name:    "third",
		Subnets: nil,
	})
	s.expectNICWithIP(c, ctrl, "eth0", corenetwork.AlphaSpaceId, "10.0.0.0/24")
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(), nil)

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	_, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `host machine "host-id" has no available device in space(s) "dmz", "third"`)
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerTwoSpacesOneMissing(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=somespace,dmz"), nil)

	ids := s.setupTwoSpaces()
	// dmz
	s.expectBridgeDeviceWithIP(c, ctrl, "eth1", ids[dmzIndex], "10.0.0.0/24")
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(), nil)

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	_, _, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, gc.NotNil)
	// both somespace and dmz are needed, but somespace is missing
	c.Assert(err.Error(), gc.Equals, `host machine "host-id" has no available device in space(s) "somespace"`)
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerNoSpaces(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)

	// There is a "somespace" and "dmz" space, and our machine has 2 network
	// interfaces, but is not part of any known space. In this circumstance,
	// we should try to bridge all of the unknown space devices, not just one
	// of them. This is are fallback mode when we don't understand the spaces of a machine.
	s.setupTwoSpaces()
	s.expectNICWithIP(c, ctrl, "ens3", corenetwork.AlphaSpaceId, "10.0.0.0/24")
	s.expectNICWithIP(c, ctrl, "ens4", corenetwork.AlphaSpaceId, "10.0.0.0/24")
	s.expectAllDefaultDevices(c, ctrl)
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(), nil)

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	// No defined spaces for the container, no *known* spaces for the host
	// machine. Triggers the fallback code to have us bridge all devices.
	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, gc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "ens3",
		BridgeName: "br-ens3",
	}, {
		DeviceName: "ens4",
		BridgeName: "br-ens4",
	}})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerContainerNetworkingMethodLocal(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)

	// There is a "somespace" and "dmz" space, our machine has 1 network
	// interface, but is not part of a known space. We have containerNetworkingMethod set to "local",
	// which means we should fall back to using 'lxdbr0' instead of
	// bridging the host device.
	s.setupTwoSpaces()
	s.expectNICWithIP(c, ctrl, "ens3", corenetwork.AlphaSpaceId, "10.0.0.0/24")
	s.expectAllDefaultDevices(c, ctrl)

	s.expectMachineAddressesDevices()

	bridgePolicy := s.policy()

	// No defined spaces for the container, no *known* spaces for the host
	// machine. Triggers the fallback code to have us bridge all devices.
	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerContainerNetworkingMethodLocalDefinedHostSpace(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil).Times(2)

	// There is a "somespace" and "dmz" space, our machine has 1 network
	// interface, but is not part of a known space. We have containerNetworkingMethod set to "local",
	// which means we should fall back to using 'lxdbr0' instead of
	// bridging the host device.
	s.setupTwoSpaces()
	s.expectNICWithIP(c, ctrl, "eth0", corenetwork.AlphaSpaceId, "10.0.0.0/24")
	s.expectAllDefaultDevices(c, ctrl)

	s.expectMachineAddressesDevices()

	bridgePolicy := s.policy()

	// No defined spaces for the container, host has spaces but we have
	// ContainerNetworkingMethodLocal set so we should fall back to lxdbr0
	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{})
	c.Check(reconfigureDelay, gc.Equals, 0)

	info, err := bridgePolicy.PopulateContainerLinkLayerDevices(s.machine, s.guest, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.HasLen, 1)
	dev := info[0]
	c.Check(dev.InterfaceName, gc.Equals, "eth0")
	c.Check(dev.ParentInterfaceName, gc.Equals, "lxdbr0")
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerContainerNetworkingMethodLocalNoAddress(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)

	// We should only use 'lxdbr0' instead of bridging the host device.
	s.setupTwoSpaces()
	s.expectNICWithIP(c, ctrl, "ens3", corenetwork.AlphaSpaceId, "10.0.0.0/24")
	s.expectBridgeDevice(ctrl, "lxdbr0")

	s.expectMachineAddressesDevices()

	bridgePolicy := s.policy()

	// No defined spaces for the container, no *known* spaces for the host
	// machine. Triggers the fallback code to have us bridge all devices.
	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{
		{DeviceName: "ens3", BridgeName: "br-ens3", MACAddress: ""},
	})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerUnknownWithConstraint(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// If we have a host machine where we don't understand its spaces, but
	// the container requests a specific space, we won't use the unknown
	// ones.
	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=somespace"), nil)
	s.setupTwoSpaces()
	s.expectNICWithIP(c, ctrl, "ens3", corenetwork.AlphaSpaceId, "10.0.0.0/24")
	s.expectNICWithIP(c, ctrl, "ens4", corenetwork.AlphaSpaceId, "10.0.0.0/24")
	s.expectAllDefaultDevices(c, ctrl)
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(), nil)

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	_, _, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals,
		`host machine "host-id" has no available device in space(s) "somespace"`)
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerUnknownAndDefault(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)

	// The host machine has 2 devices, one is in a known 'somespace' space, the other is in an unknown space.
	// We will ignore the unknown space and just return the one in 'somespace',
	// cause that is the only declared space on the machine.
	ids := s.setupTwoSpaces()
	// Default
	s.expectNICWithIP(c, ctrl, "ens3", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICWithIP(c, ctrl, "ens4", corenetwork.AlphaSpaceId, "10.0.1.0/24")
	s.expectAllDefaultDevices(c, ctrl)
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(ids[somespaceIndex]), nil)

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	// We don't need a container constraint, as the host machine is in a single space.
	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, gc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "ens3",
		BridgeName: "br-ens3",
	}})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerOneOfTwoBridged(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// With two host devices that could be bridged, we will only ask for the
	// first one to be bridged.
	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=somespace"), nil)
	ids := s.setupTwoSpaces()
	s.expectNICWithIP(c, ctrl, "ens3", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICWithIP(c, ctrl, "ens4", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICWithIP(c, ctrl, "ens5", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICWithIP(c, ctrl, "ens6", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICWithIP(c, ctrl, "ens7", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICWithIP(c, ctrl, "ens8", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICWithIP(c, ctrl, "ens3.1", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICWithIP(c, ctrl, "ens3:1", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICWithIP(c, ctrl, "ens2.1", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICWithIP(c, ctrl, "ens2.2", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICWithIP(c, ctrl, "ens20", ids[somespaceIndex], "10.0.0.0/24")

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, jc.ErrorIsNil)
	// Only the first device (by sort order) should be selected
	c.Check(missing, gc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "ens2.1",
		BridgeName: "br-ens2-1",
	}})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerTwoHostDevicesOneBridged(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// With two host devices that could be bridged, we will only ask for the
	// first one to be bridged.
	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=somespace"), nil)
	ids := s.setupTwoSpaces()
	s.expectNICWithIP(c, ctrl, "ens3", ids[somespaceIndex], "10.0.0.0/24")
	s.expectNICAndBridgeWithIP(c, ctrl, "ens4", "br-ens4", ids[somespaceIndex], "10.0.0.0/24")

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerNoConstraintsDefaultNotSpecial(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse(""), nil)

	// TODO(jam): 2016-12-28 Eventually we probably want to have a
	// model-config level default-space, but for now, 'somespace' should not be
	// special.
	ids := s.setupTwoSpaces()
	// Default
	s.expectNICWithIP(c, ctrl, "eth0", ids[somespaceIndex], "10.0.0.0/24")
	// DMZ
	s.expectNICWithIP(c, ctrl, "eth1", ids[dmzIndex], "10.0.1.0/24")
	s.machine.EXPECT().AllSpaces(gomock.Any()).Return(set.NewStrings(ids...), nil)

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, gc.ErrorMatches, "no obvious space for container.*")
	c.Assert(missing, gc.IsNil)
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerTwoSpacesOneBridged(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=somespace,dmz"), nil)

	ids := s.setupTwoSpaces()
	// somespace
	s.expectNICWithIP(c, ctrl, "eth0", ids[somespaceIndex], "10.0.0.0/24")
	// DMZ
	s.expectNICAndBridgeWithIP(c, ctrl, "eth1", "br-eth1", ids[dmzIndex], "10.0.1.0/24")

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, jc.ErrorIsNil)
	// both somespace and dmz are needed, but somespace needs to be bridged
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
	}})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerMultipleSpacesNoneBridged(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=somespace,dmz,abba"), nil)

	ids := s.setupTwoSpaces()
	// somespace
	s.expectNICWithIP(c, ctrl, "eth0", ids[somespaceIndex], "10.0.0.0/24")
	// DMZ
	s.expectNICWithIP(c, ctrl, "eth1", ids[dmzIndex], "10.0.1.0/24")

	id := strconv.Itoa(len(s.spaces))
	s.spaces = append(s.spaces, corenetwork.SpaceInfo{
		ID:      id,
		Name:    "abba",
		Subnets: nil,
	})
	s.expectNICWithIP(c, ctrl, "eth0.1", id, "10.0.2.0/24")

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, jc.ErrorIsNil)
	// both default and dmz are needed, but default needs to be bridged
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
	}, {
		DeviceName: "eth0.1",
		BridgeName: "br-eth0-1",
	}, {
		DeviceName: "eth1",
		BridgeName: "br-eth1",
	}})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerBondedNICs(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=somespace"), nil)

	ids := s.setupTwoSpaces()
	// somespace
	// We call it 'zbond' so it sorts late instead of first
	s.expectDeviceWithIP(c, ctrl, "zbond0", ids[somespaceIndex], corenetwork.BondDevice, "10.0.0.0/24")
	s.expectDevice(ctrl, "eth0", "zbond0", corenetwork.EthernetDevice, corenetwork.NonVirtualPort)
	s.expectDevice(ctrl, "eth1", "zbond0", corenetwork.EthernetDevice, corenetwork.NonVirtualPort)

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, jc.ErrorIsNil)
	// both somespace and dmz are needed, but somespace needs to be bridged
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "zbond0",
		BridgeName: "br-zbond0",
	}})
	// We are creating a bridge on a bond, so we use a non-zero delay
	c.Check(reconfigureDelay, gc.Equals, 13)
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerVLAN(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	ids := s.setupTwoSpaces()
	// We create an eth0 that has an address, and then an eth0.100 which is
	// VLAN tagged on top of that ethernet device.
	// "eth0" is in "somespace", "eth0.100" is in "dmz"
	s.expectNICWithIP(c, ctrl, "eth0", ids[somespaceIndex], "10.0.0.0/24")
	s.expectDeviceWithIP(c, ctrl, "eth0.100", ids[dmzIndex], corenetwork.VLAN8021QDevice, "10.0.1.0/24")

	// We create a container in both spaces, and we should see that it wants
	// to bridge both devices.
	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=somespace,dmz"), nil)

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
	}, {
		DeviceName: "eth0.100",
		BridgeName: "br-eth0-100",
	}})
	c.Check(reconfigureDelay, gc.Equals, 0)
}

func (s *bridgePolicySuite) TestFindMissingBridgesForContainerVLANOnBond(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	ids := s.setupTwoSpaces()
	// We have eth0 and eth1 that don't have IP addresses, that are in a
	// bond, which then has a VLAN on top of that bond. The VLAN should still
	// be a valid target for bridging
	dev := s.expectDeviceWithIP(c, ctrl, "bond0", ids[somespaceIndex], corenetwork.BondDevice, "10.0.0.0/24")
	s.expectDevice(ctrl, "eth0", "bond0", corenetwork.EthernetDevice, corenetwork.NonVirtualPort)
	s.expectDevice(ctrl, "eth1", "bond0", corenetwork.EthernetDevice, corenetwork.NonVirtualPort)
	devv := s.expectDeviceWithParentWithIP(c, ctrl, "bond0.100", "bond0", ids[dmzIndex], corenetwork.VLAN8021QDevice, "10.0.1.0/24")
	devv.EXPECT().ParentDevice().Return(dev, nil)

	// We create a container in both spaces, and we should see that it wants
	// to bridge both devices.
	s.guest.EXPECT().Constraints().Return(constraints.MustParse("spaces=somespace,dmz"), nil)

	s.expectMachineAddressesDevices()

	s.containerNetworkingMethod = "provider"
	bridgePolicy := s.policy()

	missing, reconfigureDelay, err := bridgePolicy.FindMissingBridgesForContainer(s.machine, s.guest, s.allSubnets)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(missing, jc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "bond0",
		BridgeName: "br-bond0",
	}, {
		DeviceName: "bond0.100",
		BridgeName: "br-bond0-100",
	}})
	c.Check(reconfigureDelay, gc.Equals, 13)
}

var bridgeNames = map[string]string{
	"eno0":            "br-eno0",
	"enovlan.123":     "br-enovlan-123",
	"twelvechars0":    "br-twelvechars0",
	"thirteenchars":   "b-thirteenchars",
	"enfourteenchar":  "b-fourteenchar",
	"enfifteenchars0": "b-fifteenchars0",
	"fourteenchars1":  "b-5590a4-chars1",
	"fifteenchars.12": "b-38b496-ars-12",
	"zeros0526193032": "b-000000-193032",
	"enx00e07cc81e1d": "b-x00e07cc81e1d",
}

func (s *bridgePolicySuite) TestBridgeNameForDevice(c *gc.C) {
	for deviceName, bridgeName := range bridgeNames {
		generatedBridgeName := BridgeNameForDevice(deviceName)
		c.Assert(generatedBridgeName, gc.Equals, bridgeName)
	}
}
