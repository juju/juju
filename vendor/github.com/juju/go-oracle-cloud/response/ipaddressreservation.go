// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

import "github.com/juju/go-oracle-cloud/common"

// IpAddressReservation is when you associate an instance with an
// IP network, you can specify a public IP address to
// be associated with the instance.
// An IP address reservation allows you to reserve a public
// IP address from a specified IP pool. After creating an
// IP address reservation, you can associate the public IP address with
// a vNIC on an instance by creating an IP address association.
type IpAddressReservation struct {

	// Description is the description of the ip address reservation
	Description *string `json:"description,omitmepty"`

	// IpAddressPool is the IP address pool from which you want
	// to reserve an IP address. Enter one of the following:
	// * /oracle/public/public-ippool: When you attach an IP address
	// from this pool to an instance, you enable
	// access between the public Internet and the instance.
	// * /oracle/public/cloud-ippool: When you attach
	// an IP address from this pool to an instance, the instance
	// can communicate privately (that is, without traffic going over
	// the public Internet) with other Oracle Cloud services, such as the
	IpAddressPool common.IPPool `json:"ipAddressPool,omitempty"`

	// IpAddress reserved NAT IPv4 address from the IP address pool
	IpAddress string `json:"ipAddress,omitempty"`

	// Name is the name of the ip address reservation
	Name string `json:"name"`

	// Tags is the strings that you can use to tag the IP address reservation.
	Tags []string `json:"tags,omitempty"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`
}

// AllIpAddressReservations is holds a slice of all ip address
// reservations inside the oracle cloud account
type AllIpAddressReservations struct {
	// Result of all ip address reservations
	Result []IpAddressReservation `json:"result,omitempty"`
}
