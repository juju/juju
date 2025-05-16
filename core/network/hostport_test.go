// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"fmt"
	"sort"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	coretesting "github.com/juju/juju/internal/testing"
)

type HostPortSuite struct {
	coretesting.BaseSuite
}

func TestHostPortSuite(t *stdtesting.T) { tc.Run(t, &HostPortSuite{}) }
func (s *HostPortSuite) TestFilterUnusableHostPorts(c *tc.C) {
	// The order is preserved, but machine- and link-local addresses
	// are dropped.
	expected := append(
		network.NewSpaceHostPorts(1234,
			"localhost",
			"example.com",
			"example.org",
			"2001:db8::2",
			"example.net",
			"invalid host",
			"fd00::22",
			"2001:db8::1",
			"0.1.2.0",
			"2001:db8::1",
			"localhost",
			"10.0.0.1",
			"fc00::1",
			"172.16.0.1",
			"8.8.8.8",
			"7.8.8.8",
		),
		network.NewSpaceHostPorts(9999,
			"10.0.0.1",
			"2001:db8::1", // public
		)...,
	).HostPorts()

	result := s.makeHostPorts().HostPorts().FilterUnusable()
	c.Assert(result, tc.HasLen, len(expected))
	c.Assert(result, tc.DeepEquals, expected)
}

func (*HostPortSuite) TestCollapseToHostPorts(c *tc.C) {
	servers := []network.MachineHostPorts{
		network.NewMachineHostPorts(1234,
			"0.1.2.3", "10.0.1.2", "fc00::1", "2001:db8::1", "::1",
			"127.0.0.1", "localhost", "fe80::123", "example.com",
		),
		network.NewMachineHostPorts(4321,
			"8.8.8.8", "1.2.3.4", "fc00::2", "127.0.0.1", "foo",
		),
		network.NewMachineHostPorts(9999,
			"localhost", "127.0.0.1",
		),
	}
	expected := append(servers[0], append(servers[1], servers[2]...)...).HostPorts()
	result := network.CollapseToHostPorts(servers)
	c.Assert(result, tc.HasLen, len(servers[0])+len(servers[1])+len(servers[2]))
	c.Assert(result, tc.DeepEquals, expected)
}

func (s *HostPortSuite) TestEnsureFirstHostPort(c *tc.C) {
	first := network.NewSpaceHostPorts(1234, "1.2.3.4")[0]

	// Without any HostPorts, it still works.
	hps := network.EnsureFirstHostPort(first, []network.SpaceHostPort{})
	c.Assert(hps, tc.DeepEquals, network.SpaceHostPorts{first})

	// If already there, no changes happen.
	hps = s.makeHostPorts()
	result := network.EnsureFirstHostPort(hps[0], hps)
	c.Assert(result, tc.DeepEquals, hps)

	// If not at the top, pop it up and put it on top.
	firstLast := append(hps, first)
	result = network.EnsureFirstHostPort(first, firstLast)
	c.Assert(result, tc.DeepEquals, append(network.SpaceHostPorts{first}, hps...))
}

func (*HostPortSuite) TestNewHostPorts(c *tc.C) {
	addrs := []string{"0.1.2.3", "fc00::1", "::1", "example.com"}
	expected := network.SpaceAddressesWithPort(
		network.NewSpaceAddresses(addrs...), 42,
	)
	result := network.NewSpaceHostPorts(42, addrs...)
	c.Assert(result, tc.HasLen, len(addrs))
	c.Assert(result, tc.DeepEquals, expected)
}

