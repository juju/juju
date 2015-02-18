// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params_test

import (
	"encoding/json"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
)

type (
	P struct {
		prot string
		num  int
	}
	IS []interface{}
	M  map[string]interface{}
)

type NetworkSuite struct{}

var _ = gc.Suite(&NetworkSuite{})

func (s *NetworkSuite) TestPortsResults(c *gc.C) {
	// Convenience helpers.
	mkPortsResults := func(prs ...params.PortsResult) params.PortsResults {
		return params.PortsResults{
			Results: prs,
		}
	}
	mkPortsResult := func(msg, code string, ports ...P) params.PortsResult {
		pr := params.PortsResult{}
		if msg != "" {
			pr.Error = &params.Error{msg, code}
		}
		for _, p := range ports {
			pr.Ports = append(pr.Ports, params.Port{p.prot, p.num})
		}
		return pr
	}
	mkResults := func(rs ...interface{}) M {
		return M{"Results": rs}
	}
	mkResult := func(err, ports interface{}) M {
		return M{"Error": err, "Ports": ports}
	}
	mkError := func(msg, code string) M {
		return M{"Message": msg, "Code": code}
	}
	mkPort := func(prot string, num int) M {
		return M{"Protocol": prot, "Number": num}
	}
	// Tests.
	tests := []struct {
		about    string
		results  params.PortsResults
		expected M
	}{{
		about:    "empty result set",
		results:  mkPortsResults(),
		expected: mkResults(),
	}, {
		about: "one error",
		results: mkPortsResults(
			mkPortsResult("I failed", "ERR42")),
		expected: mkResults(
			mkResult(mkError("I failed", "ERR42"), nil)),
	}, {
		about: "one succes with one port",
		results: mkPortsResults(
			mkPortsResult("", "", P{"tcp", 80})),
		expected: mkResults(
			mkResult(nil, IS{mkPort("tcp", 80)})),
	}, {
		about: "two results, one error and one success with two ports",
		results: mkPortsResults(
			mkPortsResult("I failed", "ERR42"),
			mkPortsResult("", "", P{"tcp", 80}, P{"tcp", 443})),
		expected: mkResults(
			mkResult(mkError("I failed", "ERR42"), nil),
			mkResult(nil, IS{mkPort("tcp", 80), mkPort("tcp", 443)})),
	}}
	for i, test := range tests {
		c.Logf("\ntest %d: %s", i, test.about)
		output, err := json.Marshal(test.results)
		if !c.Check(err, jc.ErrorIsNil) {
			continue
		}
		c.Logf("\nJSON output:\n%v", string(output))
		c.Check(string(output), jc.JSONEquals, test.expected)
	}
}

func (s *NetworkSuite) TestHostPort(c *gc.C) {
	mkHostPort := func(v, t, n, s string, p int) M {
		return M{
			"Value":       v,
			"Type":        t,
			"NetworkName": n,
			"Scope":       s,
			"Port":        p,
		}
	}
	tests := []struct {
		about    string
		hostPort params.HostPort
		expected M
	}{{
		about: "address only value; port is 1234",
		hostPort: params.HostPort{
			Address: params.Address{
				Value: "foo",
			},
			Port: 1234,
		},
		expected: mkHostPort("foo", "", "", "", 1234),
	}, {
		about: "address value and type; port is 1234",
		hostPort: params.HostPort{
			Address: params.Address{
				Value: "foo",
				Type:  "ipv4",
			},
			Port: 1234,
		},
		expected: mkHostPort("foo", "ipv4", "", "", 1234),
	}, {
		about: "address value, type, and network name, port is 1234",
		hostPort: params.HostPort{
			Address: params.Address{
				Value:       "foo",
				Type:        "ipv4",
				NetworkName: "bar",
			},
			Port: 1234,
		},
		expected: mkHostPort("foo", "ipv4", "bar", "", 1234),
	}, {
		about: "address all fields, port is 1234",
		hostPort: params.HostPort{
			Address: params.Address{
				Value:       "foo",
				Type:        "ipv4",
				NetworkName: "bar",
				Scope:       "public",
			},
			Port: 1234,
		},
		expected: mkHostPort("foo", "ipv4", "bar", "public", 1234),
	}, {
		about: "address all fields, port is 0",
		hostPort: params.HostPort{
			Address: params.Address{
				Value:       "foo",
				Type:        "ipv4",
				NetworkName: "bar",
				Scope:       "public",
			},
			Port: 0,
		},
		expected: mkHostPort("foo", "ipv4", "bar", "public", 0),
	}}
	for i, test := range tests {
		c.Logf("\ntest %d: %s", i, test.about)
		output, err := json.Marshal(test.hostPort)
		if !c.Check(err, jc.ErrorIsNil) {
			continue
		}
		c.Logf("\nJSON output:\n%v", string(output))
		c.Check(string(output), jc.JSONEquals, test.expected)
	}
}

