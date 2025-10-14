// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/offer"
	"github.com/juju/juju/domain/application/charm"
)

// AddRemoteApplicationOffererArgs contains the parameters required to add a new
// remote application offerer.
type AddRemoteApplicationOffererArgs struct {
	// OfferUUID is the UUID of the offer that the remote application is
	// consuming.
	OfferUUID offer.UUID

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

// AddRemoteApplicationConsumerArgs contains the parameters required to add a
// new remote application consumer.
type AddRemoteApplicationConsumerArgs struct {
	// RemoteApplicationUUID is the application UUID as as it exists in the
	// remote (consuming) model. It contains the value from the RPC param
	// ApplicationToken.
	RemoteApplicationUUID string

	// OfferUUID is the UUID of the offer that the remote application is
	// consuming.
	OfferUUID offer.UUID

	// RelationUUID is the UUID of the relation created to connect the remote
	// application to a local application, on the consuming model.
	RelationUUID string

	// ConsumerModelUUID is the UUID of the model that is consuming the
	// application.
	ConsumerModelUUID string

	// Endpoints is the collection of endpoint relations offered.
	Endpoints []charm.Relation
}
