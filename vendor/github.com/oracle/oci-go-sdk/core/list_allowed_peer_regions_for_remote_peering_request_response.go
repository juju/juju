// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package core

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// ListAllowedPeerRegionsForRemotePeeringRequest wrapper for the ListAllowedPeerRegionsForRemotePeering operation
type ListAllowedPeerRegionsForRemotePeeringRequest struct {
}

func (request ListAllowedPeerRegionsForRemotePeeringRequest) String() string {
	return common.PointerString(request)
}

// ListAllowedPeerRegionsForRemotePeeringResponse wrapper for the ListAllowedPeerRegionsForRemotePeering operation
type ListAllowedPeerRegionsForRemotePeeringResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// The []PeerRegionForRemotePeering instance
	Items []PeerRegionForRemotePeering `presentIn:"body"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about
	// a particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response ListAllowedPeerRegionsForRemotePeeringResponse) String() string {
	return common.PointerString(response)
}
