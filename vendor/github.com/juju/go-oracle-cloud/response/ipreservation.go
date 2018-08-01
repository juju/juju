// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

import "github.com/juju/go-oracle-cloud/common"

// IpReservation is an IP reservation is the allocation of a public IP address
// from an IP address pool. After creating an IP reservation,
// you can associate it with an instance by using an IP
// association, to enable access between the Internet and the instance.
type IpReservation struct {
	// Account is  the default account
	// for your identity domain.
	Account string `json:"account"`

	// Ip is an IP reservation is a public IP address
	// that you can attach to an Oracle Compute Cloud
	// Service instance that requires
	//  access to or from the Internet.
	Ip string `json:"ip"`

	// Name is the name of the ip reservation
	Name string `json:"name"`

	// Parentpool is a pool of public IP addresses
	Parentpool common.IPPool `json:"parentpool"`

	// Permanent flag is true and indicates that the IP reservation
	// has a persistent public IP address.
	// You can associate either a temporary or a persistent
	// public IP address with an instance when
	// you create the instance.
	// Temporary public IP addresses are assigned
	// dynamically from a pool of public IP addresses.
	// When you associate a temporary public IP address with an instance,
	// if the instance is restarted or is deleted and created again later,
	// its public IP address might change.
	Permanent bool `json:"permanent"`

	// Quota is Not used
	Quota *string `json:"quota,omitempty"`

	// Tags is a comma-separated list of strings
	// which helps you to identify IP reservation.
	Tags []string `json:"tags,omitempty"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`

	// Used flag when is true indicates that the IP reservation
	//  is associated with an instance.
	Used bool `json:"used"`
}

// AllIpReservations holds all ip reservation in the
// oracle cloud account
type AllIpReservations struct {
	Result []IpReservation `json:"result,omitmepty"`
}