func (s *NetworkSuite) TestMachinePortRange(c *gc.C) {
	mkPortRange := func(u, r string, f, t int, p string) M {
		return M{
			"UnitTag":     u,
			"RelationTag": r,
			"PortRange": M{
				"FromPort": f,
				"ToPort":   t,
				"Protocol": p,
			},
		}
	}
	tests := []struct {
		about            string
		machinePortRange params.MachinePortRange
		expected         M
	}{{
		about: "all values",
		machinePortRange: params.MachinePortRange{
			UnitTag:     "foo/0",
			RelationTag: "foo.db#bar.server",
			PortRange: params.PortRange{
				FromPort: 100,
				ToPort:   200,
				Protocol: "tcp",
			},
		},
		expected: mkPortRange("foo/0", "foo.db#bar.server", 100, 200, "tcp"),
	}, {
		about: "only port range, missing from",
		machinePortRange: params.MachinePortRange{
			PortRange: params.PortRange{
				ToPort:   200,
				Protocol: "tcp",
			},
		},
		expected: mkPortRange("", "", 0, 200, "tcp"),
	}, {
		about: "only port range, missing to",
		machinePortRange: params.MachinePortRange{
			PortRange: params.PortRange{
				FromPort: 100,
				Protocol: "tcp",
			},
		},
		expected: mkPortRange("", "", 100, 0, "tcp"),
	}, {
		about: "only port range, missing protocol",
		machinePortRange: params.MachinePortRange{
			PortRange: params.PortRange{
				FromPort: 100,
				ToPort:   200,
			},
		},
		expected: mkPortRange("", "", 100, 200, ""),
	}, {
		about:            "no field values",
		machinePortRange: params.MachinePortRange{},
		expected:         mkPortRange("", "", 0, 0, ""),
	}}
	for i, test := range tests {
		c.Logf("\ntest %d: %s", i, test.about)
		output, err := json.Marshal(test.machinePortRange)
		if !c.Check(err, jc.ErrorIsNil) {
			continue
		}
		c.Logf("\nJSON output:\n%v", string(output))
		c.Check(string(output), jc.JSONEquals, test.expected)
	}
}

func (s *NetworkSuite) TestPortConvenience(c *gc.C) {
	networkPort := network.Port{
		Protocol: "udp",
		Number:   55555,
	}
	paramsPort := params.FromNetworkPort(networkPort)
	c.Assert(networkPort, jc.DeepEquals, paramsPort.NetworkPort())
}

func (s *NetworkSuite) TestPortRangeConvenience(c *gc.C) {
	networkPortRange := network.PortRange{
		FromPort: 61001,
		ToPort:   61010,
		Protocol: "tcp",
	}
	paramsPortRange := params.FromNetworkPortRange(networkPortRange)
	networkPortRangeBack := paramsPortRange.NetworkPortRange()
	c.Assert(networkPortRange, jc.DeepEquals, networkPortRangeBack)
}

func (s *NetworkSuite) TestAddressConvenience(c *gc.C) {
	networkAddress := network.Address{
		Value:       "foo",
		Type:        network.IPv4Address,
		NetworkName: "bar",
		Scope:       network.ScopePublic,
	}
	paramsAddress := params.FromNetworkAddress(networkAddress)
	c.Assert(networkAddress, jc.DeepEquals, paramsAddress.NetworkAddress())
}

func (s *NetworkSuite) TestHostPortConvenience(c *gc.C) {
	networkAddress := network.Address{
		Value:       "foo",
		Type:        network.IPv4Address,
		NetworkName: "bar",
		Scope:       network.ScopePublic,
	}
	networkHostPort := network.HostPort{networkAddress, 4711}
	paramsHostPort := params.FromNetworkHostPort(networkHostPort)
	c.Assert(networkHostPort, jc.DeepEquals, paramsHostPort.NetworkHostPort())
}
