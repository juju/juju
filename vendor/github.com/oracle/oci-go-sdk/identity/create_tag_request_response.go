// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package identity

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// CreateTagRequest wrapper for the CreateTag operation
type CreateTagRequest struct {

	// The OCID of the tag namespace.
	TagNamespaceId *string `mandatory:"true" contributesTo:"path" name:"tagNamespaceId"`

	// Request object for creating a new tag in the specified tag namespace.
	CreateTagDetails `contributesTo:"body"`

	// A token that uniquely identifies a request so it can be retried in case of a timeout or
	// server error without risk of executing that same action again. Retry tokens expire after 24
	// hours, but can be invalidated before then due to conflicting operations (e.g., if a resource
	// has been deleted and purged from the system, then a retry of the original creation request
	// may be rejected).
	OpcRetryToken *string `mandatory:"false" contributesTo:"header" name:"opc-retry-token"`
}

func (request CreateTagRequest) String() string {
	return common.PointerString(request)
}

// CreateTagResponse wrapper for the CreateTag operation
type CreateTagResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// The Tag instance
	Tag `presentIn:"body"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about a
	// particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response CreateTagResponse) String() string {
	return common.PointerString(response)
}
