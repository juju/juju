// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package core

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// GetRemotePeeringConnectionRequest wrapper for the GetRemotePeeringConnection operation
type GetRemotePeeringConnectionRequest struct {

	// The OCID of the remote peering connection (RPC).
	RemotePeeringConnectionId *string `mandatory:"true" contributesTo:"path" name:"remotePeeringConnectionId"`
}

func (request GetRemotePeeringConnectionRequest) String() string {
	return common.PointerString(request)
}

// GetRemotePeeringConnectionResponse wrapper for the GetRemotePeeringConnection operation
type GetRemotePeeringConnectionResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// The RemotePeeringConnection instance
	RemotePeeringConnection `presentIn:"body"`

	// For optimistic concurrency control. See `if-match`.
	Etag *string `presentIn:"header" name:"etag"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about
	// a particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response GetRemotePeeringConnectionResponse) String() string {
	return common.PointerString(response)
}
