// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/mgo/v2/txn"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/common/networkingcommon/mocks"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// modelOpRecorder is a no-op model operation that can store
// the devices passed to it for later interrogation.
type modelOpRecorder struct {
	devs network.InterfaceInfos
}

func (modelOpRecorder) Build(_ int) ([]txn.Op, error) {
	return nil, nil
}

func (modelOpRecorder) Done(_ error) error {
	return nil
}

type networkConfigSuite struct {
	networkingcommon.BaseSuite

	tag names.MachineTag

	state   *mocks.MockLinkLayerAndSubnetsState
	machine *mocks.MockLinkLayerMachine

	modelOp modelOpRecorder
}

var _ = gc.Suite(&networkConfigSuite{})

func (s *networkConfigSuite) TestSetObservedNetworkConfigMachineNotFoundPermissionError(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.state.EXPECT().Machine("1").Return(nil, errors.NotFoundf("nope"))

	err := s.NewNetworkConfigAPI(s.state, s.getModelOp).SetObservedNetworkConfig(params.SetMachineNetworkConfig{
		Tag:    "machine-1",
		Config: nil,
	})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *networkConfigSuite) TestSetObservedNetworkConfigNoConfigNoApplyOpCall(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectMachine()
	s.callAPI(c, nil)
}

func (s *networkConfigSuite) TestSetObservedNetworkConfigCallsApplyOperation(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectMachine()

	// This basically simulates having no Fan subnets.
	s.state.EXPECT().AllSubnetInfos().Return(nil, nil)
	s.state.EXPECT().ApplyOperation(gomock.Any()).Return(nil)

	s.callAPI(c, []params.NetworkConfig{
		{
			InterfaceName: "lo",
			InterfaceType: "loopback",
			CIDR:          "127.0.0.0/8",
			Addresses: []params.Address{{
				Value: "127.0.0.1",
				Type:  "ipv4",
				Scope: "local-machine",
			}},
		}, {
			InterfaceName: "eth0",
			InterfaceType: "ethernet",
			MACAddress:    "aa:bb:cc:dd:ee:f0",
			CIDR:          "0.10.0.0/24",
			Address:       "0.10.0.2",
		}, {
			InterfaceName: "eth1",
			InterfaceType: "ethernet",
			MACAddress:    "aa:bb:cc:dd:ee:f1",
			CIDR:          "0.20.0.0/24",
			Address:       "0.20.0.2",
		},
	})

	c.Check(s.modelOp.devs, jc.DeepEquals, network.InterfaceInfos{
		{
			InterfaceName: "lo",
			InterfaceType: "loopback",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("127.0.0.1", network.WithCIDR("127.0.0.0/8")).AsProviderAddress(),
			},
			// This is a quirk of the transformation.
			// Due to the way address type is derived, this is not equivalent
			// to the provider address zero-value.
			GatewayAddress: network.NewMachineAddress("").AsProviderAddress(),
		}, {
			InterfaceName: "eth0",
			InterfaceType: "ethernet",
			MACAddress:    "aa:bb:cc:dd:ee:f0",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("0.10.0.2", network.WithCIDR("0.10.0.0/24")).AsProviderAddress(),
			},
			GatewayAddress: network.NewMachineAddress("").AsProviderAddress(),
		}, {
			InterfaceName: "eth1",
			InterfaceType: "ethernet",
			MACAddress:    "aa:bb:cc:dd:ee:f1",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("0.20.0.2", network.WithCIDR("0.20.0.0/24")).AsProviderAddress(),
			},
			GatewayAddress: network.NewMachineAddress("").AsProviderAddress(),
		},
	})
}

