// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"fmt"
	"net"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type HostPortSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&HostPortSuite{})

type hostPortTest struct {
	about         string
	hostPorts     []network.HostPort
	expectedIndex int
	preferIPv6    bool
}

// hostPortTest returns the HostPort equivalent test to the
// receiving selectTest.
func (t selectTest) hostPortTest() hostPortTest {
	hps := network.AddressesWithPort(t.addresses, 9999)
	for i := range hps {
		hps[i].Port = i + 1
	}
	return hostPortTest{
		about:         t.about,
		hostPorts:     hps,
		expectedIndex: t.expectedIndex,
		preferIPv6:    t.preferIPv6,
	}
}

// expected returns the expected host:port result
// of the test.
func (t hostPortTest) expected() string {
	if t.expectedIndex == -1 {
		return ""
	}
	return t.hostPorts[t.expectedIndex].NetAddr()
}

func (*HostPortSuite) TestSelectPublicHostPort(c *gc.C) {
	oldValue := network.GetPreferIPv6()
	defer func() {
		network.SetPreferIPv6(oldValue)
	}()
	for i, t0 := range selectPublicTests {
		t := t0.hostPortTest()
		c.Logf("test %d: %s", i, t.about)
		network.SetPreferIPv6(t.preferIPv6)
		c.Check(network.SelectPublicHostPort(t.hostPorts), jc.DeepEquals, t.expected())
	}
}

func (*HostPortSuite) TestSelectInternalHostPort(c *gc.C) {
	oldValue := network.GetPreferIPv6()
	defer func() {
		network.SetPreferIPv6(oldValue)
	}()
	for i, t0 := range selectInternalTests {
		t := t0.hostPortTest()
		c.Logf("test %d: %s", i, t.about)
		network.SetPreferIPv6(t.preferIPv6)
		c.Check(network.SelectInternalHostPort(t.hostPorts, false), jc.DeepEquals, t.expected())
	}
}

func (*HostPortSuite) TestSelectInternalMachineHostPort(c *gc.C) {
	oldValue := network.GetPreferIPv6()
	defer func() {
		network.SetPreferIPv6(oldValue)
	}()
	for i, t0 := range selectInternalMachineTests {
		t := t0.hostPortTest()
		c.Logf("test %d: %s", i, t.about)
		network.SetPreferIPv6(t.preferIPv6)
		c.Check(network.SelectInternalHostPort(t.hostPorts, true), gc.DeepEquals, t.expected())
	}
}

func (s *HostPortSuite) TestResolveOrDropHostnames(c *gc.C) {
	seq := 0
	s.PatchValue(network.NetLookupIP, func(host string) ([]net.IP, error) {
		if host == "invalid host" {
			return nil, errors.New("lookup invalid host: no such host")
		}
		if host == "localhost" {
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		}
		// Return 2 IPs for .net hosts, 1 IP otherwise.
		var ips []net.IP
		ips = append(ips, net.ParseIP(fmt.Sprintf("0.1.2.%d", seq)))
		seq++
		if strings.Contains(host, ".net") {
			ips = append(ips, net.ParseIP(fmt.Sprintf("0.1.2.%d", seq)))
			seq++
		}
		c.Logf("lookup host %q -> %v", host, ips)
		return ips, nil
	})
	resolved := network.ResolveOrDropHostnames(s.makeHostPorts())
	c.Assert(
		c.GetTestLog(),
		jc.Contains,
		`DEBUG juju.network removing unresolvable address "invalid host"`,
	)
	// Order should be preserved, duplicates dropped and hostnames,
	// except localhost resolved or dropped.
	c.Assert(resolved, jc.DeepEquals, network.NewHostPorts(1234,
		"127.0.0.1",
		"localhost", // localhost is not resolved intentionally.
		"0.1.2.0",   // from example.com
		"127.0.1.1",
		"0.1.2.1", // from example.org
		"2001:db8::2",
		"169.254.1.1",
		"0.1.2.2", // from example.net
		"0.1.2.3", // from example.net
		"fd00::22",
		"2001:db8::1",
		"169.254.1.2",
		"ff01::22",
		"10.0.0.1",
		"::1",
		"fc00::1",
		"fe80::2",
		"172.16.0.1",
		"8.8.8.8",
		"7.8.8.8",
	))
}

