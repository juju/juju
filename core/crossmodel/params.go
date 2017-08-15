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

	// CharmURL is the URL of the charm for the remote application.
	CharmURL string

	// Endpoints are the charm endpoints supported by the application.
	// TODO(wallyworld) - do not use charm.Relation here
	Endpoints []charm.Relation

	// Connects are the connections to the offer.
	Connections []OfferConnection
}

// OfferConnection holds details about a connection to an offer.
type OfferConnection struct {
	// SourceModelUUID is the UUID of the model hosting the offer.
	SourceModelUUID string

	// Username is the name of the user consuming the offer.
	Username string

	// RelationId is the id of the relation for this connection.
	RelationId int

	// Endpoint is the endpoint being connected to.
	Endpoint string

	// Status is the status of the offer connection.
	Status string
}

// ApplicationOfferDetailsResult is a result of listing a remote application.
type ApplicationOfferDetailsResult struct {
	// Result contains remote application information.
	Result *ApplicationOfferDetails

	// Error contains error related to this item.
	Error error
}
