// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type LocusSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LocusSuite{})

func (s *LocusSuite) TestLocusAdd(c *gc.C) {
	locus := NewLocus()

	interfaceInfo0 := InterfaceInfo{}
	err := locus.Add(OriginMachine, interfaceInfo0)
	c.Assert(err, jc.ErrorIsNil)

	err = locus.Add(OriginUnknown, interfaceInfo0)
	c.Assert(err, gc.ErrorMatches, `unexpected origin: ""`)

	interfaceInfo1 := InterfaceInfo{}
	err = locus.Add(OriginProvider, interfaceInfo1)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(locus, gc.DeepEquals, &Locus{
		machines:  []InterfaceInfo{interfaceInfo0},
		providers: []InterfaceInfo{interfaceInfo1},
	})
}

func (s *LocusSuite) TestLocusMachineAddresses(c *gc.C) {
	locus := NewLocus()

	address := NewProviderAddress("10.0.0.0")

	interfaceInfo0 := InterfaceInfo{
		Addresses: ProviderAddresses{address},
	}
	err := locus.Add(OriginMachine, interfaceInfo0)
	c.Assert(err, jc.ErrorIsNil)

	addresses := locus.MachineAddresses()
	c.Assert(addresses, gc.HasLen, 1)
	c.Assert(addresses[0], gc.DeepEquals, ProviderAddresses{address})

	addresses = locus.ProviderAddresses()
	c.Assert(addresses, gc.HasLen, 0)
}

func (s *LocusSuite) TestLocusProviderAddresses(c *gc.C) {
	locus := NewLocus()

	address := NewProviderAddress("10.0.0.0")

	interfaceInfo0 := InterfaceInfo{
		Addresses: ProviderAddresses{address},
	}
	err := locus.Add(OriginProvider, interfaceInfo0)
	c.Assert(err, jc.ErrorIsNil)

	addresses := locus.ProviderAddresses()
	c.Assert(addresses, gc.HasLen, 1)
	c.Assert(addresses[0], gc.DeepEquals, ProviderAddresses{address})

	addresses = locus.MachineAddresses()
	c.Assert(addresses, gc.HasLen, 0)
}

func (s *LocusSuite) TestNewLocusFromLinkLayerDevices(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockLinkLayerDeviceAddress := NewMockLinkLayerDeviceAddress(ctrl)
	mockLinkLayerDeviceAddress.EXPECT().Origin().Return(OriginProvider).Times(2)
	mockLinkLayerDeviceAddress.EXPECT().Value().Return("10.0.0.0")

	addresses := []LinkLayerDeviceAddress{
		mockLinkLayerDeviceAddress,
	}

	mockLinkLayerDevice := NewMockLinkLayerDevice(ctrl)
	mockLinkLayerDevice.EXPECT().ProviderID().Return(Id("provider-1"))
	mockLinkLayerDevice.EXPECT().Addresses().Return(addresses, nil)

	devices := []LinkLayerDevice{
		mockLinkLayerDevice,
	}

	locus, err := NewLocusFromLinkLayerDevices(devices)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(locus, gc.NotNil)
}

func (s *LocusSuite) TestNewLocusFromLinkLayerDevicesWithNonHomogeneousAddresses(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockLinkLayerDeviceAddress0 := NewMockLinkLayerDeviceAddress(ctrl)
	mockLinkLayerDeviceAddress0.EXPECT().Origin().Return(OriginProvider).Times(2)
	mockLinkLayerDeviceAddress0.EXPECT().Value().Return("10.0.0.0")

	mockLinkLayerDeviceAddress1 := NewMockLinkLayerDeviceAddress(ctrl)
	mockLinkLayerDeviceAddress1.EXPECT().Origin().Return(OriginMachine)

	addresses := []LinkLayerDeviceAddress{
		mockLinkLayerDeviceAddress0,
		mockLinkLayerDeviceAddress1,
	}

	mockLinkLayerDevice := NewMockLinkLayerDevice(ctrl)
	mockLinkLayerDevice.EXPECT().Addresses().Return(addresses, nil)

	devices := []LinkLayerDevice{
		mockLinkLayerDevice,
	}

	_, err := NewLocusFromLinkLayerDevices(devices)
	c.Assert(err, gc.ErrorMatches, "expected homogeneous link-layer device addresses")
}
