// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

type networkInfoSuite struct {
}

var _ = gc.Suite(&networkInfoSuite{})

// TestNetworkInfoDedupLogic ensures that we don't get a regression for
// LP1864072.
func (s *networkInfoSuite) TestNetworkInfoDedupLogic(c *gc.C) {
	resWithDups := params.NetworkInfoResults{
		Results: map[string]params.NetworkInfoResult{
			"ep0": {
				Error: &params.Error{Message: "these are not the interfaces you are looking for"},
			},
			"ep1": {
				EgressSubnets: []string{
					"8.8.8.8/32",
					"172.31.46.122/32",
					"172.31.46.122/32",
				},
				IngressAddresses: []string{
					"1.2.3.4",
					"172.31.46.122",
					"172.31.46.122",
				},
			},
			"ep2": {
				EgressSubnets: []string{
					"8.8.8.8/32",
				},
				IngressAddresses: []string{
					"1.1.1.1",
				},
				Info: []params.NetworkInfo{
					{
						MACAddress:    "ee:19:50:1f:3e:9a",
						InterfaceName: "es0",
						Addresses: []params.InterfaceAddress{
							{
								Hostname: "foo",
								Address:  "172.31.10.10",
								CIDR:     "172.31.10.10/32",
							},
							{
								Hostname: "foo",
								Address:  "172.31.10.10",
								CIDR:     "172.31.10.10/32",
							},
							{
								Hostname: "bar",
								Address:  "172.31.10.11",
								CIDR:     "172.31.10.11/32",
							},
						},
					},
					{
						MACAddress:    "ee:18:40:1f:3e:fe",
						InterfaceName: "es1",
						Addresses: []params.InterfaceAddress{
							{
								Hostname: "foo",
								Address:  "172.31.42.10",
								CIDR:     "172.31.42.10/32",
							},
						},
					},
				},
			},
		},
	}

	expRes := params.NetworkInfoResults{
		Results: map[string]params.NetworkInfoResult{
			"ep0": {
				Error: &params.Error{Message: "these are not the interfaces you are looking for"},
			},
			"ep1": {
				EgressSubnets: []string{
					"8.8.8.8/32",
					"172.31.46.122/32",
				},
				IngressAddresses: []string{
					"1.2.3.4",
					"172.31.46.122",
				},
			},
			"ep2": {
				EgressSubnets: []string{
					"8.8.8.8/32",
				},
				IngressAddresses: []string{
					"1.1.1.1",
				},
				Info: []params.NetworkInfo{
					{
						MACAddress:    "ee:19:50:1f:3e:9a",
						InterfaceName: "es0",
						Addresses: []params.InterfaceAddress{
							{
								Hostname: "foo",
								Address:  "172.31.10.10",
								CIDR:     "172.31.10.10/32",
							},
							{
								Hostname: "bar",
								Address:  "172.31.10.11",
								CIDR:     "172.31.10.11/32",
							},
						},
					},
					{
						MACAddress:    "ee:18:40:1f:3e:fe",
						InterfaceName: "es1",
						Addresses: []params.InterfaceAddress{
							{
								Hostname: "foo",
								Address:  "172.31.42.10",
								CIDR:     "172.31.42.10/32",
							},
						},
					},
				},
			},
		},
	}

	filteredRes := dedupNetworkInfoResults(resWithDups)
	c.Assert(filteredRes, gc.DeepEquals, expRes)
}