func (s *HostPortSuite) TestFilterUnusableHostPorts(c *gc.C) {
	// The order is preserved, but machine- and link-local addresses
	// are dropped.
	expected := network.NewHostPorts(1234,
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
	)
	result := network.FilterUnusableHostPorts(s.makeHostPorts())
	c.Assert(result, gc.HasLen, len(expected))
	c.Assert(result, jc.DeepEquals, expected)
}

func (*HostPortSuite) TestCollapseHostPorts(c *gc.C) {
	servers := [][]network.HostPort{
		network.NewHostPorts(1234,
			"0.1.2.3", "10.0.1.2", "fc00::1", "2001:db8::1", "::1",
			"127.0.0.1", "localhost", "fe80::123", "example.com",
		),
		network.NewHostPorts(4321,
			"8.8.8.8", "1.2.3.4", "fc00::2", "127.0.0.1", "foo",
		),
		network.NewHostPorts(9999,
			"localhost", "127.0.0.1",
		),
	}
	expected := append(servers[0], append(servers[1], servers[2]...)...)
	result := network.CollapseHostPorts(servers)
	c.Assert(result, gc.HasLen, len(servers[0])+len(servers[1])+len(servers[2]))
	c.Assert(result, jc.DeepEquals, expected)
}

func (s *HostPortSuite) TestEnsureFirstHostPort(c *gc.C) {
	first := network.NewHostPorts(1234, "1.2.3.4")[0]

	// Without any HostPorts, it still works.
	hps := network.EnsureFirstHostPort(first, []network.HostPort{})
	c.Assert(hps, jc.DeepEquals, []network.HostPort{first})

	// If already there, no changes happen.
	hps = s.makeHostPorts()
	result := network.EnsureFirstHostPort(hps[0], hps)
	c.Assert(result, jc.DeepEquals, hps)

	// If not at the top, pop it up and put it on top.
	firstLast := append(hps, first)
	result = network.EnsureFirstHostPort(first, firstLast)
	c.Assert(result, jc.DeepEquals, append([]network.HostPort{first}, hps...))
}

func (*HostPortSuite) TestNewHostPorts(c *gc.C) {
	addrs := []string{"0.1.2.3", "fc00::1", "::1", "example.com"}
	expected := network.AddressesWithPort(
		network.NewAddresses(addrs...), 42,
	)
	result := network.NewHostPorts(42, addrs...)
	c.Assert(result, gc.HasLen, len(addrs))
	c.Assert(result, jc.DeepEquals, expected)
}

func (*HostPortSuite) TestParseHostPortsErrors(c *gc.C) {
	for i, test := range []struct {
		input string
		err   string
	}{{
		input: "",
		err:   `cannot parse "" as address:port: missing port in address`,
	}, {
		input: " ",
		err:   `cannot parse " " as address:port: missing port in address  `,
	}, {
		input: ":",
		err:   `cannot parse ":" port: strconv.ParseInt: parsing "": invalid syntax`,
	}, {
		input: "host",
		err:   `cannot parse "host" as address:port: missing port in address host`,
	}, {
		input: "host:port",
		err:   `cannot parse "host:port" port: strconv.ParseInt: parsing "port": invalid syntax`,
	}, {
		input: "::1",
		err:   `cannot parse "::1" as address:port: too many colons in address ::1`,
	}, {
		input: "1.2.3.4",
		err:   `cannot parse "1.2.3.4" as address:port: missing port in address 1.2.3.4`,
	}, {
		input: "1.2.3.4:foo",
		err:   `cannot parse "1.2.3.4:foo" port: strconv.ParseInt: parsing "foo": invalid syntax`,
	}} {
		c.Logf("test %d: input %q", i, test.input)
		// First test all error cases with a single argument.
		hps, err := network.ParseHostPorts(test.input)
		c.Check(err, gc.ErrorMatches, test.err)
		c.Check(hps, gc.IsNil)
	}
	// Finally, test with mixed valid and invalid args.
	hps, err := network.ParseHostPorts("1.2.3.4:42", "[fc00::1]:12", "foo")
	c.Assert(err, gc.ErrorMatches, `cannot parse "foo" as address:port: missing port in address foo`)
	c.Assert(hps, gc.IsNil)
}

