// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package identity

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// GetTagNamespaceRequest wrapper for the GetTagNamespace operation
type GetTagNamespaceRequest struct {

	// The OCID of the tag namespace.
	TagNamespaceId *string `mandatory:"true" contributesTo:"path" name:"tagNamespaceId"`
}

func (request GetTagNamespaceRequest) String() string {
	return common.PointerString(request)
}

// GetTagNamespaceResponse wrapper for the GetTagNamespace operation
type GetTagNamespaceResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// The TagNamespace instance
	TagNamespace `presentIn:"body"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about a
	// particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response GetTagNamespaceResponse) String() string {
	return common.PointerString(response)
}
