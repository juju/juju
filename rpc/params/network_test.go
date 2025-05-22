// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params_test

import (
	"encoding/json"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
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

func TestNetworkSuite(t *testing.T) {
	tc.Run(t, &NetworkSuite{})
}

func (s *NetworkSuite) TestPortsResults(c *tc.C) {
	// Convenience helpers.
	mkPortsResults := func(prs ...params.PortsResult) params.PortsResults {
		return params.PortsResults{
			Results: prs,
		}
	}
	mkPortsResult := func(msg, code string, ports ...P) params.PortsResult {
		pr := params.PortsResult{}
		if msg != "" {
			pr.Error = &params.Error{Message: msg, Code: code}
		}
		for _, p := range ports {
			pr.Ports = append(pr.Ports, params.Port{p.prot, p.num})
		}
		return pr
	}
	mkResults := func(rs ...interface{}) M {
		return M{"results": rs}
	}
	mkResult := func(err, ports interface{}) M {
		result := M{"ports": ports}
		if err != nil {
			result["error"] = err
		}
		return result
	}
	mkError := func(msg, code string) M {
		return M{"message": msg, "code": code}
	}
	mkPort := func(prot string, num int) M {
		return M{"protocol": prot, "number": num}
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
		if !c.Check(err, tc.ErrorIsNil) {
			continue
		}
		c.Logf("\nJSON output:\n%v", string(output))
		c.Check(string(output), tc.JSONEquals, test.expected)
	}
}

func (s *NetworkSuite) TestHostPort(c *tc.C) {
	mkHostPort := func(v, t, s string, p int) M {
		return M{
			"value": v,
			"type":  t,
			"scope": s,
			"port":  p,
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
		expected: mkHostPort("foo", "", "", 1234),
	}, {
		about: "address value and type; port is 1234",
		hostPort: params.HostPort{
			Address: params.Address{
				Value: "foo",
				Type:  "ipv4",
			},
			Port: 1234,
		},
		expected: mkHostPort("foo", "ipv4", "", 1234),
	}, {
		about: "address value, type, and network name, port is 1234",
		hostPort: params.HostPort{
			Address: params.Address{
				Value: "foo",
				Type:  "ipv4",
			},
			Port: 1234,
		},
		expected: mkHostPort("foo", "ipv4", "", 1234),
	}, {
		about: "address all fields, port is 1234",
		hostPort: params.HostPort{
			Address: params.Address{
				Value: "foo",
				Type:  "ipv4",
				Scope: "public",
			},
			Port: 1234,
		},
		expected: mkHostPort("foo", "ipv4", "public", 1234),
	}, {
		about: "address all fields, port is 0",
		hostPort: params.HostPort{
			Address: params.Address{
				Value: "foo",
				Type:  "ipv4",
				Scope: "public",
			},
			Port: 0,
		},
		expected: mkHostPort("foo", "ipv4", "public", 0),
	}}
	for i, test := range tests {
		c.Logf("\ntest %d: %s", i, test.about)
		output, err := json.Marshal(test.hostPort)
		if !c.Check(err, tc.ErrorIsNil) {
			continue
		}
		c.Logf("\nJSON output:\n%v", string(output))
		c.Check(string(output), tc.JSONEquals, test.expected)
	}
}

func (s *NetworkSuite) TestMachinePortRange(c *tc.C) {
	mkPortRange := func(u, r string, f, t int, p string) M {
		return M{
			"unit-tag":     u,
			"relation-tag": r,
			"port-range": M{
				"from-port": f,
				"to-port":   t,
				"protocol":  p,
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
		if !c.Check(err, tc.ErrorIsNil) {
			continue
		}
		c.Logf("\nJSON output:\n%v", string(output))
		c.Check(string(output), tc.JSONEquals, test.expected)
	}
}

func (s *NetworkSuite) TestPortRangeConvenience(c *tc.C) {
	networkPortRange := network.PortRange{
		FromPort: 61001,
		ToPort:   61010,
		Protocol: "tcp",
	}
	paramsPortRange := params.FromNetworkPortRange(networkPortRange)
	networkPortRangeBack := paramsPortRange.NetworkPortRange()
	c.Assert(networkPortRange, tc.DeepEquals, networkPortRangeBack)
}

func (s *NetworkSuite) TestProviderAddressConversion(c *tc.C) {
	pAddrs := network.ProviderAddresses{
		network.NewMachineAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal), network.WithCIDR("1.2.3.0/24")).AsProviderAddress(),
		network.NewMachineAddress("1.2.3.5", network.WithScope(network.ScopeCloudLocal), network.WithSecondary(true)).AsProviderAddress(),
		network.NewMachineAddress("2.3.4.5", network.WithScope(network.ScopePublic), network.WithConfigType("dhcp")).AsProviderAddress(),
	}
	pAddrs[0].SpaceName = "test-space"
	pAddrs[0].ProviderSpaceID = "666"

	addrs := params.FromProviderAddresses(pAddrs...)
	c.Assert(params.ToProviderAddresses(addrs...), tc.DeepEquals, pAddrs)
}

func (s *NetworkSuite) TestMachineAddressConversion(c *tc.C) {
	mAddrs := []network.MachineAddress{
		network.NewMachineAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal), network.WithCIDR("1.2.3.0/24")),
		network.NewMachineAddress("1.2.3.5", network.WithScope(network.ScopeCloudLocal), network.WithSecondary(true)),
		network.NewMachineAddress("2.3.4.5", network.WithScope(network.ScopePublic), network.WithConfigType("dhcp")),
	}

	exp := []params.Address{
		{Value: "1.2.3.4", Scope: string(network.ScopeCloudLocal), Type: string(network.IPv4Address), CIDR: "1.2.3.0/24"},
		{Value: "1.2.3.5", Scope: string(network.ScopeCloudLocal), Type: string(network.IPv4Address), IsSecondary: true},
		{Value: "2.3.4.5", Scope: string(network.ScopePublic), Type: string(network.IPv4Address), ConfigType: "dhcp"},
	}
	c.Assert(params.FromMachineAddresses(mAddrs...), tc.DeepEquals, exp)
}

