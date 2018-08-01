// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package identity

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// DeleteSmtpCredentialRequest wrapper for the DeleteSmtpCredential operation
type DeleteSmtpCredentialRequest struct {

	// The OCID of the user.
	UserId *string `mandatory:"true" contributesTo:"path" name:"userId"`

	// The OCID of the SMTP credential.
	SmtpCredentialId *string `mandatory:"true" contributesTo:"path" name:"smtpCredentialId"`

	// For optimistic concurrency control. In the PUT or DELETE call for a resource, set the `if-match`
	// parameter to the value of the etag from a previous GET or POST response for that resource.  The resource
	// will be updated or deleted only if the etag you provide matches the resource's current etag value.
	IfMatch *string `mandatory:"false" contributesTo:"header" name:"if-match"`
}

func (request DeleteSmtpCredentialRequest) String() string {
	return common.PointerString(request)
}

// DeleteSmtpCredentialResponse wrapper for the DeleteSmtpCredential operation
type DeleteSmtpCredentialResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about a
	// particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response DeleteSmtpCredentialResponse) String() string {
	return common.PointerString(response)
}
