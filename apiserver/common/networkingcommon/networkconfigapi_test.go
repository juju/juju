// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/common/networkingcommon/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

// modelOpStub is a no-op model operation that can store the
// devices passed to to it for later interrogation.
type modelOpStub struct {
	devs network.InterfaceInfos
}

func (modelOpStub) Build(_ int) ([]txn.Op, error) {
	return nil, nil
}

func (modelOpStub) Done(_ error) error {
	return nil
}

type networkConfigSuite struct {
	networkingcommon.BaseSuite

	tag names.MachineTag

	state   *mocks.MockLinkLayerState
	machine *mocks.MockLinkLayerMachine

	modelOp modelOpStub
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
			CIDR:          "127.0.0.0/8",
			Addresses:     network.NewProviderAddresses("127.0.0.1"),
			// This is a quirk of the transformation.
			// Due to the way address type is derived, this is not equivalent
			// to the provider address zero-value.
			GatewayAddress: network.NewProviderAddress(""),
		}, {
			InterfaceName:  "eth0",
			InterfaceType:  "ethernet",
			MACAddress:     "aa:bb:cc:dd:ee:f0",
			CIDR:           "0.10.0.0/24",
			Addresses:      network.NewProviderAddresses("0.10.0.2"),
			GatewayAddress: network.NewProviderAddress(""),
		}, {
			InterfaceName:  "eth1",
			InterfaceType:  "ethernet",
			MACAddress:     "aa:bb:cc:dd:ee:f1",
			CIDR:           "0.20.0.0/24",
			Addresses:      network.NewProviderAddresses("0.20.0.2"),
			GatewayAddress: network.NewProviderAddress(""),
		},
	})
}

func (s *networkConfigSuite) TestSetObservedNetworkConfigFixesFanSubs(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectMachine()

	s.state.EXPECT().AllSubnetInfos().Return(network.SubnetInfos{
		{
			CIDR: "0.10.0.0/16",
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
			CIDR:          "0.10.1.0/24",
			Address:       "0.10.0.2",
		},
	})

	c.Check(s.modelOp.devs, jc.DeepEquals, network.InterfaceInfos{
		{
			InterfaceName: "eth0",
			InterfaceType: "ethernet",
			MACAddress:    "aa:bb:cc:dd:ee:f0",
			// Gets the CIDR from the Fan segment.
			CIDR:           "0.10.0.0/16",
			Addresses:      network.NewProviderAddresses("0.10.0.2"),
			GatewayAddress: network.NewProviderAddress(""),
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
	lbExp.MACAddress().Return("").MinTimes(1)
	lbExp.ParentID().Return("")
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
	ethExp.MACAddress().Return(ethMAC).MinTimes(1)
	ethExp.ParentID().Return("")
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
	delExp.ID().Return("some-model-uuid:m#0#eth99")
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
	mExp.AllAddresses().Return([]networkingcommon.LinkLayerAddress{lbAddr, delAddr}, nil)
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
			CIDR:          "127.0.0.0/8",
			Addresses:     network.NewProviderAddresses("127.0.0.1"),
		}, {
			// Existing device with new address.
			InterfaceName:    "eth0",
			InterfaceType:    "ethernet",
			MACAddress:       ethMAC,
			CIDR:             "0.10.0.0/24",
			Addresses:        network.NewProviderAddresses("0.10.0.2"),
			GatewayAddress:   network.NewProviderAddress("0.10.0.1"),
			IsDefaultGateway: true,
		}, {
			// New device and addresses for eth1.
			InterfaceName:  "eth1",
			InterfaceType:  "ethernet",
			MACAddress:     "aa:bb:cc:dd:ee:f1",
			CIDR:           "0.20.0.0/24",
			Addresses:      network.NewProviderAddresses("0.20.0.2"),
			GatewayAddress: network.NewProviderAddress("0.20.0.1"),
		}, {
			// A duplicate is effectively ignored.
			InterfaceName:  "eth1",
			InterfaceType:  "ethernet",
			MACAddress:     "aa:bb:cc:dd:ee:f1",
			CIDR:           "0.20.0.0/24",
			Addresses:      network.NewProviderAddresses("0.20.0.2"),
			GatewayAddress: network.NewProviderAddress("0.20.0.1"),
		},
	})

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

func (s *networkConfigSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.machine = mocks.NewMockLinkLayerMachine(ctrl)
	s.state = mocks.NewMockLinkLayerState(ctrl)

	return ctrl
}

func (s *networkConfigSuite) expectMachine() {
	s.tag = names.NewMachineTag("0")
	s.machine.EXPECT().Id().Return(s.tag.Id()).AnyTimes()
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
