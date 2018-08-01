// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package identity

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// UpdateDynamicGroupRequest wrapper for the UpdateDynamicGroup operation
type UpdateDynamicGroupRequest struct {

	// The OCID of the dynamic group.
	DynamicGroupId *string `mandatory:"true" contributesTo:"path" name:"dynamicGroupId"`

	// Request object for updating an dynamic group.
	UpdateDynamicGroupDetails `contributesTo:"body"`

	// For optimistic concurrency control. In the PUT or DELETE call for a resource, set the `if-match`
	// parameter to the value of the etag from a previous GET or POST response for that resource.  The resource
	// will be updated or deleted only if the etag you provide matches the resource's current etag value.
	IfMatch *string `mandatory:"false" contributesTo:"header" name:"if-match"`
}

func (request UpdateDynamicGroupRequest) String() string {
	return common.PointerString(request)
}

// UpdateDynamicGroupResponse wrapper for the UpdateDynamicGroup operation
type UpdateDynamicGroupResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// The DynamicGroup instance
	DynamicGroup `presentIn:"body"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about a
	// particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`

	// For optimistic concurrency control. See `if-match`.
	Etag *string `presentIn:"header" name:"etag"`
}

func (response UpdateDynamicGroupResponse) String() string {
	return common.PointerString(response)
}
