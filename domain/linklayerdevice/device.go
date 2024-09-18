// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package linklayerdevice

import corenetwork "github.com/juju/juju/core/network"

// DeviceType represents the type
// of a link layer device, as recorded in the
// link_layer_device_type lookup table.
type DeviceType int

const (
	DeviceTypeUnknown DeviceType = iota
	DeviceTypeLoopback
	DeviceTypeEthernet
	DeviceType8021q
	DeviceTypeBond
	DeviceTypeBridge
	DeviceTypeVXLAN
)

// MarshallDeviceType converts a link layer device type to a db link layer device type id.
func MarshallDeviceType(deviceType corenetwork.LinkLayerDeviceType) DeviceType {
	switch deviceType {
	case corenetwork.UnknownDevice:
		return DeviceTypeUnknown
	case corenetwork.LoopbackDevice:
		return DeviceTypeLoopback
	case corenetwork.EthernetDevice:
		return DeviceTypeEthernet
	case corenetwork.VLAN8021QDevice:
		return DeviceType8021q
	case corenetwork.BondDevice:
		return DeviceTypeBond
	case corenetwork.BridgeDevice:
		return DeviceTypeBridge
	case corenetwork.VXLANDevice:
		return DeviceTypeVXLAN
	}
	return DeviceTypeUnknown
}

// VirtualPortType represents the type
// of a link layer device port, as recorded
// in the virtual_port_type lookup table.
type VirtualPortType int

const (
	NonVirtualPortType VirtualPortType = iota
	OpenVswitchVirtualPortType
)

// MarshallVirtualPortType converts a virtual port type to a db virtual port type id.
func MarshallVirtualPortType(portType corenetwork.VirtualPortType) VirtualPortType {
	switch portType {
	case corenetwork.NonVirtualPort:
		return NonVirtualPortType
	case corenetwork.OvsPort:
		return OpenVswitchVirtualPortType
	}
	return NonVirtualPortType
}