func (s *NetworkSuite) TestProviderHostPortConversion(c *tc.C) {
	pHPs := []network.ProviderHostPorts{
		{
			{
				ProviderAddress: network.NewMachineAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
				NetPort:         1234,
			},
			{
				ProviderAddress: network.NewMachineAddress("2.3.4.5", network.WithScope(network.ScopePublic)).AsProviderAddress(),
				NetPort:         2345,
			},
		},
		{
			{
				ProviderAddress: network.NewMachineAddress("3.4.5.6", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
				NetPort:         3456,
			},
		},
	}
	pHPs[0][0].SpaceName = "test-space"
	pHPs[0][0].ProviderSpaceID = "666"

	hps := params.FromProviderHostsPorts(pHPs)
	c.Assert(params.ToProviderHostsPorts(hps), tc.DeepEquals, pHPs)
}

func (s *NetworkSuite) TestMachineHostPortConversion(c *tc.C) {
	hps := [][]params.HostPort{
		{
			{
				Address: params.Address{
					Value: "1.2.3.4",
					Scope: string(network.ScopeCloudLocal),
					Type:  string(network.IPv4Address),
				},
				Port: 1234,
			},
			{
				Address: params.Address{
					Value: "2.3.4.5",
					Scope: string(network.ScopePublic),
					Type:  string(network.IPv4Address),
				},
				Port: 2345,
			},
		},
		{
			{
				Address: params.Address{
					Value: "3.4.5.6",
					Scope: string(network.ScopeCloudLocal),
					Type:  string(network.IPv4Address),
				},
				Port: 3456,
			},
		},
	}

	exp := []network.MachineHostPorts{
		{
			{
				MachineAddress: network.NewMachineAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal)),
				NetPort:        1234,
			},
			{
				MachineAddress: network.NewMachineAddress("2.3.4.5", network.WithScope(network.ScopePublic)),
				NetPort:        2345,
			},
		},
		{
			{
				MachineAddress: network.NewMachineAddress("3.4.5.6", network.WithScope(network.ScopeCloudLocal)),
				NetPort:        3456,
			},
		},
	}

	c.Assert(params.ToMachineHostsPorts(hps), tc.DeepEquals, exp)
}

