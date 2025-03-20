// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerizer

import (
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
)

type linkLayerDevForSpacesSuite struct {
	baseSuite
}

var _ = gc.Suite(&linkLayerDevForSpacesSuite{})

func (s *linkLayerDevForSpacesSuite) policy() *BridgePolicy {
	return &BridgePolicy{
		allSubnets: s.baseSuite.allSubnets,
	}
}

// TODO(jam): 2017-01-31 Make sure KVM guests default to virbr0, and LXD guests use lxdbr0
// Add tests for UseLocal = True, but we have named spaces
// Add tests for UseLocal = True, but the host device is bridged

func (s *linkLayerDevForSpacesSuite) TestLinkLayerDevicesForSpaces(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectNICAndBridgeWithIP(c, ctrl, "eth0", "br-eth0", "1", "10.0.0.0/24")
	s.expectMachineAddressesDevices()

	res, err := s.policy().linkLayerDevicesForSpaces(s.machine, network.SpaceInfos{{ID: "1"}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 1)

	devices, ok := res["1"]
	c.Assert(ok, jc.IsTrue)
	c.Check(devices, gc.HasLen, 1)
	c.Check(devices[0].Name(), gc.Equals, "br-eth0")
	c.Check(devices[0].Type(), gc.Equals, network.BridgeDevice)
}

func (s *linkLayerDevForSpacesSuite) TestLinkLayerDevicesForSpacesNoSuchSpace(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectNICAndBridgeWithIP(c, ctrl, "eth0", "br-eth0", "1", "10.0.0.0/24")
	s.expectMachineAddressesDevices()

	res, err := s.policy().linkLayerDevicesForSpaces(s.machine, network.SpaceInfos{{ID: "2"}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 0)
}

func (s *linkLayerDevForSpacesSuite) TestLinkLayerDevicesForSpacesNoBridge(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectNICWithIP(c, ctrl, "eth0", "1", "10.0.0.0/24")
	s.expectMachineAddressesDevices()

	res, err := s.policy().linkLayerDevicesForSpaces(s.machine, network.SpaceInfos{{ID: "1"}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 1)

	devices, ok := res["1"]
	c.Assert(ok, jc.IsTrue)
	c.Check(devices, gc.HasLen, 1)
	c.Check(devices[0].Name(), gc.Equals, "eth0")
	c.Check(devices[0].Type(), gc.Equals, network.EthernetDevice)
}

func (s *linkLayerDevForSpacesSuite) TestLinkLayerDevicesForSpacesMultipleSpaces(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Is put into the 'somespace' space
	s.expectNICAndBridgeWithIP(c, ctrl, "eth0", "br-eth0", "1", "10.0.0.0/24")
	// Now add a NIC in the dmz space, but without a bridge
	s.expectNICWithIP(c, ctrl, "eth1", "2", "10.0.1.0/24")
	s.expectMachineAddressesDevices()

	res, err := s.policy().linkLayerDevicesForSpaces(s.machine, network.SpaceInfos{{ID: "1"}, {ID: "2"}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 2)

	somespaceDevices, ok := res["1"]
	c.Check(ok, jc.IsTrue)
	c.Check(somespaceDevices, gc.HasLen, 1)
	c.Check(somespaceDevices[0].Name(), gc.Equals, "br-eth0")
	dmzDevices, ok := res["2"]
	c.Check(ok, jc.IsTrue)
	c.Check(dmzDevices, gc.HasLen, 1)
	c.Check(dmzDevices[0].Name(), gc.Equals, "eth1")
}

func (s *linkLayerDevForSpacesSuite) TestLinkLayerDevicesForSpacesWithExtraAddresses(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectNICAndBridgeWithIP(c, ctrl, "eth0", "br-eth0", "1", "10.0.0.0/24")
	// When we poll the machine, we include any IP addresses that we
	// find. One of them is always the loopback, but we could find any
	// other addresses that someone created on the machine that we
	// don't know what they are.
	s.expectNICWithIP(c, ctrl, "lo", network.AlphaSpaceId, "10.0.1.0/24")
	s.expectNICWithIP(c, ctrl, "ens5", network.AlphaSpaceId, "10.0.1.0/24")
	s.expectMachineAddressesDevices()

	res, err := s.policy().linkLayerDevicesForSpaces(s.machine, network.SpaceInfos{{ID: "1"}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 1)

	defaultDevices, ok := res["1"]
	c.Check(ok, jc.IsTrue)
	c.Check(defaultDevices, gc.HasLen, 1)
	c.Check(defaultDevices[0].Name(), gc.Equals, "br-eth0")
}

func (s *linkLayerDevForSpacesSuite) TestLinkLayerDevicesForSpacesInDefaultSpace(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectNICWithIP(c, ctrl, "ens4", network.AlphaSpaceId, "10.0.0.0/24")
	s.expectNICWithIP(c, ctrl, "ens5", network.AlphaSpaceId, "10.0.0.0/24")
	s.expectMachineAddressesDevices()

	res, err := s.policy().linkLayerDevicesForSpaces(s.machine, network.SpaceInfos{{ID: network.AlphaSpaceId}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 1)

	devices, ok := res[network.AlphaSpaceId]
	c.Assert(ok, jc.IsTrue)
	c.Assert(devices, gc.HasLen, 2)
	c.Check(devices[0].Name(), gc.Equals, "ens4")
	c.Check(devices[0].Type(), gc.Equals, network.EthernetDevice)
	c.Check(devices[1].Name(), gc.Equals, "ens5")
	c.Check(devices[1].Type(), gc.Equals, network.EthernetDevice)
}

func (s *linkLayerDevForSpacesSuite) TestLinkLayerDevicesForSpacesWithUnknown(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectNICAndBridgeWithIP(c, ctrl, "ens4", "br-ens4", "1", "10.0.0.0/24")
	s.expectNICWithIP(c, ctrl, "ens5", network.AlphaSpaceId, "10.0.1.0/24")
	s.expectLoopbackNIC(ctrl)
	s.expectMachineAddressesDevices()

	spaces := network.SpaceInfos{{ID: network.AlphaSpaceId}, {ID: "1"}}
	res, err := s.policy().linkLayerDevicesForSpaces(s.machine, spaces)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 2)

	devices, ok := res[network.AlphaSpaceId]
	c.Assert(ok, jc.IsTrue)
	c.Assert(devices, gc.HasLen, 1)
	c.Check(devices[0].Name(), gc.Equals, "ens5")
	c.Check(devices[0].Type(), gc.Equals, network.EthernetDevice)

	devices, ok = res["1"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(devices, gc.HasLen, 1)
	c.Check(devices[0].Name(), gc.Equals, "br-ens4")
	c.Check(devices[0].Type(), gc.Equals, network.BridgeDevice)
}

func (s *linkLayerDevForSpacesSuite) TestLinkLayerDevicesForSpacesWithNoAddress(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// We create a record for the 'lxdbr0' bridge, but it doesn't have an
	// address yet (which is the case when we first show up on a machine.)
	s.expectBridgeDevice(ctrl, "lxdbr0")

	s.expectNICWithIP(c, ctrl, "ens5", network.AlphaSpaceId, "10.0.0.0/24")
	s.expectMachineAddressesDevices()

	res, err := s.policy().linkLayerDevicesForSpaces(s.machine, network.SpaceInfos{{ID: network.AlphaSpaceId}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 1)

	devices, ok := res[network.AlphaSpaceId]
	c.Assert(ok, jc.IsTrue)
	c.Assert(devices, gc.HasLen, 1)
	names := make([]string, len(devices))
	for i, dev := range devices {
		names[i] = dev.Name()
	}
	c.Check(names, gc.DeepEquals, []string{"ens5"})
}

func (s *linkLayerDevForSpacesSuite) TestLinkLayerDevicesForSpacesUnknownIgnoresLoopAndExcludesKnownBridges(c *gc.C) {
	// TODO(jam): 2016-12-28 arguably we should also be aware of Docker
	// devices, possibly the better plan is to look at whether there are
	// routes from the given bridge out into the rest of the world.
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectNICWithIP(c, ctrl, "ens3", network.AlphaSpaceId, "10.0.0.0/24")
	s.expectNICAndBridgeWithIP(c, ctrl, "ens4", "br-ens4", network.AlphaSpaceId, "10.0.0.0/24")
	s.expectLoopbackNIC(ctrl)
	s.expectBridgeDevice(ctrl, "lxcbr0")
	s.expectBridgeDevice(ctrl, "lxdbr0")
	s.expectBridgeDevice(ctrl, "virbr0")
	s.expectMachineAddressesDevices()

	res, err := s.policy().linkLayerDevicesForSpaces(s.machine, network.SpaceInfos{{ID: network.AlphaSpaceId}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 1)
	devices, ok := res[network.AlphaSpaceId]
	c.Assert(ok, jc.IsTrue)
	names := make([]string, len(devices))
	for i, dev := range devices {
		names[i] = dev.Name()
	}
	c.Check(names, gc.DeepEquals, []string{"br-ens4", "ens3"})
}

func (s *linkLayerDevForSpacesSuite) TestLinkLayerDevicesForSpacesSortOrder(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectNICAndBridgeWithIP(c, ctrl, "eth0", "br-eth0", network.AlphaSpaceId, "10.0.0.0/24")
	s.setupForNaturalSort(ctrl)
	s.expectMachineAddressesDevices()

	res, err := s.policy().linkLayerDevicesForSpaces(s.machine, network.SpaceInfos{{ID: network.AlphaSpaceId}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 1)
	defaultDevices, ok := res[network.AlphaSpaceId]
	c.Check(ok, jc.IsTrue)
	names := make([]string, 0, len(defaultDevices))
	for _, dev := range defaultDevices {
		names = append(names, dev.Name())
	}
	c.Check(names, gc.DeepEquals, []string{
		"br-eth0", "br-eth1", "br-eth1.1", "br-eth1:1", "br-eth10", "br-eth10.2",
	})
}

type testDev struct {
	name   string
	parent string
}

func (s *linkLayerDevForSpacesSuite) setupForNaturalSort(ctrl *gomock.Controller) {
	// Add more devices to the "default" space, to make sure the result comes
	// back in NaturallySorted order
	subnet := NewMockSubnet(ctrl)
	sExp := subnet.EXPECT()
	sExp.SpaceID().Return(network.AlphaSpaceId).AnyTimes()

	testDevs := []testDev{
		{"eth1", "br-eth1"},
		{"eth1.1", "br-eth1.1"},
		{"eth1:1", "br-eth1:1"},
		{"eth10", "br-eth10"},
		{"eth10.2", "br-eth10.2"},
		{"eth2", ""},
		{"eth20", ""},
		{"eth3", ""},
	}

	for _, d := range testDevs {
		s.expectDevice(ctrl, d.name, d.parent, network.EthernetDevice, network.NonVirtualPort)
		if d.parent == "" {
			continue
		}
		s.expectBridgeDevice(ctrl, d.parent)

		address := NewMockAddress(ctrl)
		aExp := address.EXPECT()
		aExp.SubnetCIDR().Return("192.168.0.0/24").AnyTimes()
		aExp.DeviceName().Return(d.parent).AnyTimes()

		s.addresses = append(s.addresses, address)
	}
	s.allSubnets = append(s.allSubnets, network.SubnetInfo{
		CIDR:    "192.168.0.0/24",
		SpaceID: network.AlphaSpaceId,
	})
}

type baseSuite struct {
	testing.IsolationSuite

	machine *MockContainer

	devices    []LinkLayerDevice
	addresses  []Address
	allSubnets network.SubnetInfos
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.devices = make([]LinkLayerDevice, 0)
	s.addresses = make([]Address, 0)
	s.allSubnets = make(network.SubnetInfos, 0)
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.machine = NewMockContainer(ctrl)
	s.machine.EXPECT().Id().Return("host-id").AnyTimes()

	return ctrl
}

func (s *baseSuite) expectMachineAddressesDevices() {
	mExp := s.machine.EXPECT()
	mExp.AllLinkLayerDevices().Return(s.devices, nil).AnyTimes()
	mExp.AllDeviceAddresses().Return(s.addresses, nil).AnyTimes()
}

func (s *baseSuite) expectNICAndBridgeWithIP(c *gc.C, ctrl *gomock.Controller, dev, parent, spaceID string, cidr string) {
	s.expectDevice(ctrl, dev, parent, network.EthernetDevice, network.NonVirtualPort)
	s.expectBridgeDevice(ctrl, parent)

	address := NewMockAddress(ctrl)
	aExp := address.EXPECT()
	aExp.SubnetCIDR().Return(cidr).AnyTimes()
	aExp.DeviceName().Return(parent).AnyTimes()

	// Append the expected subnet.
	s.allSubnets = append(s.allSubnets, network.SubnetInfo{
		CIDR:    cidr,
		SpaceID: spaceID,
	})

	s.addresses = append(s.addresses, address)
}

func (s *baseSuite) expectNICWithIP(c *gc.C, ctrl *gomock.Controller, dev, spaceID, cidr string) *MockLinkLayerDevice {
	return s.expectNICWithIPAndPortType(c, ctrl, dev, spaceID, network.NonVirtualPort, cidr)
}

func (s *baseSuite) expectNICWithIPAndPortType(c *gc.C, ctrl *gomock.Controller, devName, spaceID string, portType network.VirtualPortType, cidr string) *MockLinkLayerDevice {
	dev := s.expectDevice(ctrl, devName, "", network.EthernetDevice, portType)

	address := NewMockAddress(ctrl)
	aExp := address.EXPECT()
	aExp.SubnetCIDR().Return(cidr).AnyTimes()
	aExp.DeviceName().Return(devName).AnyTimes()

	// Append the expected subnet.
	s.allSubnets = append(s.allSubnets, network.SubnetInfo{
		CIDR:    cidr,
		SpaceID: spaceID,
	})

	s.addresses = append(s.addresses, address)
	return dev
}

func (s *baseSuite) expectLoopbackNIC(ctrl *gomock.Controller) {
	s.expectDevice(ctrl, "lo", "", network.LoopbackDevice, network.NonVirtualPort)

	address := NewMockAddress(ctrl)
	aExp := address.EXPECT()
	aExp.DeviceName().Return("lo").AnyTimes()

	s.addresses = append(s.addresses, address)
}

func (s *baseSuite) expectBridgeDevice(ctrl *gomock.Controller, dev string) {
	s.expectDevice(ctrl, dev, "", network.BridgeDevice, network.NonVirtualPort)
}

func (s *baseSuite) expectBridgeDeviceWithIP(c *gc.C, ctrl *gomock.Controller, dev, spaceID, cidr string) {
	s.expectDeviceWithIP(c, ctrl, dev, spaceID, network.BridgeDevice, cidr)
}

func (s *baseSuite) expectDeviceWithIP(c *gc.C, ctrl *gomock.Controller, dev, spaceID string, devType network.LinkLayerDeviceType, cidr string) *MockLinkLayerDevice {
	return s.expectDeviceWithParentWithIP(c, ctrl, dev, "", spaceID, devType, cidr)
}

func (s *baseSuite) expectDeviceWithParentWithIP(c *gc.C, ctrl *gomock.Controller, dev, parent, spaceID string, devType network.LinkLayerDeviceType, cidr string) *MockLinkLayerDevice {
	d := s.expectDevice(ctrl, dev, parent, devType, network.NonVirtualPort)

	address := NewMockAddress(ctrl)
	aExp := address.EXPECT()
	aExp.SubnetCIDR().Return(cidr).AnyTimes()
	aExp.DeviceName().Return(dev).AnyTimes()

	// Append the expected subnet.
	s.allSubnets = append(s.allSubnets, network.SubnetInfo{
		CIDR:    cidr,
		SpaceID: spaceID,
	})

	s.addresses = append(s.addresses, address)
	return d
}

func (s *baseSuite) expectDevice(
	ctrl *gomock.Controller, dev, parent string, devType network.LinkLayerDeviceType,
	portType network.VirtualPortType,
) *MockLinkLayerDevice {
	bridgeDevice := NewMockLinkLayerDevice(ctrl)
	bEXP := bridgeDevice.EXPECT()
	bEXP.Name().Return(dev).AnyTimes()
	bEXP.Type().Return(devType).AnyTimes()
	bEXP.ParentName().Return(parent).AnyTimes()
	bEXP.VirtualPortType().Return(portType).AnyTimes()
	bEXP.MACAddress().Return("").AnyTimes()
	bEXP.EthernetDeviceForBridge(ethernetDeviceMatcher{}, false, gomock.Any()).DoAndReturn(func(name string, _ bool, _ network.SubnetInfos) (network.InterfaceInfo, error) {
		return network.InterfaceInfo{
			ParentInterfaceName: dev,
			MACAddress:          "00:16:3e:00:00:00",
			InterfaceName:       name,
			InterfaceType:       network.EthernetDevice,
		}, nil
	}).AnyTimes()
	s.devices = append(s.devices, bridgeDevice)
	return bridgeDevice
}

type ethernetDeviceMatcher struct {
}

func (m ethernetDeviceMatcher) Matches(x interface{}) bool {
	dev, ok := x.(string)
	if !ok {
		return false
	}
	return strings.HasPrefix(dev, "eth")
}

func (ethernetDeviceMatcher) String() string {
	return "matches EthernetDeviceForBridge()"
}
