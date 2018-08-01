// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package identity

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// UpdateTagNamespaceRequest wrapper for the UpdateTagNamespace operation
type UpdateTagNamespaceRequest struct {

	// The OCID of the tag namespace.
	TagNamespaceId *string `mandatory:"true" contributesTo:"path" name:"tagNamespaceId"`

	// Request object for updating a namespace.
	UpdateTagNamespaceDetails `contributesTo:"body"`
}

func (request UpdateTagNamespaceRequest) String() string {
	return common.PointerString(request)
}

// UpdateTagNamespaceResponse wrapper for the UpdateTagNamespace operation
type UpdateTagNamespaceResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// The TagNamespace instance
	TagNamespace `presentIn:"body"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about a
	// particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response UpdateTagNamespaceResponse) String() string {
	return common.PointerString(response)
}
