// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
)

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
