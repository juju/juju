// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package core

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// GetVolumeBackupPolicyAssignmentRequest wrapper for the GetVolumeBackupPolicyAssignment operation
type GetVolumeBackupPolicyAssignmentRequest struct {

	// The OCID of the volume backup policy assignment.
	PolicyAssignmentId *string `mandatory:"true" contributesTo:"path" name:"policyAssignmentId"`
}

func (request GetVolumeBackupPolicyAssignmentRequest) String() string {
	return common.PointerString(request)
}

// GetVolumeBackupPolicyAssignmentResponse wrapper for the GetVolumeBackupPolicyAssignment operation
type GetVolumeBackupPolicyAssignmentResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// The VolumeBackupPolicyAssignment instance
	VolumeBackupPolicyAssignment `presentIn:"body"`

	// For optimistic concurrency control. See `if-match`.
	Etag *string `presentIn:"header" name:"etag"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about
	// a particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response GetVolumeBackupPolicyAssignmentResponse) String() string {
	return common.PointerString(response)
}