func (*HostPortSuite) TestParseHostPortsSuccess(c *gc.C) {
	for i, test := range []struct {
		args   []string
		expect []network.HostPort
	}{{
		args:   nil,
		expect: []network.HostPort{},
	}, {
		args:   []string{"1.2.3.4:42"},
		expect: network.NewHostPorts(42, "1.2.3.4"),
	}, {
		args:   []string{"[fc00::1]:1234"},
		expect: network.NewHostPorts(1234, "fc00::1"),
	}, {
		args: []string{"[fc00::1]:1234", "127.0.0.1:4321", "example.com:42"},
		expect: []network.HostPort{
			{network.NewAddress("fc00::1"), 1234},
			{network.NewAddress("127.0.0.1"), 4321},
			{network.NewAddress("example.com"), 42},
		},
	}} {
		c.Logf("test %d: args %v", i, test.args)
		hps, err := network.ParseHostPorts(test.args...)
		c.Check(err, jc.ErrorIsNil)
		c.Check(hps, jc.DeepEquals, test.expect)
	}
}

func (*HostPortSuite) TestAddressesWithPortAndHostsWithoutPort(c *gc.C) {
	addrs := network.NewAddresses("0.1.2.3", "0.2.4.6")
	hps := network.AddressesWithPort(addrs, 999)
	c.Assert(hps, jc.DeepEquals, []network.HostPort{{
		Address: network.NewAddress("0.1.2.3"),
		Port:    999,
	}, {
		Address: network.NewAddress("0.2.4.6"),
		Port:    999,
	}})
	c.Assert(network.HostsWithoutPort(hps), jc.DeepEquals, addrs)
}

func (s *HostPortSuite) TestSortHostPorts(c *gc.C) {
	hps := s.makeHostPorts()
	// Simulate prefer-ipv6: false first.
	network.SortHostPorts(hps, false)
	c.Assert(hps, jc.DeepEquals, network.NewHostPorts(1234,
		// Public IPv4 addresses on top.
		"0.1.2.0",
		"7.8.8.8",
		"8.8.8.8",
		// After that public IPv6 addresses.
		"2001:db8::1",
		"2001:db8::1",
		"2001:db8::2",
		// Then hostnames.
		"example.com",
		"example.net",
		"example.org",
		"invalid host",
		"localhost",
		"localhost",
		// Then IPv4 cloud-local addresses.
		"10.0.0.1",
		"172.16.0.1",
		// Then IPv6 cloud-local addresses.
		"fc00::1",
		"fd00::22",
		// Then machine-local IPv4 addresses.
		"127.0.0.1",
		"127.0.0.1",
		"127.0.1.1",
		// Then machine-local IPv6 addresses.
		"::1",
		"::1",
		// Then link-local IPv4 addresses.
		"169.254.1.1",
		"169.254.1.2",
		// Finally, link-local IPv6 addresses.
		"fe80::2",
		"ff01::22",
	))

	// Now, simulate prefer-ipv6: true.
	network.SortHostPorts(hps, true)
	c.Assert(hps, jc.DeepEquals, network.NewHostPorts(1234,
		// Public IPv6 addresses on top.
		"2001:db8::1",
		"2001:db8::1",
		"2001:db8::2",
		// After that public IPv4 addresses.
		"0.1.2.0",
		"7.8.8.8",
		"8.8.8.8",
		// Then hostnames.
		"example.com",
		"example.net",
		"example.org",
		"invalid host",
		"localhost",
		"localhost",
		// Then IPv6 cloud-local addresses.
		"fc00::1",
		"fd00::22",
		// Then IPv4 cloud-local addresses.
		"10.0.0.1",
		"172.16.0.1",
		// Then machine-local IPv6 addresses.
		"::1",
		"::1",
		// Then machine-local IPv4 addresses.
		"127.0.0.1",
		"127.0.0.1",
		"127.0.1.1",
		// Then link-local IPv6 addresses.
		"fe80::2",
		"ff01::22",
		// Finally, link-local IPv4 addresses.
		"169.254.1.1",
		"169.254.1.2",
	))
}

