// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type PortRangeSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&PortRangeSuite{})

func (*PortRangeSuite) TestConflictsWith(c *gc.C) {
	var testCases = []struct {
		about          string
		first          network.PortRange
		second         network.PortRange
		expectConflict bool
	}{{
		"identical ports",
		network.PortRange{80, 80, "TCP"},
		network.PortRange{80, 80, "TCP"},
		true,
	}, {
		"different ports",
		network.PortRange{80, 80, "TCP"},
		network.PortRange{90, 90, "TCP"},
		false,
	}, {
		"touching ranges",
		network.PortRange{100, 200, "TCP"},
		network.PortRange{201, 240, "TCP"},
		false,
	}, {
		"touching ranges with overlap",
		network.PortRange{100, 200, "TCP"},
		network.PortRange{200, 240, "TCP"},
		true,
	}, {
		"different protocols",
		network.PortRange{80, 80, "UDP"},
		network.PortRange{80, 80, "TCP"},
		false,
	}, {
		"outside range",
		network.PortRange{100, 200, "TCP"},
		network.PortRange{80, 80, "TCP"},
		false,
	}, {
		"overlap end",
		network.PortRange{100, 200, "TCP"},
		network.PortRange{80, 120, "TCP"},
		true,
	}, {
		"complete overlap",
		network.PortRange{100, 200, "TCP"},
		network.PortRange{120, 140, "TCP"},
		true,
	}}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		c.Check(t.first.ConflictsWith(t.second), gc.Equals, t.expectConflict)
		c.Check(t.second.ConflictsWith(t.first), gc.Equals, t.expectConflict)
	}
}

func (*PortRangeSuite) TestStrings(c *gc.C) {
	c.Assert(
		network.PortRange{80, 80, "TCP"}.String(),
		gc.Equals,
		"80/tcp",
	)
	c.Assert(
		network.PortRange{80, 80, "TCP"}.GoString(),
		gc.Equals,
		"80/tcp",
	)
	c.Assert(
		network.PortRange{80, 100, "TCP"}.String(),
		gc.Equals,
		"80-100/tcp",
	)
	c.Assert(
		network.PortRange{80, 100, "TCP"}.GoString(),
		gc.Equals,
		"80-100/tcp",
	)
}

func (*PortRangeSuite) TestValidate(c *gc.C) {
	testCases := []struct {
		about    string
		ports    network.PortRange
		expected string
	}{{
		"single valid port",
		network.PortRange{80, 80, "tcp"},
		"",
	}, {
		"valid port range",
		network.PortRange{80, 90, "tcp"},
		"",
	}, {
		"valid udp port range",
		network.PortRange{80, 90, "UDP"},
		"",
	}, {
		"invalid port range boundaries",
		network.PortRange{90, 80, "tcp"},
		"invalid port range 90-80/tcp",
	}, {
		"both FromPort and ToPort too large",
		network.PortRange{88888, 99999, "tcp"},
		"invalid port range 88888-99999/tcp",
	}, {
		"FromPort too large",
		network.PortRange{88888, 65535, "tcp"},
		"invalid port range 88888-65535/tcp",
	}, {
		"FromPort too small",
		network.PortRange{0, 80, "tcp"},
		"invalid port range 0-80/tcp",
	}, {
		"ToPort too large",
		network.PortRange{1, 99999, "tcp"},
		"invalid port range 1-99999/tcp",
	}, {
		"both ports 0",
		network.PortRange{0, 0, "tcp"},
		"invalid port range 0-0/tcp",
	}, {
		"invalid protocol",
		network.PortRange{80, 80, "some protocol"},
		`invalid protocol "some protocol", expected "tcp" or "udp"`,
	}}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		if t.expected == "" {
			c.Check(t.ports.Validate(), gc.IsNil)
		} else {
			c.Check(t.ports.Validate(), gc.ErrorMatches, t.expected)
		}
	}
}

func (*PortRangeSuite) TestSortPortRanges(c *gc.C) {
	ranges := []network.PortRange{
		{10, 100, "udp"},
		{80, 90, "tcp"},
		{80, 80, "tcp"},
	}
	expected := []network.PortRange{
		{80, 80, "tcp"},
		{80, 90, "tcp"},
		{10, 100, "udp"},
	}
	network.SortPortRanges(ranges)
	c.Assert(ranges, gc.DeepEquals, expected)
}