func (*HostPortSuite) TestParseHostPortsErrors(c *tc.C) {
	for i, test := range []struct {
		input string
		err   string
	}{{
		input: "",
		err:   `cannot parse "" as address:port: .*missing port in address.*`,
	}, {
		input: " ",
		err:   `cannot parse " " as address:port: .*missing port in address.*`,
	}, {
		input: ":",
		err:   `cannot parse ":" port: strconv.(ParseInt|Atoi): parsing "": invalid syntax`,
	}, {
		input: "host",
		err:   `cannot parse "host" as address:port: .*missing port in address.*`,
	}, {
		input: "host:port",
		err:   `cannot parse "host:port" port: strconv.(ParseInt|Atoi): parsing "port": invalid syntax`,
	}, {
		input: "::1",
		err:   `cannot parse "::1" as address:port: .*too many colons in address.*`,
	}, {
		input: "1.2.3.4",
		err:   `cannot parse "1.2.3.4" as address:port: .*missing port in address.*`,
	}, {
		input: "1.2.3.4:foo",
		err:   `cannot parse "1.2.3.4:foo" port: strconv.(ParseInt|Atoi): parsing "foo": invalid syntax`,
	}} {
		c.Logf("test %d: input %q", i, test.input)
		// First test all error cases with a single argument.
		hps, err := network.ParseMachineHostPort(test.input)
		c.Check(err, tc.ErrorMatches, test.err)
		c.Check(hps, tc.IsNil)
	}
	// Finally, test with mixed valid and invalid args.
	hps, err := network.ParseProviderHostPorts("1.2.3.4:42", "[fc00::1]:12", "foo")
	c.Assert(err, tc.ErrorMatches, `cannot parse "foo" as address:port: .*missing port in address.*`)
	c.Assert(hps, tc.IsNil)
}

func (*HostPortSuite) TestParseProviderHostPortsSuccess(c *tc.C) {
	for i, test := range []struct {
		args   []string
		expect network.ProviderHostPorts
	}{{
		args:   nil,
		expect: []network.ProviderHostPort{},
	}, {
		args:   []string{"1.2.3.4:42"},
		expect: []network.ProviderHostPort{{network.NewMachineAddress("1.2.3.4").AsProviderAddress(), 42}},
	}, {
		args:   []string{"[fc00::1]:1234"},
		expect: []network.ProviderHostPort{{network.NewMachineAddress("fc00::1").AsProviderAddress(), 1234}},
	}, {
		args: []string{"[fc00::1]:1234", "127.0.0.1:4321", "example.com:42"},
		expect: []network.ProviderHostPort{
			{network.NewMachineAddress("fc00::1").AsProviderAddress(), 1234},
			{network.NewMachineAddress("127.0.0.1").AsProviderAddress(), 4321},
			{network.NewMachineAddress("example.com").AsProviderAddress(), 42},
		},
	}} {
		c.Logf("test %d: args %v", i, test.args)
		hps, err := network.ParseProviderHostPorts(test.args...)
		c.Check(err, tc.ErrorIsNil)
		c.Check(hps, tc.DeepEquals, test.expect)
	}
}

func (*HostPortSuite) TestAddressesWithPort(c *tc.C) {
	addrs := network.NewSpaceAddresses("0.1.2.3", "0.2.4.6")
	hps := network.SpaceAddressesWithPort(addrs, 999)
	c.Assert(hps, tc.DeepEquals, network.SpaceHostPorts{{
		SpaceAddress: network.NewSpaceAddress("0.1.2.3"),
		NetPort:      999,
	}, {
		SpaceAddress: network.NewSpaceAddress("0.2.4.6"),
		NetPort:      999,
	}})
}

func (s *HostPortSuite) assertHostPorts(c *tc.C, actual network.HostPorts, expected ...string) {
	c.Assert(actual.Strings(), tc.DeepEquals, expected)
}