var netAddrTests = []struct {
	addr   network.Address
	port   int
	expect string
}{{
	addr:   network.NewAddress("0.1.2.3"),
	port:   99,
	expect: "0.1.2.3:99",
}, {
	addr:   network.NewAddress("2001:DB8::1"),
	port:   100,
	expect: "[2001:DB8::1]:100",
}, {
	addr:   network.NewAddress("172.16.0.1"),
	port:   52,
	expect: "172.16.0.1:52",
}, {
	addr:   network.NewAddress("fc00::2"),
	port:   1111,
	expect: "[fc00::2]:1111",
}, {
	addr:   network.NewAddress("example.com"),
	port:   9999,
	expect: "example.com:9999",
}, {
	addr:   network.NewScopedAddress("example.com", network.ScopePublic),
	port:   1234,
	expect: "example.com:1234",
}, {
	addr:   network.NewAddress("169.254.1.2"),
	port:   123,
	expect: "169.254.1.2:123",
}, {
	addr:   network.NewAddress("fe80::222"),
	port:   321,
	expect: "[fe80::222]:321",
}, {
	addr:   network.NewAddress("127.0.0.2"),
	port:   121,
	expect: "127.0.0.2:121",
}, {
	addr:   network.NewAddress("::1"),
	port:   111,
	expect: "[::1]:111",
}}

func (*HostPortSuite) TestNetAddrAndString(c *gc.C) {
	for i, test := range netAddrTests {
		c.Logf("test %d: %q", i, test.addr)
		hp := network.HostPort{
			Address: test.addr,
			Port:    test.port,
		}
		c.Check(hp.NetAddr(), gc.Equals, test.expect)
		c.Check(hp.String(), gc.Equals, test.expect)
		c.Check(hp.GoString(), gc.Equals, test.expect)
	}
}

func (s *HostPortSuite) TestDropDuplicatedHostPorts(c *gc.C) {
	hps := s.makeHostPorts()
	noDups := network.DropDuplicatedHostPorts(hps)
	c.Assert(noDups, gc.Not(gc.HasLen), len(hps))
	c.Assert(noDups, jc.DeepEquals, network.NewHostPorts(1234,
		"127.0.0.1",
		"localhost",
		"example.com",
		"127.0.1.1",
		"example.org",
		"2001:db8::2",
		"169.254.1.1",
		"example.net",
		"invalid host",
		"fd00::22",
		"2001:db8::1",
		"169.254.1.2",
		"ff01::22",
		"0.1.2.0",
		"10.0.0.1",
		"::1",
		"fc00::1",
		"fe80::2",
		"172.16.0.1",
		"8.8.8.8",
		"7.8.8.8",
	))
}

func (s *HostPortSuite) TestHostPortsToStrings(c *gc.C) {
	hps := s.makeHostPorts()
	strHPs := network.HostPortsToStrings(hps)
	c.Assert(strHPs, gc.HasLen, len(hps))
	c.Assert(strHPs, jc.DeepEquals, []string{
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
	})
}

func (*HostPortSuite) makeHostPorts() []network.HostPort {
	return network.NewHostPorts(1234,
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
	)
}
