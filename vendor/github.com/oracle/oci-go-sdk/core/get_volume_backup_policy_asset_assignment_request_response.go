// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package core

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// GetVolumeBackupPolicyAssetAssignmentRequest wrapper for the GetVolumeBackupPolicyAssetAssignment operation
type GetVolumeBackupPolicyAssetAssignmentRequest struct {

	// The OCID of an asset (e.g. a volume).
	AssetId *string `mandatory:"true" contributesTo:"query" name:"assetId"`

	// The maximum number of items to return in a paginated "List" call.
	// Example: `500`
	Limit *int `mandatory:"false" contributesTo:"query" name:"limit"`

	// The value of the `opc-next-page` response header from the previous "List" call.
	Page *string `mandatory:"false" contributesTo:"query" name:"page"`
}

func (request GetVolumeBackupPolicyAssetAssignmentRequest) String() string {
	return common.PointerString(request)
}

// GetVolumeBackupPolicyAssetAssignmentResponse wrapper for the GetVolumeBackupPolicyAssetAssignment operation
type GetVolumeBackupPolicyAssetAssignmentResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// The []VolumeBackupPolicyAssignment instance
	Items []VolumeBackupPolicyAssignment `presentIn:"body"`

	// For pagination of a list of items. When paging through a list, if this header appears in the response,
	// then a partial list might have been returned. Include this value as the `page` parameter for the
	// subsequent GET request to get the next batch of items.
	OpcNextPage *string `presentIn:"header" name:"opc-next-page"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about
	// a particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response GetVolumeBackupPolicyAssetAssignmentResponse) String() string {
	return common.PointerString(response)
}
