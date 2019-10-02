// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
)

type GetOpsForHostPortsSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&GetOpsForHostPortsSuite{})

func (s *GetOpsForHostPortsSuite) TestGetOpsForHostPortsChangeWithSpaces(c *gc.C) {
	addresses := map[string][]network.HostPort{
		"0": {
			{
				Address: network.Address{
					Value:           "10.144.9.113",
					Type:            "ipv4",
					Scope:           "local-cloud",
					SpaceName:       "default",
					SpaceProviderId: "foo",
				},
				Port: 17070,
			}, {
				Address: network.Address{
					Value: "127.0.0.1",
					Type:  "ipv4",
					Scope: "local-machine",
				},
				Port: 17070,
			},
		},
		"1": {
			{
				Address: network.Address{
					Value:           "10.144.9.62",
					Type:            "ipv4",
					Scope:           "local-cloud",
					SpaceName:       "default",
					SpaceProviderId: "foo",
				},
				Port: 17070,
			}, {
				Address: network.Address{
					Value: "127.0.0.1",
					Type:  "ipv4",
					Scope: "local-machine",
				},
				Port: 17070,
			},
		},
		"2": {
			{
				Address: network.Address{
					Value:           "10.144.9.56",
					Type:            "ipv4",
					Scope:           "local-cloud",
					SpaceName:       "default",
					SpaceProviderId: "foo",
				},
				Port: 17070,
			}, {
				Address: network.Address{
					Value: "127.0.0.1",
					Type:  "ipv4",
					Scope: "local-machine",
				},
				Port: 17070,
			},
		},
	}
	// Iterate the map each time to generate the slice with the machines
	// in a different order.
	addressSlice := func() [][]network.HostPort {
		var result [][]network.HostPort
		for _, value := range addresses {
			result = append(result, value)
		}
		return result
	}

	controllers, closer := s.state.db().GetCollection(controllersC)
	defer closer()

	ops, err := s.state.getOpsForHostPortsChange(controllers, apiHostPortsKey, addressSlice())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(ops), jc.GreaterThan, 0)
	// Run the ops.
	err = s.state.db().RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)

	// Now iterate over the map a few times to get different ordering, and assert the
	// ops to update the host ports is empty.
	for i := 0; i < 5; i++ {
		ops, err := s.state.getOpsForHostPortsChange(controllers, apiHostPortsKey, addressSlice())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ops, gc.HasLen, 0)
	}
}

type AddressEqualitySuite struct{}

var _ = gc.Suite(&AddressEqualitySuite{})

func (*AddressEqualitySuite) TestHostPortsEqual(c *gc.C) {
	first := [][]network.HostPort{
		{
			{
				Address: network.Address{
					Value: "10.144.9.113",
					Type:  "ipv4",
					Scope: "local-cloud",
				},
				Port: 17070,
			}, {
				Address: network.Address{
					Value: "127.0.0.1",
					Type:  "ipv4",
					Scope: "local-machine",
				},
				Port: 17070,
			},
		}, {
			{
				Address: network.Address{
					Value: "10.144.9.62",
					Type:  "ipv4",
					Scope: "local-cloud",
				},
				Port: 17070,
			}, {
				Address: network.Address{
					Value: "127.0.0.1",
					Type:  "ipv4",
					Scope: "local-machine",
				},
				Port: 17070,
			},
		}, {
			{
				Address: network.Address{
					Value: "10.144.9.56",
					Type:  "ipv4",
					Scope: "local-cloud",
				},
				Port: 17070,
			}, {
				Address: network.Address{
					Value: "127.0.0.1",
					Type:  "ipv4",
					Scope: "local-machine",
				},
				Port: 17070,
			},
		},
	}
	// second is the same as first with the first set of machines at the
	// end rather than the start.
	second := [][]network.HostPort{
		{
			{
				Address: network.Address{
					Value: "10.144.9.62",
					Type:  "ipv4",
					Scope: "local-cloud",
				},
				Port: 17070,
			}, {
				Address: network.Address{
					Value: "127.0.0.1",
					Type:  "ipv4",
					Scope: "local-machine",
				},
				Port: 17070,
			},
		}, {
			{
				Address: network.Address{
					Value: "10.144.9.56",
					Type:  "ipv4",
					Scope: "local-cloud",
				},
				Port: 17070,
			}, {
				Address: network.Address{
					Value: "127.0.0.1",
					Type:  "ipv4",
					Scope: "local-machine",
				},
				Port: 17070,
			},
		}, {
			{
				Address: network.Address{
					Value: "10.144.9.113",
					Type:  "ipv4",
					Scope: "local-cloud",
				},
				Port: 17070,
			}, {
				Address: network.Address{
					Value: "127.0.0.1",
					Type:  "ipv4",
					Scope: "local-machine",
				},
				Port: 17070,
			},
		},
	}
	c.Assert(hostsPortsEqual(first, second), jc.IsTrue)
}

func (s *AddressEqualitySuite) TestAddressConversion(c *gc.C) {
	machineAddress := network.Address{
		Value: "foo",
		Type:  network.IPv4Address,
		Scope: network.ScopePublic,
	}
	stateAddress := fromNetworkAddress(machineAddress, "machine")
	c.Assert(machineAddress, jc.DeepEquals, stateAddress.networkAddress())

	providerAddress := network.Address{
		Value:           "bar",
		Type:            network.IPv4Address,
		Scope:           network.ScopePublic,
		SpaceName:       "test-space",
		SpaceProviderId: "666",
	}
	stateAddress = fromNetworkAddress(providerAddress, "provider")
	c.Assert(providerAddress, jc.DeepEquals, stateAddress.networkAddress())
}