func (*PortRangeSuite) TestCollapsePorts(c *gc.C) {
	testCases := []struct {
		about    string
		ports    []network.Port
		expected []network.PortRange
	}{{
		"single port",
		[]network.Port{{"tcp", 80}},
		[]network.PortRange{{80, 80, "tcp"}},
	}, {
		"continuous port range (increasing)",
		[]network.Port{{"tcp", 80}, {"tcp", 81}, {"tcp", 82}, {"tcp", 83}},
		[]network.PortRange{{80, 83, "tcp"}},
	}, {
		"continuous port range (decreasing)",
		[]network.Port{{"tcp", 83}, {"tcp", 82}, {"tcp", 81}, {"tcp", 80}},
		[]network.PortRange{{80, 83, "tcp"}},
	}, {
		"non-continuous port range (increasing)",
		[]network.Port{{"tcp", 80}, {"tcp", 81}, {"tcp", 82}, {"tcp", 84}, {"tcp", 85}},
		[]network.PortRange{{80, 82, "tcp"}, {84, 85, "tcp"}},
	}, {
		"non-continuous port range (decreasing)",
		[]network.Port{{"tcp", 85}, {"tcp", 84}, {"tcp", 82}, {"tcp", 81}, {"tcp", 80}},
		[]network.PortRange{{80, 82, "tcp"}, {84, 85, "tcp"}},
	}, {
		"alternating tcp / udp ports (increasing)",
		[]network.Port{{"tcp", 80}, {"udp", 81}, {"tcp", 82}, {"udp", 83}, {"tcp", 84}},
		[]network.PortRange{{80, 80, "tcp"}, {82, 82, "tcp"}, {84, 84, "tcp"}, {81, 81, "udp"}, {83, 83, "udp"}},
	}, {
		"alternating tcp / udp ports (decreasing)",
		[]network.Port{{"tcp", 84}, {"udp", 83}, {"tcp", 82}, {"udp", 81}, {"tcp", 80}},
		[]network.PortRange{{80, 80, "tcp"}, {82, 82, "tcp"}, {84, 84, "tcp"}, {81, 81, "udp"}, {83, 83, "udp"}},
	}, {
		"non-continuous port range (udp vs tcp - increasing)",
		[]network.Port{{"tcp", 80}, {"tcp", 81}, {"tcp", 82}, {"udp", 84}, {"tcp", 83}},
		[]network.PortRange{{80, 83, "tcp"}, {84, 84, "udp"}},
	}, {
		"non-continuous port range (udp vs tcp - decreasing)",
		[]network.Port{{"tcp", 83}, {"udp", 84}, {"tcp", 82}, {"tcp", 81}, {"tcp", 80}},
		[]network.PortRange{{80, 83, "tcp"}, {84, 84, "udp"}},
	}}
	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		c.Check(network.CollapsePorts(t.ports), jc.DeepEquals, t.expected)
	}
}

func (*PortRangeSuite) TestParsePortRange(c *gc.C) {
	portRange, err := network.ParsePortRange("8000-8099/tcp")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(portRange.Protocol, gc.Equals, "tcp")
	c.Check(portRange.FromPort, gc.Equals, 8000)
	c.Check(portRange.ToPort, gc.Equals, 8099)
}

func (*PortRangeSuite) TestParsePortRangeSingle(c *gc.C) {
	portRange, err := network.ParsePortRange("80/tcp")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(portRange.Protocol, gc.Equals, "tcp")
	c.Check(portRange.FromPort, gc.Equals, 80)
	c.Check(portRange.ToPort, gc.Equals, 80)
}

func (*PortRangeSuite) TestParsePortRangeDefaultProtocol(c *gc.C) {
	portRange, err := network.ParsePortRange("80")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(portRange.Protocol, gc.Equals, "tcp")
	c.Check(portRange.FromPort, gc.Equals, 80)
	c.Check(portRange.ToPort, gc.Equals, 80)
}

func (*PortRangeSuite) TestParsePortRangeRoundTrip(c *gc.C) {
	portRange, err := network.ParsePortRange("8000-8099/tcp")
	c.Assert(err, jc.ErrorIsNil)
	portRangeStr := portRange.String()

	c.Check(portRangeStr, gc.Equals, "8000-8099/tcp")
}

func (*PortRangeSuite) TestParsePortRangeMultiRange(c *gc.C) {
	_, err := network.ParsePortRange("10-55-100")

	c.Check(err, gc.ErrorMatches, `invalid port range "10-55-100".*`)
}

func (*PortRangeSuite) TestParsePortRangeNonIntPort(c *gc.C) {
	_, err := network.ParsePortRange("spam-100")

	c.Check(err, gc.ErrorMatches, `invalid port "spam".*`)
}

func (*PortRangeSuite) TestMustParsePortRange(c *gc.C) {
	portRange := network.MustParsePortRange("8000-8099/tcp")

	c.Check(portRange.Protocol, gc.Equals, "tcp")
	c.Check(portRange.FromPort, gc.Equals, 8000)
	c.Check(portRange.ToPort, gc.Equals, 8099)
}

func (*PortRangeSuite) TestMustParsePortRangeInvalid(c *gc.C) {
	f := func() {
		network.MustParsePortRange("10-55-100")
	}

	c.Check(f, gc.PanicMatches, `invalid port range "10-55-100".*`)
}

func (*PortRangeSuite) TestParsePortRanges(c *gc.C) {
	portRanges, err := network.ParsePortRanges("80/tcp,8000-8099/tcp")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(portRanges, gc.HasLen, 2)
	c.Check(portRanges[0].Protocol, gc.Equals, "tcp")
	c.Check(portRanges[0].FromPort, gc.Equals, 80)
	c.Check(portRanges[0].ToPort, gc.Equals, 80)
	c.Check(portRanges[1].Protocol, gc.Equals, "tcp")
	c.Check(portRanges[1].FromPort, gc.Equals, 8000)
	c.Check(portRanges[1].ToPort, gc.Equals, 8099)
}

func (*PortRangeSuite) TestParsePortRangesSingle(c *gc.C) {
	portRanges, err := network.ParsePortRanges("80")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(portRanges, gc.HasLen, 1)
	c.Check(portRanges[0].Protocol, gc.Equals, "tcp")
	c.Check(portRanges[0].FromPort, gc.Equals, 80)
	c.Check(portRanges[0].ToPort, gc.Equals, 80)
}

func (*PortRangeSuite) TestParsePortRangesSpaces(c *gc.C) {
	portRanges, err := network.ParsePortRanges(" 80, 	8000-8099  ")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(portRanges, gc.HasLen, 2)
	c.Check(portRanges[0].Protocol, gc.Equals, "tcp")
	c.Check(portRanges[0].FromPort, gc.Equals, 80)
	c.Check(portRanges[0].ToPort, gc.Equals, 80)
	c.Check(portRanges[1].Protocol, gc.Equals, "tcp")
	c.Check(portRanges[1].FromPort, gc.Equals, 8000)
	c.Check(portRanges[1].ToPort, gc.Equals, 8099)
}
