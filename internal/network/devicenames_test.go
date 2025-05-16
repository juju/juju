// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/network"
	"github.com/juju/juju/internal/testhelpers"
)

type DeviceNamesSuite struct {
	testhelpers.IsolationSuite
}

func TestDeviceNamesSuite(t *stdtesting.T) { tc.Run(t, &DeviceNamesSuite{}) }
func (*DeviceNamesSuite) TestNaturallySortDeviceNames(c *tc.C) {
	for i, test := range []struct {
		message  string
		input    []string
		expected []string
	}{{
		message:  "empty input, empty output",
		input:    []string{},
		expected: []string{},
	}, {
		message: "nil input, nil output",
	}, {
		message:  "one input",
		input:    []string{"a"},
		expected: []string{"a"},
	}, {
		message:  "two values, no numbers",
		input:    []string{"b", "a"},
		expected: []string{"a", "b"},
	}, {
		message:  "two values, mixed content",
		input:    []string{"b1", "a1"},
		expected: []string{"a1", "b1"},
	}, {
		message:  "identical values, numbers only",
		input:    []string{"1", "1", "1", "1"},
		expected: []string{"1", "1", "1", "1"},
	}, {
		message:  "identical values, mixed content",
		input:    []string{"a1", "a1", "a1", "a1"},
		expected: []string{"a1", "a1", "a1", "a1"},
	}, {
		message:  "reversed input",
		input:    []string{"a10", "a9", "a8", "a7", "a6", "a5", "a4", "a3", "a2", "a1", "a0"},
		expected: []string{"a0", "a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10"},
	}, {
		message:  "multiple numbers per value",
		input:    []string{"a10.11", "a10.10", "a10.1"},
		expected: []string{"a10.1", "a10.10", "a10.11"},
	}, {
		message:  "value with leading zero",
		input:    []string{"a50", "a51.", "a50.31", "a50.4", "a5.034e1", "a50.300"},
		expected: []string{"a5.034e1", "a50", "a50.4", "a50.31", "a50.300", "a51."},
	}, {
		message:  "value with multiple leading zeros",
		input:    []string{"a50", "a51.", "a0050.31", "a50.4", "a5.034e1", "a00050.300"},
		expected: []string{"a00050.300", "a0050.31", "a5.034e1", "a50", "a50.4", "a51."},
	}, {
		message:  "strings with numbers in ascending order",
		input:    []string{"a2", "a5", "a9", "a1", "a4", "a10", "a6"},
		expected: []string{"a1", "a2", "a4", "a5", "a6", "a9", "a10"},
	}, {
		message:  "values that look like version numbers",
		input:    []string{"1.9.9a", "1.11", "1.9.9b", "1.11.4", "1.10.1"},
		expected: []string{"1.9.9a", "1.9.9b", "1.10.1", "1.11", "1.11.4"},
	}, {
		message:  "bridge device names",
		input:    []string{"br-eth10", "br-eth2", "br-eth1"},
		expected: []string{"br-eth1", "br-eth2", "br-eth10"},
	}, {
		message:  "bridge device names with VLAN numbers",
		input:    []string{"br-eth10.10", "br-eth2.10", "br-eth200", "br-eth1.100", "br-eth1.10"},
		expected: []string{"br-eth1.10", "br-eth1.100", "br-eth2.10", "br-eth10.10", "br-eth200"},
	}, {
		message:  "bridge device names with leading zero",
		input:    []string{"br-eth0", "br-eth10.10", "br-eth2.10", "br-eth1.100", "br-eth1.10", "br-eth10"},
		expected: []string{"br-eth0", "br-eth1.10", "br-eth1.100", "br-eth2.10", "br-eth10", "br-eth10.10"},
	}} {
		c.Logf("%v: %s", i, test.message)
		result := network.NaturallySortDeviceNames(test.input...)
		c.Assert(result, tc.HasLen, len(test.input))
		c.Assert(result, tc.DeepEquals, test.expected)
	}
}
