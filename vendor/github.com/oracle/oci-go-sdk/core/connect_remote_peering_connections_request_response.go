// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package core

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// ConnectRemotePeeringConnectionsRequest wrapper for the ConnectRemotePeeringConnections operation
type ConnectRemotePeeringConnectionsRequest struct {

	// The OCID of the remote peering connection (RPC).
	RemotePeeringConnectionId *string `mandatory:"true" contributesTo:"path" name:"remotePeeringConnectionId"`

	// Details to connect peering connection with peering connection from remote region
	ConnectRemotePeeringConnectionsDetails `contributesTo:"body"`
}

func (request ConnectRemotePeeringConnectionsRequest) String() string {
	return common.PointerString(request)
}

// ConnectRemotePeeringConnectionsResponse wrapper for the ConnectRemotePeeringConnections operation
type ConnectRemotePeeringConnectionsResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about
	// a particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response ConnectRemotePeeringConnectionsResponse) String() string {
	return common.PointerString(response)
}
