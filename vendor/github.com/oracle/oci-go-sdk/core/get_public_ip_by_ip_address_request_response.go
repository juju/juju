// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package core

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// GetPublicIpByIpAddressRequest wrapper for the GetPublicIpByIpAddress operation
type GetPublicIpByIpAddressRequest struct {

	// IP address details for fetching the public IP.
	GetPublicIpByIpAddressDetails `contributesTo:"body"`
}

func (request GetPublicIpByIpAddressRequest) String() string {
	return common.PointerString(request)
}

// GetPublicIpByIpAddressResponse wrapper for the GetPublicIpByIpAddress operation
type GetPublicIpByIpAddressResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// The PublicIp instance
	PublicIp `presentIn:"body"`

	// For optimistic concurrency control. See `if-match`.
	Etag *string `presentIn:"header" name:"etag"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about a
	// particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response GetPublicIpByIpAddressResponse) String() string {
	return common.PointerString(response)
}
