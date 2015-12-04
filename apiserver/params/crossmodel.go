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

// ServiceOfferParams is used to offer remote service.
type ServiceOfferParams struct {
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

// ServiceOffersParams contains a collection of offers to allow adding offers in bulk.
type ServiceOffersParams struct {
	Offers []ServiceOfferParams `json:"offers"`
}

// ServiceOfferResult is a result of listing a remote service offer.
type ServiceOfferResult struct {
	// Result contains service offer information.
	Result ServiceOffer `json:"result"`

	// Error contains related error.
	Error *Error `json:"error,omitempty"`
}

// ServiceOffersResults is a result of listing remote service offers.
type ServiceOffersResults struct {
	// Result contains collection of remote service results.
	Results []ServiceOfferResult `json:"results,omitempty"`
}

// ServiceURLs is a filter used to select remote services via show call.
type ServiceURLs struct {
	// URLs contains collection of urls for services that are to be shown.
	ServiceUrls []string `json:"serviceurls,omitempty"`
}

// OfferedService represents attributes for an offered service.
type OfferedService struct {
	ServiceURL  string            `json:"serviceurl"`
	ServiceName string            `json:"servicename"`
	CharmName   string            `json:"charmname"`
	Description string            `json:"description"`
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

// OfferedServiceDetails is a service found during a request to list remote services.
type OfferedServiceDetails struct {
	// ServiceURL may contain user supplied service url.
	ServiceURL string `json:"serviceurl,omitempty"`

	// ServiceName contains name of service being offered.
	ServiceName string `json:"servicename"`

	// CharmName is the charm name of this service.
	CharmName string `json:"charmname"`

	// UsersCount is the count of how many users are connected to this shared service.
	UsersCount int `json:"userscount,omitempty"`

	// Endpoints is a list of charm relations that this remote service offered.
	Endpoints []RemoteEndpoint `json:"endpoints"`
}

// OfferedServiceDetailsResult is a result of listing a remote service.
type OfferedServiceDetailsResult struct {
	// Result contains remote service information.
	Result *OfferedServiceDetails `json:"result,omitempty"`

	// Error contains error related to this item.
	Error *Error `json:"error,omitempty"`
}

// ListOffersFilterResults is a result of listing remote service offers
// for a service directory.
type ListOffersFilterResults struct {
	// Error contains error related to this directory.
	Error *Error `json:"error,omitempty"`

	// Result contains collection of remote service item results for this directory.
	Result []OfferedServiceDetailsResult `json:"result,omitempty"`
}

// ListOffersResults is a result of listing remote service offers
// for service directories.
type ListOffersResults struct {
	// Results contains collection of remote directories results.
	Results []ListOffersFilterResults `json:"results,omitempty"`
}

// OfferedServiceFilters has sets of filters that
// are used by a vendor to query remote services that the vendor has offered.
type OfferedServiceFilters struct {
	Filters []OfferedServiceFilter `json:"filters,omitempty"`
}

// OfferedServiceFilter has a set of filter terms that
// are used by a vendor to query remote services that the vendor has offered.
type OfferedServiceFilter struct {
	FilterTerms []ListOffersFilterTerm `json:"filterterms,omitempty"`
}

// ListOffersFilterTerm has filter criteria that
// are used by a vendor to query remote services that the vendor has offered.
type ListOffersFilterTerm struct {
	// ServiceURL is url for remote service.
	// This may be a part of valid URL.
	ServiceURL string `json:"serviceurl,omitempty"`

	// Endpoint contains endpoint properties for filter.
	Endpoint RemoteEndpoint `json:"endpoint"`

	// CharmName is the charm name of this service.
	CharmName string `json:"charmname,omitempty"`
}
