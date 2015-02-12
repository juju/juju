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

type M map[string]interface{}

type NetworkSuite struct{}

var _ = gc.Suite(&NetworkSuite{})

func (s *NetworkSuite) TestPortsResults(c *gc.C) {
	createResults := func(rs ...interface{}) M {
		return M{"Results": rs}
	}
	createResult := func(err, ports interface{}) M {
		return M{"Error": err, "Ports": ports}
	}
	createError := func(msg, code string) M {
		return M{"Message": msg, "Code": code}
	}
	createPort := func(prot string, num int) M {
		return M{"Protocol": prot, "Number": num}
	}
	tests := []struct {
		about    string
		results  params.PortsResults
		expected M
	}{{
		about: "one error",
		results: params.PortsResults{
			Results: []params.PortsResult{
				params.PortsResult{Error: &params.Error{"I failed", "ERR42"}},
			},
		},
		expected: createResults(createResult(createError("I failed", "ERR42"), nil)),
	}, {
		about: "one port",
		results: params.PortsResults{
			Results: []params.PortsResult{
				params.PortsResult{Ports: []params.Port{{"http", 80}}},
			},
		},
		expected: createResults(createResult(nil, []interface{}{createPort("http", 80)})),
	}}
	for i, test := range tests {
		c.Logf("\ntest %d: %s", i, test.about)
		output, err := json.Marshal(test.results)
		c.Assert(err, jc.ErrorIsNil)
		c.Logf("\nJSON output:\n%v", string(output))
		c.Assert(string(output), jc.JSONEquals, test.expected)
	}
}

// TODO(dimitern): apiserver/params should not depend on the network
// package: network types used as fields in request/response structs
// should be replaced with equivalent string, []string, or [][]string
// types, so the wire-format of the API protocol will remain stable,
// even if a network type changes its serialization format.
//
// This test ensures the following network package types used as
// fields are still properly serialized and deserialized:
//
// params.PortsResult.Ports                []params.Port
// params.MachinePortRange.PortRange       network.PortRange
// params.MachineAddresses.Addresses       []network.Address
// params.AddMachineParams.Addrs           []network.Address
// params.RsyslogConfigResult.HostPorts    []params.HostPort
// params.APIHostPortsResult.Servers       [][]params.HostPort
// params.LoginResult.Servers              [][]params.HostPort
// params.LoginResultV1.Servers            [][]params.HostPort
//func (s *NetworkSuite) TestNetworkEntities(c *gc.C) {
//	setPort := func(addrs []params.HostPort, port int) []params.HostPort {
//		hps := make([]params.HostPort, len(addrs))
//		for i, addr := range addrs {
//			hps[i] = params.HostPort{
//				Value:       addr.Value,
//				Type:        addr.Type,
//				NetworkName: addr.NetworkName,
//				Scope:       addr.Scope,
//				Port:        port,
//			}
//		}
//		return hps
//	}
//	allBaseHostPorts := []params.HostPort{
//		{Value: "foo0", Type: "bar0", NetworkName: "baz0", Scope: "none0"},
//		{Type: "bar1", NetworkName: "baz1", Scope: "none1"},
//		{Value: "foo2", NetworkName: "baz2", Scope: "none2"},
//		{Value: "foo3", Type: "bar3", Scope: "none3"},
//		{Value: "foo4", Type: "bar4", NetworkName: "baz4"},
//		{Value: "foo5", Type: "bar5"},
//		{Value: "foo6"},
//		{},
//	}
//	allHostPortCombos := setPort(allBaseHostPorts, 1234)
//	allHostPortCombos = append(allHostPortCombos, setPort(allBaseHostPorts, 0)...)
//	allServerHostPorts := [][]params.HostPort{
//		allHostPortCombos,
//		setPort(allBaseHostPorts, 0),
//		setPort(allBaseHostPorts, 1234),
//		{},
//	}
//
//	for i, test := range []struct {
//		about string
//		input interface{}
//	}{{
//		about: "params.PortResult.Ports",
//		input: []params.PortsResult{{
//			Ports: []params.Port{
//				{Protocol: "foo", Number: 42},
//				{Protocol: "bar"},
//				{Number: 99},
//				{},
//			},
//		}, {},
//		},
//	}, {
//		about: "params.MachinePortRange.PortRange",
//		input: []params.MachinePortRange{{
//			UnitTag:     "foo",
//			RelationTag: "bar",
//			PortRange:   network.PortRange{FromPort: 42, ToPort: 69, Protocol: "baz"},
//		}, {
//			PortRange: network.PortRange{ToPort: 69, Protocol: "foo"},
//		}, {
//			PortRange: network.PortRange{FromPort: 42, Protocol: "bar"},
//		}, {
//			PortRange: network.PortRange{Protocol: "baz"},
//		}, {
//			PortRange: network.PortRange{FromPort: 42, ToPort: 69},
//		}, {
//			PortRange: network.PortRange{},
//		}, {},
//		},
//	}, {
//		about: "params.MachineAddresses.Addresses",
//		input: []params.MachineAddresses{{
//			Tag:       "foo",
//			Addresses: allBaseHostPorts,
//		}, {},
//		},
//	}, {
//		about: "params.AddMachineParams.Addrs",
//		input: []params.AddMachineParams{{
//			Series:    "foo",
//			ParentId:  "bar",
//			Placement: nil,
//			Addrs:     allBaseHostPorts,
//		}, {},
//		},
//	}, {
//		about: "params.RsyslogConfigResult.HostPorts",
//		input: []params.RsyslogConfigResult{{
//			CACert:    "fake",
//			HostPorts: allHostPortCombos,
//		}, {},
//		},
//	}, {
//		about: "params.APIHostPortsResult.Servers",
//		input: []params.APIHostPortsResult{{
//			Servers: allServerHostPorts,
//		}, {},
//		},
//	}, {
//		about: "params.LoginResult.Servers",
//		input: []params.LoginResult{{
//			Servers: allServerHostPorts,
//		}, {},
//		},
//	}, {
//		about: "params.LoginResultV1.Servers",
//		input: []params.LoginResultV1{{
//			Servers: allServerHostPorts,
//		}, {},
//		},
//	}} {
//		c.Logf("\ntest %d: %s", i, test.about)
//		output, err := json.Marshal(test.input)
//		c.Assert(err, jc.ErrorIsNil)
//		c.Logf("\nJSON output:\n%v", string(output))
//		c.Assert(string(output), jc.JSONEquals, test.input)
//	}
//}

func (s *NetworkSuite) TestAddressConvenience(c *gc.C) {
	networkAddress := network.Address{
		Value:       "foo",
		Type:        network.IPv4Address,
		NetworkName: "bar",
		Scope:       network.ScopePublic,
	}
	paramsAddress := params.FromNetworkAddress(networkAddress)
	c.Assert(networkAddress, gc.DeepEquals, paramsAddress.NetworkAddress())
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
	c.Assert(networkHostPort, gc.DeepEquals, paramsHostPort.NetworkHostPort())
}