func (s *HostPortSuite) TestSortHostPorts(c *tc.C) {
	hps := s.makeHostPorts()
	sort.Sort(hps)
	s.assertHostPorts(c, hps.HostPorts(),
		// Public IPv4 addresses on top.
		"0.1.2.0:1234",
		"7.8.8.8:1234",
		"8.8.8.8:1234",
		// After that public IPv6 addresses.
		"[2001:db8::1]:1234",
		"[2001:db8::1]:1234",
		"[2001:db8::1]:9999",
		"[2001:db8::2]:1234",
		// Then hostnames.
		"example.com:1234",
		"example.net:1234",
		"example.org:1234",
		"invalid host:1234",
		"localhost:1234",
		"localhost:1234",
		// Then IPv4 cloud-local addresses.
		"10.0.0.1:1234",
		"10.0.0.1:9999",
		"172.16.0.1:1234",
		// Then IPv6 cloud-local addresses.
		"[fc00::1]:1234",
		"[fd00::22]:1234",
		// Then machine-local IPv4 addresses.
		"127.0.0.1:1234",
		"127.0.0.1:1234",
		"127.0.0.1:9999",
		"127.0.1.1:1234",
		// Then machine-local IPv6 addresses.
		"[::1]:1234",
		"[::1]:1234",
		// Then link-local IPv4 addresses.
		"169.254.1.1:1234",
		"169.254.1.2:1234",
		// Finally, link-local IPv6 addresses.
		"[fe80::2]:1234",
		"[fe80::2]:9999",
		"[ff01::22]:1234",
	)
}

var netAddrTests = []struct {
	addr   network.SpaceAddress
	port   int
	expect string
}{{
	addr:   network.NewSpaceAddress("0.1.2.3"),
	port:   99,
	expect: "0.1.2.3:99",
}, {
	addr:   network.NewSpaceAddress("2001:DB8::1"),
	port:   100,
	expect: "[2001:DB8::1]:100",
}, {
	addr:   network.NewSpaceAddress("172.16.0.1"),
	port:   52,
	expect: "172.16.0.1:52",
}, {
	addr:   network.NewSpaceAddress("fc00::2"),
	port:   1111,
	expect: "[fc00::2]:1111",
}, {
	addr:   network.NewSpaceAddress("example.com"),
	port:   9999,
	expect: "example.com:9999",
}, {
	addr:   network.NewSpaceAddress("example.com", network.WithScope(network.ScopePublic)),
	port:   1234,
	expect: "example.com:1234",
}, {
	addr:   network.NewSpaceAddress("169.254.1.2"),
	port:   123,
	expect: "169.254.1.2:123",
}, {
	addr:   network.NewSpaceAddress("fe80::222"),
	port:   321,
	expect: "[fe80::222]:321",
}, {
	addr:   network.NewSpaceAddress("127.0.0.2"),
	port:   121,
	expect: "127.0.0.2:121",
}, {
	addr:   network.NewSpaceAddress("::1"),
	port:   111,
	expect: "[::1]:111",
}}

func (*HostPortSuite) TestDialAddressAndString(c *tc.C) {
	for i, test := range netAddrTests {
		c.Logf("test %d: %q", i, test.addr)
		hp := network.SpaceHostPort{
			SpaceAddress: test.addr,
			NetPort:      network.NetPort(test.port),
		}
		c.Check(network.DialAddress(hp), tc.Equals, test.expect)
		c.Check(hp.String(), tc.Equals, test.expect)
		c.Check(hp.GoString(), tc.Equals, test.expect)
	}
}

func (s *HostPortSuite) TestHostPortsToStrings(c *tc.C) {
	hps := s.makeHostPorts()
	strHPs := hps.HostPorts().Strings()
	c.Assert(strHPs, tc.HasLen, len(hps))
	c.Assert(strHPs, tc.DeepEquals, []string{
		"127.0.0.1:1234",
		"localhost:1234",
		"example.com:1234",
		"127.0.1.1:1234",
		"example.org:1234",
		"[2001:db8::2]:1234",
		"169.254.1.1:1234",
		"example.net:1234",
		"invalid host:1234",
		"[fd00::22]:1234",
		"127.0.0.1:1234",
		"[2001:db8::1]:1234",
		"169.254.1.2:1234",
		"[ff01::22]:1234",
		"0.1.2.0:1234",
		"[2001:db8::1]:1234",
		"localhost:1234",
		"10.0.0.1:1234",
		"[::1]:1234",
		"[fc00::1]:1234",
		"[fe80::2]:1234",
		"172.16.0.1:1234",
		"[::1]:1234",
		"8.8.8.8:1234",
		"7.8.8.8:1234",
		"127.0.0.1:9999",
		"10.0.0.1:9999",
		"[2001:db8::1]:9999",
		"[fe80::2]:9999",
	})
}

