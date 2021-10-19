// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"runtime"
	"strings"
)

// LinkLayerDeviceType defines the type of a link-layer network device.
type LinkLayerDeviceType string

const (
	// UnknownDevice indicates that the type of this device is not known.
	UnknownDevice LinkLayerDeviceType = ""

	// LoopbackDevice is used for loopback devices.
	LoopbackDevice LinkLayerDeviceType = "loopback"

	// EthernetDevice is used for Ethernet (IEEE 802.3) devices.
	EthernetDevice LinkLayerDeviceType = "ethernet"

	// VLAN8021QDevice is used for IEEE 802.1Q VLAN devices.
	VLAN8021QDevice LinkLayerDeviceType = "802.1q"

	// BondDevice is used for bonding devices.
	BondDevice LinkLayerDeviceType = "bond"

	// BridgeDevice is used for OSI layer-2 bridge devices.
	BridgeDevice LinkLayerDeviceType = "bridge"

	// VXLANDevice is used for Virtual Extensible LAN devices.
	VXLANDevice LinkLayerDeviceType = "vxlan"
)

// IsValidLinkLayerDeviceType returns whether the given value is a valid
// link-layer network device type.
func IsValidLinkLayerDeviceType(value string) bool {
	switch LinkLayerDeviceType(value) {
	case LoopbackDevice, EthernetDevice, VLAN8021QDevice, BondDevice, BridgeDevice, VXLANDevice:
		return true
	}
	return false
}

// IsValidLinkLayerDeviceName returns whether the given name is a valid network
// link-layer device name, depending on the runtime.GOOS value.
func IsValidLinkLayerDeviceName(name string) bool {
	return isValidLinkLayerDeviceName(name, runtime.GOOS)
}

func isValidLinkLayerDeviceName(name string, runtimeOS string) bool {
	if runtimeOS == "linux" {
		return isValidLinuxDeviceName(name)
	}
	hasHash := strings.Contains(name, "#")
	return !hasHash && stringLengthBetween(name, 1, 255)
}

// isValidLinuxDeviceName returns whether the given deviceName is valid,
// using the same criteria as dev_valid_name(9) in the Linux kernel:
// - no whitespace allowed
// - length from 1 to 15 ASCII characters
// - literal "." and ".." as names are not allowed.
// Additionally, we don't allow "#" in the name.
func isValidLinuxDeviceName(name string) bool {
	hasWhitespace := whitespaceReplacer.Replace(name) != name
	isDot, isDoubleDot := name == ".", name == ".."
	hasValidLength := stringLengthBetween(name, 1, 15)
	hasHash := strings.Contains(name, "#")

	return hasValidLength && !(hasHash || hasWhitespace || isDot || isDoubleDot)
}

// whitespaceReplacer strips whitespace characters from the input string.
var whitespaceReplacer = strings.NewReplacer(
	" ", "",
	"\t", "",
	"\v", "",
	"\n", "",
	"\r", "",
)

func stringLengthBetween(value string, minLength, maxLength uint) bool {
	length := uint(len(value))
	return length >= minLength && length <= maxLength
}
