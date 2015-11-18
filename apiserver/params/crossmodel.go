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
	Result ServiceOffer `json:"result,omitempty"`

	// Error contains related error.
	Error *Error `json:"error,omitempty"`
}

// RemoteServiceResults is a result of listing remote service offers.
type RemoteServiceResults struct {
	Results []RemoteServiceResult `json:"results,omitempty"`
}

type ShowFilter struct {
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
	Result OfferedService `json:"result,omitempty"`
	Error  *Error         `json:"error,omitempty"`
}

// OfferedServiceResults represents the result of a ListOfferedServices call.
type OfferedServiceResults struct {
	Results []OfferedServiceResult
}

// OfferedServiceQueryParams is used to specify the URLs
// for which we want to load offered service details.
type OfferedServiceQueryParams struct {
	URLS []string
}