func (s *networkConfigSuite) TestSetObservedNetworkConfigFixesFanSubs(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectMachine()

	s.state.EXPECT().AllSubnetInfos().Return(network.SubnetInfos{
		{
			CIDR: "10.10.0.0/16",
			FanInfo: &network.FanCIDRs{
				FanLocalUnderlay: "",
				FanOverlay:       "anything-not-empty",
			},
		},
	}, nil)

	s.state.EXPECT().ApplyOperation(gomock.Any()).Return(nil)

	s.callAPI(c, []params.NetworkConfig{
		{
			InterfaceName: "eth0",
			InterfaceType: "ethernet",
			MACAddress:    "aa:bb:cc:dd:ee:f0",
			Address:       "10.10.10.2",
			Addresses: []params.Address{
				{
					Value: "10.10.10.2",
					CIDR:  "10.0.0.0/8",
					Type:  string(network.IPv4Address),
					Scope: string(network.ScopeCloudLocal),
				},
			},
		},
	})

	c.Check(s.modelOp.devs, jc.DeepEquals, network.InterfaceInfos{
		{
			InterfaceName: "eth0",
			InterfaceType: "ethernet",
			MACAddress:    "aa:bb:cc:dd:ee:f0",
			// Gets the CIDR from the Fan segment.
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("10.10.10.2", network.WithCIDR("10.10.0.0/16")).AsProviderAddress(),
			},
			GatewayAddress: network.NewMachineAddress("").AsProviderAddress(),
		},
	})
}

func (s *networkConfigSuite) TestUpdateMachineLinkLayerOpMultipleAddressSuccess(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Loopback device already exists with an address.
	// It will be unchanged and generate no update ops.
	lbDev := mocks.NewMockLinkLayerDevice(ctrl)
	lbExp := lbDev.EXPECT()
	lbExp.Name().Return("lo").MinTimes(1)
	lbExp.UpdateOps(state.LinkLayerDeviceArgs{
		Name:        "lo",
		Type:        "loopback",
		IsAutoStart: true,
		IsUp:        true,
	}).Return(nil)

	lbAddr := mocks.NewMockLinkLayerAddress(ctrl)
	lbAddrExp := lbAddr.EXPECT()
	lbAddrExp.DeviceName().Return("lo").MinTimes(1)
	lbAddrExp.Value().Return("127.0.0.1")
	lbAddrExp.UpdateOps(state.LinkLayerDeviceAddress{
		DeviceName:   "lo",
		ConfigMethod: "static",
		CIDRAddress:  "127.0.0.1/8",
	}).Return(nil, nil)

	// Device eth0 exists with no addresses and will have one added to it.
	ethMAC := "aa:bb:cc:dd:ee:f0"
	ethDev := mocks.NewMockLinkLayerDevice(ctrl)
	ethExp := ethDev.EXPECT()
	ethExp.Name().Return("eth0").MinTimes(1)
	ethExp.UpdateOps(state.LinkLayerDeviceArgs{
		Name:        "eth0",
		Type:        "ethernet",
		MACAddress:  ethMAC,
		IsAutoStart: true,
		IsUp:        true,
	}).Return(nil)
	ethExp.AddAddressOps(state.LinkLayerDeviceAddress{
		DeviceName:       "eth0",
		ConfigMethod:     "static",
		CIDRAddress:      "0.10.0.2/24",
		GatewayAddress:   "0.10.0.1",
		IsDefaultGateway: true,
	}).Return([]txn.Op{{}}, nil)

	// Device eth99 exists with an address.
	// Being unobserved in the incoming info, both the device and its
	// address will be deleted.
	delDev := mocks.NewMockLinkLayerDevice(ctrl)
	delExp := delDev.EXPECT()
	delExp.MACAddress().Return("aa:aa:aa:aa:aa:aa").MinTimes(1)
	delExp.Name().Return("eth99").MinTimes(1)
	delExp.ProviderID().Return(network.Id(""))
	delExp.RemoveOps().Return([]txn.Op{{}})

	delAddr := mocks.NewMockLinkLayerAddress(ctrl)
	delAddrExp := delAddr.EXPECT()
	delAddrExp.DeviceName().Return("eth99").MinTimes(1)
	delAddrExp.Origin().Return(network.OriginMachine)
	delAddrExp.Value().Return("10.0.0.1")
	delAddrExp.RemoveOps().Return([]txn.Op{{}})

	s.expectMachine()
	mExp := s.machine.EXPECT()
	mExp.AllLinkLayerDevices().Return([]networkingcommon.LinkLayerDevice{lbDev, ethDev, delDev}, nil)
	mExp.AllDeviceAddresses().Return([]networkingcommon.LinkLayerAddress{lbAddr, delAddr}, nil)
	mExp.AddLinkLayerDeviceOps(
		state.LinkLayerDeviceArgs{
			Name:        "eth1",
			Type:        "ethernet",
			MACAddress:  "aa:bb:cc:dd:ee:f1",
			IsAutoStart: true,
			IsUp:        true,
		},
		state.LinkLayerDeviceAddress{
			DeviceName:     "eth1",
			ConfigMethod:   "static",
			CIDRAddress:    "0.20.0.2/24",
			GatewayAddress: "0.20.0.1",
		},
	).Return([]txn.Op{{}, {}}, nil)

	op := s.NewUpdateMachineLinkLayerOp(s.machine, network.InterfaceInfos{
		{
			// Existing device and address.
			InterfaceName: "lo",
			InterfaceType: "loopback",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("127.0.0.1", network.WithCIDR("127.0.0.0/8")).AsProviderAddress(),
			},
		}, {
			// Existing device with new address.
			InterfaceName: "eth0",
			InterfaceType: "ethernet",
			MACAddress:    ethMAC,
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("0.10.0.2", network.WithCIDR("0.10.0.0/24")).AsProviderAddress(),
			},
			GatewayAddress:   network.NewMachineAddress("0.10.0.1").AsProviderAddress(),
			IsDefaultGateway: true,
		}, {
			// New device and addresses for eth1.
			InterfaceName: "eth1",
			InterfaceType: "ethernet",
			MACAddress:    "aa:bb:cc:dd:ee:f1",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("0.20.0.2", network.WithCIDR("0.20.0.0/24")).AsProviderAddress(),
			},
			GatewayAddress: network.NewMachineAddress("0.20.0.1").AsProviderAddress(),
		}, {
			// A duplicate is effectively ignored.
			InterfaceName: "eth1",
			InterfaceType: "ethernet",
			MACAddress:    "aa:bb:cc:dd:ee:f1",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("0.20.0.2", network.WithCIDR("0.20.0.0/24")).AsProviderAddress(),
			},
			GatewayAddress: network.NewMachineAddress("0.20.0.1").AsProviderAddress(),
		},
	}, false, s.state)

	ops, err := op.Build(0)
	c.Assert(err, jc.ErrorIsNil)

	// No ops for the unchanged device/address.
	// One each for:
	// - Adding an address to the existing device.
	// - Adding a new device.
	// - Adding a new address to the new device.
	// - Deleting the address from the unobserved device.
	// - Deleting the unobserved device.
	c.Check(ops, gc.HasLen, 5)
}

