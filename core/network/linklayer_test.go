// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type linkLayerSuite struct {
	testhelpers.IsolationSuite
}

func TestLinkLayerSuite(t *stdtesting.T) { tc.Run(t, &linkLayerSuite{}) }
func (s *linkLayerSuite) TestIsValidLinkLayerDeviceTypeValid(c *tc.C) {
	validTypes := []LinkLayerDeviceType{
		LoopbackDevice,
		EthernetDevice,
		VLAN8021QDevice,
		BondDevice,
		BridgeDevice,
		VXLANDevice,
	}

	for _, value := range validTypes {
		result := IsValidLinkLayerDeviceType(string(value))
		c.Check(result, tc.IsTrue)
	}
}

func (s *linkLayerSuite) TestIsValidLinkLayerDeviceTypeInvalid(c *tc.C) {
	result := IsValidLinkLayerDeviceType("")
	c.Check(result, tc.IsFalse)

	result = IsValidLinkLayerDeviceType("anything")
	c.Check(result, tc.IsFalse)

	result = IsValidLinkLayerDeviceType(" ")
	c.Check(result, tc.IsFalse)

	result = IsValidLinkLayerDeviceType("unknown")
	c.Check(result, tc.IsFalse)
}

var validUnixDeviceNames = []string{"eth0", "eno1", "br-eth0.123", "tun:1", "bond0.42"}

func (s *linkLayerSuite) TestIsValidLinkLayerDeviceNameValidNamesLinux(c *tc.C) {
	for i, name := range validUnixDeviceNames {
		c.Logf("test #%d: %q -> valid", i, name)
		result := isValidLinkLayerDeviceName(name, "linux")
		c.Check(result, tc.IsTrue)
	}
}

func (s *linkLayerSuite) TestIsValidLinkLayerDeviceNameInvalidNamesLinux(c *tc.C) {
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
		c.Check(result, tc.IsFalse)
	}
}

func (s *linkLayerSuite) TestIsValidLinkLayerDeviceNameValidNamesNonLinux(c *tc.C) {
	validDeviceNames := append(validUnixDeviceNames,
		// Windows network device as friendly name and as underlying UUID.
		"Local Area Connection", "{4a62b748-43d0-4136-92e4-22ce7ee31938}",
	)

	for i, name := range validDeviceNames {
		c.Logf("test #%d: %q -> valid", i, name)
		result := isValidLinkLayerDeviceName(name, "non-linux")
		c.Check(result, tc.IsTrue)
	}
}

func (s *linkLayerSuite) TestIsValidLinkLayerDeviceNameInvalidNamesNonLinux(c *tc.C) {
	os := "non-linux"

	result := isValidLinkLayerDeviceName("", os)
	c.Check(result, tc.IsFalse)

	const wayTooLongLength = 1024
	result = isValidLinkLayerDeviceName(strings.Repeat("x", wayTooLongLength), os)
	c.Check(result, tc.IsFalse)

	result = isValidLinkLayerDeviceName("hash# not allowed", os)
	c.Check(result, tc.IsFalse)
}

func (s *linkLayerSuite) TestStringLengthBetweenWhenTooShort(c *tc.C) {
	result := stringLengthBetween("", 1, 2)
	c.Check(result, tc.IsFalse)

	result = stringLengthBetween("", 1, 1)
	c.Check(result, tc.IsFalse)

	result = stringLengthBetween("1", 2, 3)
	c.Check(result, tc.IsFalse)

	result = stringLengthBetween("12", 3, 3)
	c.Check(result, tc.IsFalse)
}

func (s *linkLayerSuite) TestStringLengthBetweenWhenTooLong(c *tc.C) {
	result := stringLengthBetween("1", 0, 0)
	c.Check(result, tc.IsFalse)

	result = stringLengthBetween("12", 1, 1)
	c.Check(result, tc.IsFalse)

	result = stringLengthBetween("123", 1, 2)
	c.Check(result, tc.IsFalse)

	result = stringLengthBetween("123", 0, 1)
	c.Check(result, tc.IsFalse)
}

func (s *linkLayerSuite) TestStringLengthBetweenWhenWithinLimit(c *tc.C) {
	const (
		minLength = 1
		maxLength = 255
	)
	for i := minLength; i <= maxLength; i++ {
		input := strings.Repeat("x", i)
		result := stringLengthBetween(input, minLength, maxLength)
		c.Check(result, tc.IsTrue)
	}
}
