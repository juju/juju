// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package identity

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// GetTagRequest wrapper for the GetTag operation
type GetTagRequest struct {

	// The OCID of the tag namespace.
	TagNamespaceId *string `mandatory:"true" contributesTo:"path" name:"tagNamespaceId"`

	// The name of the tag.
	TagName *string `mandatory:"true" contributesTo:"path" name:"tagName"`
}

func (request GetTagRequest) String() string {
	return common.PointerString(request)
}

// GetTagResponse wrapper for the GetTag operation
type GetTagResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// The Tag instance
	Tag `presentIn:"body"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about a
	// particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response GetTagResponse) String() string {
	return common.PointerString(response)
}