func (s *networkConfigSuite) TestUpdateMachineLinkLayerOpUnobservedParentNotRemoved(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Device eth99 exists with an address.
	// The address will be deleted.
	// The device is a parent of an incoming device,
	// and therefore will not be deleted.
	delDev := mocks.NewMockLinkLayerDevice(ctrl)
	delExp := delDev.EXPECT()
	delExp.MACAddress().Return("aa:aa:aa:aa:aa:aa").MinTimes(1)
	delExp.Name().Return("eth99").MinTimes(1)
	delExp.ProviderID().Return(network.Id(""))

	delAddr := mocks.NewMockLinkLayerAddress(ctrl)
	delAddrExp := delAddr.EXPECT()
	delAddrExp.DeviceName().Return("eth99").MinTimes(1)
	delAddrExp.Origin().Return(network.OriginMachine)
	delAddrExp.Value().Return("10.0.0.1")
	delAddrExp.RemoveOps().Return([]txn.Op{{}})

	s.expectMachine()
	mExp := s.machine.EXPECT()
	mExp.AllLinkLayerDevices().Return([]networkingcommon.LinkLayerDevice{delDev}, nil)
	mExp.AllDeviceAddresses().Return([]networkingcommon.LinkLayerAddress{delAddr}, nil)
	mExp.AddLinkLayerDeviceOps(
		state.LinkLayerDeviceArgs{
			Name:        "eth1",
			Type:        "ethernet",
			MACAddress:  "aa:bb:cc:dd:ee:f1",
			IsAutoStart: true,
			IsUp:        true,
			ParentName:  "eth99",
		},
		state.LinkLayerDeviceAddress{
			DeviceName:     "eth1",
			ConfigMethod:   "static",
			CIDRAddress:    "0.20.0.2/24",
			GatewayAddress: "0.20.0.1",
			Origin:         network.OriginMachine,
		},
	).Return([]txn.Op{{}, {}}, nil)

	op := s.NewUpdateMachineLinkLayerOp(s.machine, network.InterfaceInfos{
		{
			// New device and addresses for eth1 with eth99 as the parent.
			InterfaceName: "eth1",
			InterfaceType: "ethernet",
			MACAddress:    "aa:bb:cc:dd:ee:f1",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("0.20.0.2", network.WithCIDR("0.20.0.0/24")).AsProviderAddress(),
			},
			GatewayAddress:      network.NewMachineAddress("0.20.0.1").AsProviderAddress(),
			ParentInterfaceName: "eth99",
			Origin:              network.OriginMachine,
		},
	}, false, s.state)

	_, err := op.Build(0)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkConfigSuite) TestUpdateMachineLinkLayerOpNewSubnetsAdded(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// A machine with no link-layer data.
	s.expectMachine()
	mExp := s.machine.EXPECT()
	mExp.AllLinkLayerDevices().Return(nil, nil)
	mExp.AllDeviceAddresses().Return(nil, nil)

	// We expect 3 devices and their addresses to be added.
	mExp.AddLinkLayerDeviceOps(gomock.Any(), gomock.Any()).Return([]txn.Op{{}, {}}, nil).Times(3)

	// Simulate the first being added, and the 2nd already existing.
	// There will be no addition of the loopback subnet.
	s.state.EXPECT().AddSubnetOps(network.SubnetInfo{CIDR: "0.10.0.0/24"}).Return([]txn.Op{{}}, nil)
	s.state.EXPECT().AddSubnetOps(network.SubnetInfo{CIDR: "0.20.0.0/24"}).Return(nil, errors.AlreadyExistsf("blat"))

	op := s.NewUpdateMachineLinkLayerOp(s.machine, network.InterfaceInfos{
		{
			InterfaceName: "lo",
			InterfaceType: "loopback",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("127.0.0.1", network.WithCIDR("127.0.0.0/8")).AsProviderAddress(),
			},
		}, {
			InterfaceName: "eth0",
			InterfaceType: "ethernet",
			MACAddress:    "aa:bb:cc:dd:ee:ff",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("0.10.0.2", network.WithCIDR("0.10.0.0/24")).AsProviderAddress(),
			},
			GatewayAddress:   network.NewMachineAddress("0.10.0.1").AsProviderAddress(),
			IsDefaultGateway: true,
		}, {
			InterfaceName: "eth1",
			InterfaceType: "ethernet",
			MACAddress:    "aa:bb:cc:dd:ee:f1",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("0.20.0.2", network.WithCIDR("0.20.0.0/24")).AsProviderAddress(),
			},
			GatewayAddress: network.NewMachineAddress("0.20.0.1").AsProviderAddress(),
		},
	}, true, s.state)

	ops, err := op.Build(0)
	c.Assert(err, jc.ErrorIsNil)

	// Expected ops are:
	// - One each for the 3 new devices.
	// - One each for the 3 new device addresses.
	// - One for the not-yet-seen/non-loopback subnet.
	c.Check(ops, gc.HasLen, 7)
}