func (*HostPortSuite) makeHostPorts() network.SpaceHostPorts {
	return append(
		network.NewSpaceHostPorts(1234,
			"127.0.0.1",    // machine-local
			"localhost",    // hostname
			"example.com",  // hostname
			"127.0.1.1",    // machine-local
			"example.org",  // hostname
			"2001:db8::2",  // public
			"169.254.1.1",  // link-local
			"example.net",  // hostname
			"invalid host", // hostname
			"fd00::22",     // cloud-local
			"127.0.0.1",    // machine-local
			"2001:db8::1",  // public
			"169.254.1.2",  // link-local
			"ff01::22",     // link-local
			"0.1.2.0",      // public
			"2001:db8::1",  // public
			"localhost",    // hostname
			"10.0.0.1",     // cloud-local
			"::1",          // machine-local
			"fc00::1",      // cloud-local
			"fe80::2",      // link-local
			"172.16.0.1",   // cloud-local
			"::1",          // machine-local
			"8.8.8.8",      // public
			"7.8.8.8",      // public
		),
		network.NewSpaceHostPorts(9999,
			"127.0.0.1",   // machine-local
			"10.0.0.1",    // cloud-local
			"2001:db8::1", // public
			"fe80::2",     // link-local
		)...,
	)
}

func (s *HostPortSuite) TestUniqueHostPortsSimpleInput(c *tc.C) {
	input := network.NewSpaceHostPorts(1234, "127.0.0.1", "::1")
	expected := input.HostPorts()
	c.Assert(input.HostPorts().Unique(), tc.DeepEquals, expected)
}

func (s *HostPortSuite) TestUniqueHostPortsOnlyDuplicates(c *tc.C) {
	input := s.manyMachineHostPorts(c, 10000, nil) // use IANA reserved port
	expected := input[0:1].HostPorts()
	c.Assert(input.HostPorts().Unique(), tc.DeepEquals, expected)
}

func (s *HostPortSuite) TestUniqueHostPortsHugeUniqueInput(c *tc.C) {
	input := s.manyMachineHostPorts(c, maxTCPPort, func(port int) string {
		return fmt.Sprintf("127.1.0.1:%d", port)
	})
	expected := input.HostPorts()
	c.Assert(input.HostPorts().Unique(), tc.DeepEquals, expected)
}

const maxTCPPort = 65535

func (s *HostPortSuite) manyMachineHostPorts(
	c *tc.C, count int, addressFunc func(index int) string) network.MachineHostPorts {
	if addressFunc == nil {
		addressFunc = func(_ int) string {
			return "127.0.0.1:49151" // all use the same IANA reserved port.
		}
	}

	results := make([]network.MachineHostPort, count)
	for i := range results {
		hostPort, err := network.ParseMachineHostPort(addressFunc(i))
		c.Assert(err, tc.ErrorIsNil)
		results[i] = *hostPort
	}
	return results
}

type selectInternalHostPortsTest struct {
	about     string
	addresses network.SpaceHostPorts
	expected  []string
}

