// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

import "github.com/juju/go-oracle-cloud/common"

// IpAssociation is a link between an IP reservation
// and the vcable of an instance. A vcable is an attachment
// point to a specific network interface of an instance.
// A vcable is created automatically when an instance is created
// and is deleted when the instance is deleted.
type IpAssociation struct {

	// Account is  the default account
	// for your identity domain.
	Account string `json:"account"`

	// Ip is the public IP address which is
	// attached to an Oracle Compute Cloud Service
	// instance that requires access to or from the Internet.
	Ip string `json:"ip"`

	// Name is the name of the ip association
	Name string `json:"name"`

	//To associate a temporary IP address from the pool,
	// specify ippool:/oracle/public/ippool.
	// To associate a persistent IP address, specify // ipreservation:ipreservation_name, where ipreservation_name
	// is three-part name of an existing IP reservation in the // /Compute-identity_domain/user/object_name format. For more
	// information about how to create an IP reservation,
	// see CreateIpReservation.
	Parentpool common.IPPool `json:"parentpool"`

	// Reservation is the  ip reservation object in the format name
	Reservation string `json:"reservation"`

	// Uri is the uniform Resource Identifier
	Uri string `json:"uri"`

	// Vcable is the vcable it of the instance that
	// is associated with the ip reservation.
	Vcable common.VcableID `json:"vcable"`
}

// AllIpAssociations holds all ip association
// in the oracle account
type AllIpAssociations struct {
	Result []IpAssociation `json:"result,omitempty"`
}
