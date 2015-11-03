// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

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

// CrossModelOfferResult holds service tag that has been offered
// and maybe an error that occured while its offer was prepared.
type CrossModelOfferResult struct {
	// Service has service's tag.
	Service string `json:"service"`
	Error   *Error `json:"error,omitempty"`
}

// CrossModelOfferResults holds results for bulk service offers.
type CrossModelOfferResults struct {
	Results []CrossModelOfferResult `json:"results,omitempty"`
}
