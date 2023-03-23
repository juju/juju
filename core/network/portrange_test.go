// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
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
	c.Assert(
		network.PortRange{-1, -1, "ICMP"}.String(),
		gc.Equals,
		"icmp",
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
		"port range bounds must be between 0 and 65535, got 88888-99999",
	}, {
		"FromPort too large",
		network.PortRange{88888, 65535, "tcp"},
		"invalid port range 88888-65535/tcp",
	}, {
		"FromPort too small",
		network.PortRange{-1, 80, "tcp"},
		"port range bounds must be between 0 and 65535, got -1-80",
	}, {
		"ToPort too large",
		network.PortRange{1, 99999, "tcp"},
		"port range bounds must be between 0 and 65535, got 1-99999",
	}, {
		"invalid protocol",
		network.PortRange{80, 80, "some protocol"},
		`invalid protocol "some protocol", expected "tcp", "udp", or "icmp"`,
	}, {
		"invalid icmp port",
		network.PortRange{1, 1, "icmp"},
		`protocol "icmp" doesn't support any ports; got "1"`,
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

func (*PortRangeSuite) TestParseIcmpProtocol(c *gc.C) {
	portRange, err := network.ParsePortRange("icmp")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(portRange.Protocol, gc.Equals, "icmp")
	c.Check(portRange.FromPort, gc.Equals, -1)
	c.Check(portRange.ToPort, gc.Equals, -1)
}

func (*PortRangeSuite) TestParseIcmpProtocolRoundTrip(c *gc.C) {
	portRange, err := network.ParsePortRange("icmp")
	c.Assert(err, jc.ErrorIsNil)
	portRangeStr := portRange.String()

	c.Check(portRangeStr, gc.Equals, "icmp")
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

func (*PortRangeSuite) TestCombinePortRanges(c *gc.C) {
	testCases := []struct {
		in       []network.PortRange
		expected []network.PortRange
	}{{
		[]network.PortRange{{80, 80, "tcp"}},
		[]network.PortRange{{80, 80, "tcp"}},
	}, {
		[]network.PortRange{{80, 82, "tcp"}, {83, 85, "tcp"}},
		[]network.PortRange{{80, 85, "tcp"}},
	}, {
		[]network.PortRange{{83, 85, "tcp"}, {80, 82, "tcp"}},
		[]network.PortRange{{80, 85, "tcp"}},
	}, {
		[]network.PortRange{{80, 83, "tcp"}, {85, 87, "tcp"}},
		[]network.PortRange{{80, 83, "tcp"}, {85, 87, "tcp"}},
	}, {
		[]network.PortRange{{85, 87, "tcp"}, {80, 83, "tcp"}},
		[]network.PortRange{{80, 83, "tcp"}, {85, 87, "tcp"}},
	}, {
		[]network.PortRange{{85, 87, "tcp"}, {80, 83, "tcp"}},
		[]network.PortRange{{80, 83, "tcp"}, {85, 87, "tcp"}},
	}, {
		[]network.PortRange{{80, 83, "tcp"}, {84, 87, "udp"}},
		[]network.PortRange{{80, 83, "tcp"}, {84, 87, "udp"}},
	}, {
		[]network.PortRange{{80, 82, "tcp"}, {80, 80, "udp"}, {83, 83, "tcp"}, {81, 84, "udp"}, {84, 85, "tcp"}},
		[]network.PortRange{{80, 85, "tcp"}, {80, 84, "udp"}},
	}, {
		[]network.PortRange{{80, 82, "tcp"}, {81, 84, "udp"}, {84, 84, "tcp"}, {86, 87, "udp"}, {80, 80, "udp"}},
		[]network.PortRange{{80, 82, "tcp"}, {84, 84, "tcp"}, {80, 84, "udp"}, {86, 87, "udp"}},
	}}
	for i, t := range testCases {
		c.Logf("test %d", i)
		c.Check(network.CombinePortRanges(t.in...), jc.DeepEquals, t.expected)
	}
}

func (p *PortRangeSuite) TestPortRangeLength(c *gc.C) {
	testCases := []struct {
		about        string
		ports        network.PortRange
		expectLength int
	}{{
		"single valid port",
		network.MustParsePortRange("80/tcp"),
		1,
	}, {
		"tcp port range",
		network.MustParsePortRange("80-90/tcp"),
		11,
	}, {
		"udp port range",
		network.MustParsePortRange("80-90/udp"),
		11,
	}, {
		"ICMP range",
		network.PortRange{Protocol: "icmp", FromPort: -1, ToPort: -1},
		1,
	}, {
		"longest valid range",
		network.MustParsePortRange("1-65535/tcp"),
		65535,
	}}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		c.Check(t.ports.Length(), gc.Equals, t.expectLength)
	}
}

func (p *PortRangeSuite) TestSanitizeBounds(c *gc.C) {
	tests := []struct {
		about  string
		input  network.PortRange
		output network.PortRange
	}{{
		"valid range",
		network.PortRange{FromPort: 100, ToPort: 200},
		network.PortRange{FromPort: 100, ToPort: 200},
	}, {
		"negative lower bound",
		network.PortRange{FromPort: -10, ToPort: 10},
		network.PortRange{FromPort: 1, ToPort: 10},
	}, {
		"zero lower bound",
		network.PortRange{FromPort: 0, ToPort: 10},
		network.PortRange{FromPort: 1, ToPort: 10},
	}, {
		"negative upper bound",
		network.PortRange{FromPort: 42, ToPort: -20},
		network.PortRange{FromPort: 1, ToPort: 42},
	}, {
		"zero upper bound",
		network.PortRange{FromPort: 42, ToPort: 0},
		network.PortRange{FromPort: 1, ToPort: 42},
	}, {
		"both bounds negative",
		network.PortRange{FromPort: -10, ToPort: -20},
		network.PortRange{FromPort: 1, ToPort: 1},
	}, {
		"both bounds zero",
		network.PortRange{FromPort: 0, ToPort: 0},
		network.PortRange{FromPort: 1, ToPort: 1},
	}, {
		"swapped bounds",
		network.PortRange{FromPort: 20, ToPort: 10},
		network.PortRange{FromPort: 10, ToPort: 20},
	}, {
		"too large upper bound",
		network.PortRange{FromPort: 20, ToPort: 99999},
		network.PortRange{FromPort: 20, ToPort: 65535},
	}, {
		"too large lower bound",
		network.PortRange{FromPort: 99999, ToPort: 10},
		network.PortRange{FromPort: 10, ToPort: 65535},
	}, {
		"both bounds too large",
		network.PortRange{FromPort: 88888, ToPort: 99999},
		network.PortRange{FromPort: 65535, ToPort: 65535},
	}, {
		"lower negative, upper too large",
		network.PortRange{FromPort: -10, ToPort: 99999},
		network.PortRange{FromPort: 1, ToPort: 65535},
	}, {
		"lower zero, upper too large",
		network.PortRange{FromPort: 0, ToPort: 99999},
		network.PortRange{FromPort: 1, ToPort: 65535},
	}}
	for i, t := range tests {
		c.Logf("test %d: %s", i, t.about)
		c.Check(t.input.SanitizeBounds(), jc.DeepEquals, t.output)
	}
}

func (p *PortRangeSuite) TestUniquePortRanges(c *gc.C) {
	in := []network.PortRange{
		network.MustParsePortRange("123/tcp"),
		network.MustParsePortRange("123/tcp"),
		network.MustParsePortRange("123/tcp"),
		network.MustParsePortRange("456/tcp"),
	}

	exp := []network.PortRange{
		network.MustParsePortRange("123/tcp"),
		network.MustParsePortRange("456/tcp"),
	}

	got := network.UniquePortRanges(in)
	c.Assert(got, gc.DeepEquals, exp, gc.Commentf("expected duplicate port ranges to be removed"))
}

func (p *PortRangeSuite) TestUniquePortRangesInGroup(c *gc.C) {
	in := network.GroupedPortRanges{
		"foxtrot": []network.PortRange{
			network.MustParsePortRange("123/tcp"),
			network.MustParsePortRange("123/tcp"),
		},
		"unicorn": []network.PortRange{
			network.MustParsePortRange("123/tcp"),
			network.MustParsePortRange("456/tcp"),
		},
	}

	exp := []network.PortRange{
		network.MustParsePortRange("123/tcp"),
		network.MustParsePortRange("456/tcp"),
	}

	got := in.UniquePortRanges()
	c.Assert(got, gc.DeepEquals, exp, gc.Commentf("expected duplicate port ranges to be removed"))
}

func (p *PortRangeSuite) TestGroupedPortRangesEquality(c *gc.C) {
	specs := []struct {
		descr    string
		a, b     network.GroupedPortRanges
		expEqual bool
	}{
		{
			descr: "equal port ranges in random order",
			a: network.GroupedPortRanges{
				"foo": []network.PortRange{
					network.MustParsePortRange("123/tcp"),
					network.MustParsePortRange("456/tcp"),
				},
				"bar": []network.PortRange{
					network.MustParsePortRange("123/tcp"),
				},
			},
			b: network.GroupedPortRanges{
				"foo": []network.PortRange{
					network.MustParsePortRange("456/tcp"),
					network.MustParsePortRange("123/tcp"),
				},
				"bar": []network.PortRange{
					network.MustParsePortRange("123/tcp"),
				},
			},
			expEqual: true,
		},
		{
			descr: "groups with different lengths",
			a: network.GroupedPortRanges{
				"foo": []network.PortRange{
					network.MustParsePortRange("123/tcp"),
					network.MustParsePortRange("456/tcp"),
				},
			},
			b: network.GroupedPortRanges{
				"foo": []network.PortRange{
					network.MustParsePortRange("123/tcp"),
				},
			},
			expEqual: false,
		},
		{
			descr: "groups with same length but different keys",
			a: network.GroupedPortRanges{
				"foo": []network.PortRange{
					network.MustParsePortRange("123/tcp"),
					network.MustParsePortRange("456/tcp"),
				},
			},
			b: network.GroupedPortRanges{
				"bar": []network.PortRange{
					network.MustParsePortRange("123/tcp"),
				},
			},
			expEqual: false,
		},
	}

	for i, spec := range specs {
		c.Logf("test %d: %s", i, spec.descr)
		got := spec.a.EqualTo(spec.b)
		c.Assert(got, gc.Equals, spec.expEqual)
	}
}
