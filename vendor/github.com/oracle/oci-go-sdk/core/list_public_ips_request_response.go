// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package core

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// ListPublicIpsRequest wrapper for the ListPublicIps operation
type ListPublicIpsRequest struct {

	// Whether the public IP is regional or specific to a particular Availability Domain.
	// * `REGION`: The public IP exists within a region and can be assigned to a private IP
	// in any Availability Domain in the region. Reserved public IPs have `scope` = `REGION`.
	// * `AVAILABILITY_DOMAIN`: The public IP exists within the Availability Domain of the private IP
	// it's assigned to, which is specified by the `availabilityDomain` property of the public IP object.
	// Ephemeral public IPs have `scope` = `AVAILABILITY_DOMAIN`.
	Scope ListPublicIpsScopeEnum `mandatory:"true" contributesTo:"query" name:"scope" omitEmpty:"true"`

	// The OCID of the compartment.
	CompartmentId *string `mandatory:"true" contributesTo:"query" name:"compartmentId"`

	// The maximum number of items to return in a paginated "List" call.
	// Example: `500`
	Limit *int `mandatory:"false" contributesTo:"query" name:"limit"`

	// The value of the `opc-next-page` response header from the previous "List" call.
	Page *string `mandatory:"false" contributesTo:"query" name:"page"`

	// The name of the Availability Domain.
	// Example: `Uocm:PHX-AD-1`
	AvailabilityDomain *string `mandatory:"false" contributesTo:"query" name:"availabilityDomain"`
}

func (request ListPublicIpsRequest) String() string {
	return common.PointerString(request)
}

// ListPublicIpsResponse wrapper for the ListPublicIps operation
type ListPublicIpsResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// The []PublicIp instance
	Items []PublicIp `presentIn:"body"`

	// For pagination of a list of items. When paging through a list, if this header appears in the response,
	// then a partial list might have been returned. Include this value as the `page` parameter for the
	// subsequent GET request to get the next batch of items.
	OpcNextPage *string `presentIn:"header" name:"opc-next-page"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about
	// a particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response ListPublicIpsResponse) String() string {
	return common.PointerString(response)
}

// ListPublicIpsScopeEnum Enum with underlying type: string
type ListPublicIpsScopeEnum string

// Set of constants representing the allowable values for ListPublicIpsScope
const (
	ListPublicIpsScopeRegion             ListPublicIpsScopeEnum = "REGION"
	ListPublicIpsScopeAvailabilityDomain ListPublicIpsScopeEnum = "AVAILABILITY_DOMAIN"
)

var mappingListPublicIpsScope = map[string]ListPublicIpsScopeEnum{
	"REGION":              ListPublicIpsScopeRegion,
	"AVAILABILITY_DOMAIN": ListPublicIpsScopeAvailabilityDomain,
}

// GetListPublicIpsScopeEnumValues Enumerates the set of values for ListPublicIpsScope
func GetListPublicIpsScopeEnumValues() []ListPublicIpsScopeEnum {
	values := make([]ListPublicIpsScopeEnum, 0)
	for _, v := range mappingListPublicIpsScope {
		values = append(values, v)
	}
	return values
}
