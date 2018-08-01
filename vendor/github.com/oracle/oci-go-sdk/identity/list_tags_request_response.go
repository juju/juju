// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package identity

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// ListTagsRequest wrapper for the ListTags operation
type ListTagsRequest struct {

	// The OCID of the tag namespace.
	TagNamespaceId *string `mandatory:"true" contributesTo:"path" name:"tagNamespaceId"`

	// The value of the `opc-next-page` response header from the previous "List" call.
	Page *string `mandatory:"false" contributesTo:"query" name:"page"`

	// The maximum number of items to return in a paginated "List" call.
	Limit *int `mandatory:"false" contributesTo:"query" name:"limit"`
}

func (request ListTagsRequest) String() string {
	return common.PointerString(request)
}

// ListTagsResponse wrapper for the ListTags operation
type ListTagsResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// The []TagSummary instance
	Items []TagSummary `presentIn:"body"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about a
	// particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`

	// For pagination of a list of tags. When paging through a list, if this header appears in the response,
	// then a partial list might have been returned. Include this value as the `page` parameter for the
	// subsequent GET request to get the next batch of items.
	OpcNextPage *string `presentIn:"header" name:"opc-next-page"`
}

func (response ListTagsResponse) String() string {
	return common.PointerString(response)
}