var prioritizeInternalHostPortsTests = []selectInternalHostPortsTest{{
	"no addresses gives empty string result",
	[]network.SpaceHostPort{},
	[]string{},
}, {
	"a public IPv4 address is selected",
	[]network.SpaceHostPort{
		{network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)), 9999},
	},
	[]string{"8.8.8.8:9999"},
}, {
	"cloud local IPv4 addresses are selected",
	[]network.SpaceHostPort{
		{network.NewSpaceAddress("10.1.0.1", network.WithScope(network.ScopeCloudLocal)), 8888},
		{network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)), 123},
		{network.NewSpaceAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)), 1234},
	},
	[]string{"10.1.0.1:8888", "10.0.0.1:1234", "8.8.8.8:123"},
}, {
	"a machine local or link-local address is not selected",
	[]network.SpaceHostPort{
		{network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)), 111},
		{network.NewSpaceAddress("::1", network.WithScope(network.ScopeMachineLocal)), 222},
		{network.NewSpaceAddress("fe80::1", network.WithScope(network.ScopeLinkLocal)), 333},
	},
	[]string{},
}, {
	"cloud local addresses are preferred to a public addresses",
	[]network.SpaceHostPort{
		{network.NewSpaceAddress("2001:db8::1", network.WithScope(network.ScopePublic)), 123},
		{network.NewSpaceAddress("fc00::1", network.WithScope(network.ScopeCloudLocal)), 123},
		{network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)), 123},
		{network.NewSpaceAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)), 4444},
	},
	[]string{"10.0.0.1:4444", "[fc00::1]:123", "8.8.8.8:123", "[2001:db8::1]:123"},
}}

func (s *HostPortSuite) TestPrioritizeInternalHostPorts(c *tc.C) {
	for i, t := range prioritizeInternalHostPortsTests {
		c.Logf("test %d: %s", i, t.about)
		prioritized := t.addresses.HostPorts().PrioritizedForScope(network.ScopeMatchCloudLocal)
		c.Check(prioritized, tc.DeepEquals, t.expected)
	}
}

var selectInternalHostPortsTests = []selectInternalHostPortsTest{{
	"no addresses gives empty string result",
	[]network.SpaceHostPort{},
	[]string{},
}, {
	"a public IPv4 address is selected",
	[]network.SpaceHostPort{
		{network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)), 9999},
	},
	[]string{"8.8.8.8:9999"},
}, {
	"cloud local IPv4 addresses are selected",
	[]network.SpaceHostPort{
		{network.NewSpaceAddress("10.1.0.1", network.WithScope(network.ScopeCloudLocal)), 8888},
		{network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)), 123},
		{network.NewSpaceAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)), 1234},
	},
	[]string{"10.1.0.1:8888", "10.0.0.1:1234"},
}, {
	"a machine local or link-local address is not selected",
	[]network.SpaceHostPort{
		{network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)), 111},
		{network.NewSpaceAddress("::1", network.WithScope(network.ScopeMachineLocal)), 222},
		{network.NewSpaceAddress("fe80::1", network.WithScope(network.ScopeLinkLocal)), 333},
	},
	[]string{},
}, {
	"cloud local IPv4 addresses are preferred to a public addresses",
	[]network.SpaceHostPort{
		{network.NewSpaceAddress("2001:db8::1", network.WithScope(network.ScopePublic)), 123},
		{network.NewSpaceAddress("fc00::1", network.WithScope(network.ScopeCloudLocal)), 123},
		{network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)), 123},
		{network.NewSpaceAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)), 4444},
	},
	[]string{"10.0.0.1:4444"},
}, {
	"cloud local IPv6 addresses are preferred to a public addresses",
	[]network.SpaceHostPort{
		{network.NewSpaceAddress("2001:db8::1", network.WithScope(network.ScopePublic)), 123},
		{network.NewSpaceAddress("fc00::1", network.WithScope(network.ScopeCloudLocal)), 123},
		{network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic)), 123},
	},
	[]string{"[fc00::1]:123"},
}}

func (s *HostPortSuite) TestSelectInternalHostPorts(c *tc.C) {
	for i, t := range selectInternalHostPortsTests {
		c.Logf("test %d: %s", i, t.about)
		c.Check(t.addresses.AllMatchingScope(network.ScopeMatchCloudLocal), tc.DeepEquals, t.expected)
	}
}
