// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"gopkg.in/juju/charm.v6-unstable"
)

// ApplicationOfferDetails represents a remote application used when vendor
// lists their own applications.
type ApplicationOfferDetails struct {
	// OfferName is the name of the offer
	OfferName string

	// ApplicationName is the application name to which the offer pertains.
	ApplicationName string

	// OfferURL is the URL where the offer can be located.
	OfferURL string

	// CharmName is a name of a charm for remote application.
	CharmName string

	// Endpoints are the charm endpoints supported by the application.
	// TODO(wallyworld) - do not use charm.Relation here
	Endpoints []charm.Relation

	// ConnectedCount are the number of users that are consuming the application.
	ConnectedCount int
}

// ApplicationOfferDetailsResult is a result of listing a remote application.
type ApplicationOfferDetailsResult struct {
	// Result contains remote application information.
	Result *ApplicationOfferDetails

	// Error contains error related to this item.
	Error error
}