func (s *NetworkSuite) TestHostPortConversion(c *tc.C) {
	mHPs := []network.MachineHostPorts{
		{
			{
				MachineAddress: network.NewMachineAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal)),
				NetPort:        1234,
			},
			{
				MachineAddress: network.NewMachineAddress("2.3.4.5", network.WithScope(network.ScopePublic)),
				NetPort:        2345,
			},
		},
		{
			{
				MachineAddress: network.NewMachineAddress("3.4.5.6", network.WithScope(network.ScopeCloudLocal)),
				NetPort:        3456,
			},
		},
	}

	hps := make([]network.HostPorts, len(mHPs))
	for i, mHP := range mHPs {
		hps[i] = mHP.HostPorts()
	}

	pHPs := params.FromHostsPorts(hps)
	c.Assert(params.ToMachineHostsPorts(pHPs), tc.DeepEquals, mHPs)
}

func (s *NetworkSuite) TestSetNetworkConfigBackFillMachineOrigin(c *tc.C) {
	cfg := params.SetMachineNetworkConfig{
		Tag: "machine-0",
		Config: []params.NetworkConfig{
			{
				ProviderId: "1",
				// This would not happen in the wild, but serves to
				// differentiate from the back-filled entries.
				NetworkOrigin: params.NetworkOrigin(network.OriginProvider),
			},
			{ProviderId: "2"},
			{ProviderId: "3"},
		},
	}

	cfg.BackFillMachineOrigin()
	c.Assert(cfg.Config, tc.DeepEquals, []params.NetworkConfig{
		{
			ProviderId:    "1",
			NetworkOrigin: params.NetworkOrigin(network.OriginProvider),
		},
		{
			ProviderId:    "2",
			NetworkOrigin: params.NetworkOrigin(network.OriginMachine),
		},
		{
			ProviderId:    "3",
			NetworkOrigin: params.NetworkOrigin(network.OriginMachine),
		},
	})
}

func (s *NetworkSuite) TestNetworkConfigFromInterfaceMACNormalization(c *tc.C) {
	in := network.InterfaceInfos{
		{
			// All-caps and dashes
			MACAddress: "00-AA-BB-CC-DD",
		},
		{
			// All-caps and colons
			MACAddress: "00:AA:BB:CC:DD",
		},
		{
			// Already normalized
			MACAddress: "00:aa:bb:cc:dd",
		},
	}

	got := params.NetworkConfigFromInterfaceInfo(in)
	c.Assert(got, tc.DeepEquals, []params.NetworkConfig{
		{
			MACAddress: "00:aa:bb:cc:dd",
		},
		{
			MACAddress: "00:aa:bb:cc:dd",
		},
		{
			MACAddress: "00:aa:bb:cc:dd",
		},
	})
}
func (s *NetworkSuite) TestInterfaceFromNetworkConfigMACNormalization(c *tc.C) {
	cfg := []params.NetworkConfig{
		{
			// All-caps and dashes
			MACAddress:     "AA-BB-CC-DD-EE-FF",
			GatewayAddress: "192.168.0.254",
		},
		{
			// All-caps and colons
			MACAddress:     "AA:BB:CC:DD:EE:FF",
			GatewayAddress: "192.168.0.254",
		},
		{
			// Already normalized
			MACAddress:     "aa:bb:cc:dd:ee:ff",
			GatewayAddress: "192.168.0.254",
		},
	}

	got := params.InterfaceInfoFromNetworkConfig(cfg)
	gwAddr := network.NewMachineAddress("192.168.0.254").AsProviderAddress()
	c.Assert(got, tc.DeepEquals, network.InterfaceInfos{
		{
			MACAddress:     "aa:bb:cc:dd:ee:ff",
			GatewayAddress: gwAddr,
		},
		{
			MACAddress:     "aa:bb:cc:dd:ee:ff",
			GatewayAddress: gwAddr,
		},
		{
			MACAddress:     "aa:bb:cc:dd:ee:ff",
			GatewayAddress: gwAddr,
		},
	})
}
