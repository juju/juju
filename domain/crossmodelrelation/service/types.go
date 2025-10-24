// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/offer"
	"github.com/juju/juju/domain/application/charm"
)

// AddRemoteApplicationOffererArgs contains the parameters required to add a new
// remote application offerer.
type AddRemoteApplicationOffererArgs struct {
	// OfferUUID is the UUID of the offer that the remote application is
	// consuming.
	OfferUUID offer.UUID

	// OfferURL is the URL of this offer, used to located an offered appliction
	// and it's exported endpoints
	OfferURL crossmodel.OfferURL

	// OffererControllerUUID is the UUID of the controller that the remote
	// application is in.
	OffererControllerUUID *string

	// OffererModelUUID is the UUID of the model that is offering the
	// application.
	OffererModelUUID string

	// Endpoints is the collection of endpoint names offered.
	Endpoints []charm.Relation

	// Macaroon is the macaroon that the remote application uses to
	// authenticate with the offerer model.
	Macaroon *macaroon.Macaroon
}

// AddConsumedRelationArgs contains the parameters required to add a
// new remote application consumer and the new relation.
type AddConsumedRelationArgs struct {
	// ConsumerApplicationUUID is the application UUID as it exists in the
	// remote (consuming) model. It contains the value from the RPC param
	// ConsumerApplicationToken.
	ConsumerApplicationUUID string

	// ConsumerApplicationEndpoint is the consumed endpoint relation in the relation.
	ConsumerApplicationEndpoint charm.Relation

	// OfferUUID is the UUID of the offer that the remote application is
	// consuming.
	OfferUUID offer.UUID

	// RelationUUID is the UUID of the relation created to connect the consuming
	// application to a offering application, on the consuming model.
	RelationUUID string

	// ConsumerModelUUID is the UUID of the model that is consuming the
	// application.
	ConsumerModelUUID string

	// OfferingEndpointName is the name of the endpoint on the offering
	// application to use in the relation.
	OfferingEndpointName string

	// Username is the name of the user making the request.
	Username string
}

// OfferFilter is used to query applications offered
// by this model.
type OfferFilter struct {
	// OfferName is the name of the offer.
	OfferName string

	// ApplicationName is the name of the application to which the offer pertains.
	ApplicationName string

	// ApplicationDescription is a description of the application's functionality,
	// typically copied from the charm metadata.
	ApplicationDescription string

	// Endpoint contains an endpoint filter criteria.
	Endpoints []EndpointFilterTerm

	// AllowedConsumers are the users allowed to consume the offer.
	AllowedConsumers []string

	// ConnectedUsers are the users currently related to the offer.
	ConnectedUsers []string
}

// EndpointFilterTerm represents a remote endpoint filter.
type EndpointFilterTerm struct {
	// Name is an endpoint name.
	Name string

	// Interface is an endpoint interface.
	Interface string

	// Role is an endpoint role.
	Role charm.RelationRole
}
