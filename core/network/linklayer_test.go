// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type linkLayerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&linkLayerSuite{})

func (s *linkLayerSuite) TestIsValidLinkLayerDeviceTypeValid(c *gc.C) {
	validTypes := []LinkLayerDeviceType{
		LoopbackDevice,
		EthernetDevice,
		VLAN8021QDevice,
		BondDevice,
		BridgeDevice,
	}

	for _, value := range validTypes {
		result := IsValidLinkLayerDeviceType(string(value))
		c.Check(result, jc.IsTrue)
	}
}

func (s *linkLayerSuite) TestIsValidLinkLayerDeviceTypeInvalid(c *gc.C) {
	result := IsValidLinkLayerDeviceType("")
	c.Check(result, jc.IsFalse)

	result = IsValidLinkLayerDeviceType("anything")
	c.Check(result, jc.IsFalse)

	result = IsValidLinkLayerDeviceType(" ")
	c.Check(result, jc.IsFalse)

	result = IsValidLinkLayerDeviceType("unknown")
	c.Check(result, jc.IsFalse)
}

var validUnixDeviceNames = []string{"eth0", "eno1", "br-eth0.123", "tun:1", "bond0.42"}

func (s *linkLayerSuite) TestIsValidLinkLayerDeviceNameValidNamesLinux(c *gc.C) {
	for i, name := range validUnixDeviceNames {
		c.Logf("test #%d: %q -> valid", i, name)
		result := isValidLinkLayerDeviceName(name, "linux")
		c.Check(result, jc.IsTrue)
	}
}

func (s *linkLayerSuite) TestIsValidLinkLayerDeviceNameInvalidNamesLinux(c *gc.C) {
	for i, name := range []string{
		"",
		strings.Repeat("x", 16),
		"with-hash#",
		"has spaces",
		"has\tabs",
		"has\newline",
		"has\r",
		"has\vtab",
		".",
		"..",
	} {
		c.Logf("test #%d: %q -> invalid", i, name)
		result := isValidLinkLayerDeviceName(name, "linux")
		c.Check(result, jc.IsFalse)
	}
}

func (s *linkLayerSuite) TestIsValidLinkLayerDeviceNameValidNamesNonLinux(c *gc.C) {
	validDeviceNames := append(validUnixDeviceNames,
		// Windows network device as friendly name and as underlying UUID.
		"Local Area Connection", "{4a62b748-43d0-4136-92e4-22ce7ee31938}",
	)

	for i, name := range validDeviceNames {
		c.Logf("test #%d: %q -> valid", i, name)
		result := isValidLinkLayerDeviceName(name, "non-linux")
		c.Check(result, jc.IsTrue)
	}
}

func (s *linkLayerSuite) TestIsValidLinkLayerDeviceNameInvalidNamesNonLinux(c *gc.C) {
	os := "non-linux"

	result := isValidLinkLayerDeviceName("", os)
	c.Check(result, jc.IsFalse)

	const wayTooLongLength = 1024
	result = isValidLinkLayerDeviceName(strings.Repeat("x", wayTooLongLength), os)
	c.Check(result, jc.IsFalse)

	result = isValidLinkLayerDeviceName("hash# not allowed", os)
	c.Check(result, jc.IsFalse)
}

func (s *linkLayerSuite) TestStringLengthBetweenWhenTooShort(c *gc.C) {
	result := stringLengthBetween("", 1, 2)
	c.Check(result, jc.IsFalse)

	result = stringLengthBetween("", 1, 1)
	c.Check(result, jc.IsFalse)

	result = stringLengthBetween("1", 2, 3)
	c.Check(result, jc.IsFalse)

	result = stringLengthBetween("12", 3, 3)
	c.Check(result, jc.IsFalse)
}

func (s *linkLayerSuite) TestStringLengthBetweenWhenTooLong(c *gc.C) {
	result := stringLengthBetween("1", 0, 0)
	c.Check(result, jc.IsFalse)

	result = stringLengthBetween("12", 1, 1)
	c.Check(result, jc.IsFalse)

	result = stringLengthBetween("123", 1, 2)
	c.Check(result, jc.IsFalse)

	result = stringLengthBetween("123", 0, 1)
	c.Check(result, jc.IsFalse)
}

func (s *linkLayerSuite) TestStringLengthBetweenWhenWithinLimit(c *gc.C) {
	const (
		minLength = 1
		maxLength = 255
	)
	for i := minLength; i <= maxLength; i++ {
		input := strings.Repeat("x", i)
		result := stringLengthBetween(input, minLength, maxLength)
		c.Check(result, jc.IsTrue)
	}
}

func (s *linkLayerSuite) TestIsValidAddressConfigMethodWithValidValues(c *gc.C) {
	validTypes := []AddressConfigMethod{
		LoopbackAddress,
		StaticAddress,
		DynamicAddress,
		ManualAddress,
	}

	for _, value := range validTypes {
		result := IsValidAddressConfigMethod(string(value))
		c.Check(result, jc.IsTrue)
	}
}

func (s *linkLayerSuite) TestIsValidAddressConfigMethodWithInvalidValues(c *gc.C) {
	result := IsValidAddressConfigMethod("")
	c.Check(result, jc.IsFalse)

	result = IsValidAddressConfigMethod("anything")
	c.Check(result, jc.IsFalse)

	result = IsValidAddressConfigMethod(" ")
	c.Check(result, jc.IsFalse)
}
