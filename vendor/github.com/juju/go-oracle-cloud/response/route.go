// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

import "github.com/juju/go-oracle-cloud/common"

// Route is after creating IP networks and adding
// instances to the networks, you can specify connections
// between different networks by creating routes and IP
// network exchanges.
// Route specifies the IP address of the destination as
// well as a vNICset which provides the next hop for
// routing packets. Using routes to enable traffic between subnets
// allows you to specify multiple routes to each IP network.
// Using vNICsets in routes also provides egress load balancing
// and high availability.
type Route struct {

	// Specify common.AdminDistanceZero or common.AdminDistranceOne,
	// or common.AdminDistranceTwo,
	// as the route's administrative distance.
	// If you do not specify a value, the
	// default value is common.AdminDistanceZero,
	// The same prefix can be used in multiple routes.
	// In this case, packets are routed over all the matching routes with
	// the lowest administrative distance. In the case multiple routes with
	// the same lowest administrative distance match,
	// routing occurs over all these routes using ECMP.
	AdminDistance common.AdminDistance `json:"adminDistance,omitempty"`

	// Description is the description of the route
	Description string `json:"description,omitempty,omitempty"`

	// IpAddressPrefix is the IPv4 address prefix, in CIDR format,
	// of the external network (external to the vNIC set)
	// from which you want to route traffic.
	IpAddressPrefix string `json:"ipAddressPrefix"`

	// Name is the name of the route
	Name string `json:"name"`

	// NextHopVnicSet is the name of the virtual NIC set to route matching
	// packets to. Routed flows are load-balanced among all
	// the virtual NICs in the virtual NIC set.
	NextHopVnicSet string `json:"nextHopVnicSet"`

	// Tags associated with the object.
	Tags []string `json:"tags,omitempty"`

	// Uri si the Uniform Resource Identifier
	Uri string `json:"uri"`
}

// AllRoutes contains all declared routes found in the
// oracle cloud account
type AllRoutes struct {
	// Result holds a slice of all routes
	Result []Route `json:"result,omitempty"`
}
