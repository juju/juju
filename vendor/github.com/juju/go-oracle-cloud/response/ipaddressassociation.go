// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

// IpAddressAssociation IP address association
// to associate an IP address reservation
type IpAddressAssociation struct {
	// Name is the name of the ip address association
	Name string `json:"name"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`

	// Description is the description of the object.
	Description string `json:"description,omitempty"`

	// Tags associated with the object.
	Tags []string `json:"tags,omitempty"`

	//Vnic represents the multipart name of the virtual
	// NIC associated with this NAT IP reservation
	Vnic string `json:"vnic"`

	// IpAddressReservation is the NAT IP address reservation
	IpAddressReservation string `json:"ipAddressReservation"`
}

// AllIpAddressAssociations holds a slice of
// all the ip address associations in the oracle account
type AllIpAddressAssociations struct {
	// Result is the hole resutl of the AllIpAddressAssociation call
	Result []IpAddressAssociation `json:"result,omitempty"`
}
