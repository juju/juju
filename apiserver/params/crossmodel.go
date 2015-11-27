// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"gopkg.in/juju/charm.v6-unstable"
)

// EndpointFilterAttributes is used to filter offers matching the
// specified endpoint criteria.
type EndpointFilterAttributes struct {
	Role      charm.RelationRole `json:"role"`
	Interface string             `json:"interface"`
}

// OfferFilters is used to query offers in a service directory.
// Offers matching any of the filters are returned.
type OfferFilters struct {
	Directory string
	Filters   []OfferFilter
}

// OfferFilter is used to query offers in a service directory.
type OfferFilter struct {
	ServiceURL         string                     `json:"serviceurl"`
	SourceLabel        string                     `json:"sourcelabel"`
	SourceEnvUUIDTag   string                     `json:"sourceuuid"`
	ServiceName        string                     `json:"servicename"`
	ServiceDescription string                     `json:"servicedescription"`
	ServiceUser        string                     `json:"serviceuser"`
	Endpoints          []EndpointFilterAttributes `json:"endpoints"`
	AllowedUserTags    []string                   `json:"allowedusers"`
}

// ServiceOffer represents a service offering from an external environment.
type ServiceOffer struct {
	ServiceURL         string           `json:"serviceurl"`
	SourceEnvironTag   string           `json:"sourceenviron"`
	SourceLabel        string           `json:"sourcelabel"`
	ServiceName        string           `json:"servicename"`
	ServiceDescription string           `json:"servicedescription"`
	Endpoints          []RemoteEndpoint `json:"endpoints"`
}

// AddServiceOffers is used when adding offers to a service directory.
type AddServiceOffers struct {
	Offers []AddServiceOffer
}

// AddServiceOffer represents a service offering from an external environment.
type AddServiceOffer struct {
	ServiceOffer
	// UserTags are those who can consume the offer.
	UserTags []string `json:"users"`
}

// ServiceOfferResults is a result of listing service offers.
type ServiceOfferResults struct {
	Offers []ServiceOffer
	Error  *Error
}

// RemoteEndpoint represents a remote service endpoint.
type RemoteEndpoint struct {
	Name      string              `json:"name"`
	Role      charm.RelationRole  `json:"role"`
	Interface string              `json:"interface"`
	Limit     int                 `json:"limit"`
	Scope     charm.RelationScope `json:"scope"`
}

// RemoteServiceOffer is used to offer remote service.
type RemoteServiceOffer struct {
	// ServiceURL may contain user supplied service url.
	ServiceURL string `json:"serviceurl,omitempty"`

	// ServiceName contains name of service being offered.
	ServiceName string `json:"servicename"`

	// Description is description for the offered service.
	// For now, this defaults to description provided in the charm or
	// is supplied by the user.
	ServiceDescription string `json:"servicedescription"`

	// Endpoints contains offered service endpoints.
	Endpoints []string `json:"endpoints"`

	// AllowedUserTags contains tags of users that are allowed to use this offered service.
	AllowedUserTags []string `json:"allowedusers"`
}

// RemoteServiceOffers contains a collection of offers to allow adding offers in bulk.
type RemoteServiceOffers struct {
	Offers []RemoteServiceOffer `json:"offers"`
}

// RemoteServiceResult is a result of listing a remote service offer.
type RemoteServiceResult struct {
	// Result contains service offer information.
	Result ServiceOffer `json:"result"`

	// Error contains related error.
	Error *Error `json:"error,omitempty"`
}

// RemoteServiceResults is a result of listing remote service offers.
type RemoteServiceResults struct {
	// Result contains collection of remote service results.
	Results []RemoteServiceResult `json:"results,omitempty"`
}

// ShowFilter is a filter used to select remote services via show call.
type ShowFilter struct {
	// URLs contains collection of urls for services that are to be shown.
	URLs []string `json:"urls,omitempty"`
}

// OfferedService represents attributes for an offered service.
// TODO(wallyworld) - consolidate this with the CLI when possible.
type OfferedService struct {
	ServiceURL  string            `json:"serviceurl"`
	ServiceName string            `json:"servicename"`
	Registered  bool              `json:"registered"`
	Endpoints   map[string]string `json:"endpoints"`
}

// OfferedServiceResult holds the result of loading an
// offerred service at a URL.
type OfferedServiceResult struct {
	Result OfferedService `json:"result"`
	Error  *Error         `json:"error,omitempty"`
}

// OfferedServiceResults represents the result of a ListOfferedServices call.
type OfferedServiceResults struct {
	Results []OfferedServiceResult
}

// OfferedServiceQueryParams is used to specify the URLs
// for which we want to load offered service details.
type OfferedServiceQueryParams struct {
	ServiceUrls []string
}

// ListEndpointsServiceItem is a service found during a request to list remote services.
type ListEndpointsServiceItem struct {
	// ServiceURL may contain user supplied service url.
	ServiceURL string `json:"serviceurl,omitempty"`

	// ServiceName contains name of service being offered.
	ServiceName string `json:"servicename"`

	// CharmName is the charm name of this service.
	CharmName string `json:"charmname"`

	// UsersCount is the count of how many users are connected to this shared service.
	UsersCount int `json:"userscount,omitempty"`

	// Endpoints is a list of charm relations that this remote service offered.
	Endpoints []charm.Relation `json:"endpoints"`
}

// ListEndpointsServiceItemResult is a result of listing a remote service.
type ListEndpointsServiceItemResult struct {
	// Result contains remote service information.
	Result *ListEndpointsServiceItem `json:"result,omitempty"`

	// Error contains error related to this item.
	Error *Error `json:"error,omitempty"`
}

// ListEndpointsItemsResult is a result of listing remote service offers
// for a service directory.
type ListEndpointsItemsResult struct {
	// Error contains error related to this directory.
	Error *Error `json:"error,omitempty"`

	// Result contains collection of remote service item results for this directory.
	Result []ListEndpointsServiceItemResult `json:"result,omitempty"`
}

// ListEndpointsItemsResults is a result of listing remote service offers
// for service directories.
type ListEndpointsItemsResults struct {
	// Results contains collection of remote directories results.
	Results []ListEndpointsItemsResult `json:"results,omitempty"`
}

// ListEndpointsFilters has sets of filters that
// are used by a vendor to query remote services that the vendor has offered.
type ListEndpointsFilters struct {
	Filters []ListEndpointsFilter `json:"filters,omitempty"`
}

// ListEndpointsFilter has a set of filter terms that
// are used by a vendor to query remote services that the vendor has offered.
type ListEndpointsFilter struct {
	FilterTerms []ListEndpointsFilterTerm `json:"filterterms,omitempty"`
}

// ListEndpointsFilterTerm has filter criteria that
// are used by a vendor to query remote services that the vendor has offered.
type ListEndpointsFilterTerm struct {
	// ServiceURL is url for remote service.
	// This may be a part of valid URL.
	ServiceURL string `json:"serviceurl,omitempty"`

	// Endpoint contains endpoint properties for filter.
	Endpoint RemoteEndpoint `json:"endpoint"`

	// CharmName is the charm name of this service.
	CharmName string `json:"charmname,omitempty"`
}
