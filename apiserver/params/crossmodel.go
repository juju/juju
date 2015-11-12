// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"gopkg.in/juju/charm.v6-unstable"
)

// TODO (wallyworld) - use the ServiceOffer struct below
// CrossModelOffer holds information about service's offer.
type CrossModelOffer struct {
	// Service has service's tag.
	Service string `json:"service"`

	// Endpoints list of service's endpoints that are being offered.
	Endpoints []string `json:"endpoints"`

	// URL is the location where these endpoitns will be accessible from.
	URL string `json:"url"`

	// Users is the list of user tags that are given permission to these endpoints.
	Users []string `json:"users"`
}

// CrossModelOffers holds cross model relations offers..
type CrossModelOffers struct {
	Offers []CrossModelOffer `json:"offers"`
}

// EndpointFilterAttributes is used to filter offers matching the
// specified endpoint criteria.
type EndpointFilterAttributes struct {
	Role      charm.RelationRole `json:"role"`
	Interface string             `json:"interface"`
}

// OfferFilters is used to query offers in a service directory.
// Offers matching any of the filters are returned.
type OfferFilters struct {
	Filters []OfferFilter
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
