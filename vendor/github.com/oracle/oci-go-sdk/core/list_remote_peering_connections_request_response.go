// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package core

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// ListRemotePeeringConnectionsRequest wrapper for the ListRemotePeeringConnections operation
type ListRemotePeeringConnectionsRequest struct {

	// The OCID of the compartment.
	CompartmentId *string `mandatory:"true" contributesTo:"query" name:"compartmentId"`

	// The OCID of the DRG.
	DrgId *string `mandatory:"false" contributesTo:"query" name:"drgId"`

	// The maximum number of items to return in a paginated "List" call.
	// Example: `500`
	Limit *int `mandatory:"false" contributesTo:"query" name:"limit"`

	// The value of the `opc-next-page` response header from the previous "List" call.
	Page *string `mandatory:"false" contributesTo:"query" name:"page"`
}

func (request ListRemotePeeringConnectionsRequest) String() string {
	return common.PointerString(request)
}

// ListRemotePeeringConnectionsResponse wrapper for the ListRemotePeeringConnections operation
type ListRemotePeeringConnectionsResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// The []RemotePeeringConnection instance
	Items []RemotePeeringConnection `presentIn:"body"`

	// A pagination token to the start of the next page, if one exist.
	OpcNextPage *string `presentIn:"header" name:"opc-next-page"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about
	// a particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response ListRemotePeeringConnectionsResponse) String() string {
	return common.PointerString(response)
}
