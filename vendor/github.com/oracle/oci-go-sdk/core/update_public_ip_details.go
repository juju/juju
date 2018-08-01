// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

// Core Services API
//
// APIs for Networking Service, Compute Service, and Block Volume Service.
//

package core

import (
	"github.com/oracle/oci-go-sdk/common"
)

// UpdatePublicIpDetails The representation of UpdatePublicIpDetails
type UpdatePublicIpDetails struct {

	// A user-friendly name. Does not have to be unique, and it's changeable. Avoid
	// entering confidential information.
	DisplayName *string `mandatory:"false" json:"displayName"`

	// The OCID of the private IP to assign the public IP to.
	// * If the public IP is already assigned to a different private IP, it will be unassigned
	// and then reassigned to the specified private IP.
	// * If you set this field to an empty string, the public IP will be unassigned from the
	// private IP it is currently assigned to.
	PrivateIpId *string `mandatory:"false" json:"privateIpId"`
}

func (m UpdatePublicIpDetails) String() string {
	return common.PointerString(m)
}
