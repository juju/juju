// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// CrossModelOffer holds information about service's offer.
type CrossModelOffer struct {
	// Service has service's tag.
	Service string `json:"service"`

	// Endpoints list of service's endpoints that are being offered.
	Endpoints []string `json:"endpoints"`

	// URL is the location where these endpoints will be accessible from.
	URL string `json:"url"`

	// Users is the list of user tags that are given permission to these endpoints.
	Users []string `json:"users"`
}

// CrossModelOffers holds cross model relations offers..
type CrossModelOffers struct {
	Offers []CrossModelOffer `json:"offers"`
}

// SAASDetailsResult holds information about SAAS endpoints.
type SAASDetailsResult struct {
	// Service has service's tag.
	Service string `json:"service"`

	// Endpoints list of service's endpoints that are being offered.
	Endpoints []string `json:"endpoints"`

	// Desc is the user entered description.
	Description string `json:"description,omitempty"`
}

// SAASSearchFilter holds filter used for show, find and list
// operations for cross model relations.
type SAASSearchFilter struct {
	// URL has Juju location for offered service.
	URL string `json:"url"`
}
