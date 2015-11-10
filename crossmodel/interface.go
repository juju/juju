// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
)

// Offer holds information about service's offer.
type Offer struct {
	// Service has service's tag.
	Service names.ServiceTag

	// Endpoints list of service's endpoints that are being offered.
	Endpoints []string

	// URL is the location where these endpoitns will be accessible from.
	URL string

	// Users is the list of user tags that are given permission to these endpoints.
	Users []names.UserTag
}

// PublicUser is the default user used to indicate
// public access to a shared service.
var PublicUser = names.NewUserTag("public")

// A ServiceDirectoryProvider holds service offerings from external environments.
type ServiceDirectoryProvider interface {

	// AddOffer adds a new service offering to the directory, able to be consumed by
	// the specified users.
	AddOffer(url string, offerDetails params.ServiceOfferDetails, users []names.UserTag) error

	// List offers returns the offers satisfying the specified filter.
	ListOffers(filter ...params.OfferFilter) ([]params.ServiceOffer, error)
}