func (s *networkConfigSuite) TestUpdateMachineLinkLayerOpBridgedDeviceMovesAddress(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	hwAddr := "aa:bb:cc:dd:ee:ff"

	// Device eth0 exists with an address.
	childDev := mocks.NewMockLinkLayerDevice(ctrl)
	childExp := childDev.EXPECT()
	childExp.Name().Return("eth0").MinTimes(1)

	// We expect an update with the bridge as parent.
	childExp.UpdateOps(state.LinkLayerDeviceArgs{
		Name:        "eth0",
		Type:        "ethernet",
		MACAddress:  hwAddr,
		IsAutoStart: true,
		IsUp:        true,
		ParentName:  "br-eth0",
	}).Return([]txn.Op{{}})

	// We expect the eth0 address to be removed.
	childAddr := mocks.NewMockLinkLayerAddress(ctrl)
	childAddrExp := childAddr.EXPECT()
	childAddrExp.DeviceName().Return("eth0").MinTimes(1)
	childAddrExp.Origin().Return(network.OriginMachine)
	childAddrExp.Value().Return("10.0.0.5")
	childAddrExp.RemoveOps().Return([]txn.Op{{}})

	s.expectMachine()
	mExp := s.machine.EXPECT()
	mExp.AllLinkLayerDevices().Return([]networkingcommon.LinkLayerDevice{childDev}, nil)
	mExp.AllDeviceAddresses().Return([]networkingcommon.LinkLayerAddress{childAddr}, nil)
	mExp.AddLinkLayerDeviceOps(
		state.LinkLayerDeviceArgs{
			Name:        "br-eth0",
			Type:        "bridge",
			MACAddress:  hwAddr,
			IsAutoStart: true,
			IsUp:        true,
		},
		state.LinkLayerDeviceAddress{
			DeviceName:     "br-eth0",
			ConfigMethod:   "static",
			CIDRAddress:    "10.0.0.6/24",
			GatewayAddress: "10.0.0.1",
			Origin:         network.OriginMachine,
		},
	).Return([]txn.Op{{}, {}}, nil)

	// Device eth0 becomes bridged.
	// It no longer has an address, but has the bridge as its parent.
	// The parent device (same MAC) has the IP address.
	op := s.NewUpdateMachineLinkLayerOp(s.machine, network.InterfaceInfos{
		{
			InterfaceName:       "eth0",
			InterfaceType:       "ethernet",
			MACAddress:          hwAddr,
			ParentInterfaceName: "br-eth0",
			Origin:              network.OriginMachine,
		},
		{
			InterfaceName: "br-eth0",
			InterfaceType: "bridge",
			MACAddress:    hwAddr,
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("10.0.0.6", network.WithCIDR("10.0.0.0/24")).AsProviderAddress(),
			},
			GatewayAddress: network.NewMachineAddress("10.0.0.1").AsProviderAddress(),
			Origin:         network.OriginMachine,
		},
	}, false, s.state)

	_, err := op.Build(0)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkConfigSuite) TestUpdateMachineLinkLayerOpReprocessesDevices(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	hwAddr := "aa:bb:cc:dd:ee:ff"

	s.expectMachine()
	mExp := s.machine.EXPECT()
	mExp.AllLinkLayerDevices().Return(nil, nil).Times(2)
	mExp.AllDeviceAddresses().Return(nil, nil).Times(2)

	// Expect the device addition to be attempted twice.
	mExp.AddLinkLayerDeviceOps(
		state.LinkLayerDeviceArgs{
			Name:        "eth0",
			Type:        "ethernet",
			MACAddress:  hwAddr,
			IsAutoStart: true,
			IsUp:        true,
		},
	).Return([]txn.Op{{}, {}}, nil).Times(2)

	op := s.NewUpdateMachineLinkLayerOp(s.machine, network.InterfaceInfos{
		{
			InterfaceName: "eth0",
			InterfaceType: "ethernet",
			MACAddress:    hwAddr,
			Origin:        network.OriginMachine,
		},
	}, false, s.state)

	_, err := op.Build(0)
	c.Assert(err, jc.ErrorIsNil)

	// Simulate transaction churn.
	_, err = op.Build(1)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkConfigSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.machine = mocks.NewMockLinkLayerMachine(ctrl)
	s.state = mocks.NewMockLinkLayerAndSubnetsState(ctrl)

	return ctrl
}

func (s *networkConfigSuite) expectMachine() {
	s.tag = names.NewMachineTag("0")
	s.machine.EXPECT().Id().Return(s.tag.Id()).AnyTimes()
	s.machine.EXPECT().ModelUUID().Return("some-model-uuid").AnyTimes()
	s.state.EXPECT().Machine(s.tag.Id()).Return(s.machine, nil).AnyTimes()
}

func (s *networkConfigSuite) callAPI(c *gc.C, config []params.NetworkConfig) {
	c.Assert(s.NewNetworkConfigAPI(s.state, s.getModelOp).SetObservedNetworkConfig(params.SetMachineNetworkConfig{
		Tag:    s.tag.String(),
		Config: config,
	}), jc.ErrorIsNil)
}

func (s *networkConfigSuite) getModelOp(
	_ networkingcommon.LinkLayerMachine, devs network.InterfaceInfos,
) state.ModelOperation {
	s.modelOp.devs = devs
	return s.modelOp
}
