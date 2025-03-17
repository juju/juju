// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/internal/charm"
)

// todo(gfouillet): This file and types may be not at the right place. It is
//    basically types pulled over from state which are used in this package.
//    Most should be moved in the futur CMR domain.

// AddRemoteApplicationParams contains the parameters for adding a remote application
// to the model.
type AddRemoteApplicationParams struct {
	// Name is the name to give the remote application. This does not have to
	// match the application name in the URL, or the name in the remote model.
	Name string

	// OfferUUID is the UUID of the offer.
	OfferUUID string

	// URL is either empty, or the URL that the remote application was offered
	// with on the hosting model.
	URL string

	// ExternalControllerUUID, if set, is the UUID of the controller other
	// than this one, which is hosting the offer.
	ExternalControllerUUID string

	// SourceModel is the tag of the model to which the remote application belongs.
	SourceModel names.ModelTag

	// Token is an opaque string that identifies the remote application in the
	// source model.
	Token string

	// Endpoints describes the endpoints that the remote application implements.
	Endpoints []charm.Relation

	// IsConsumerProxy is true when a remote application is created as a result
	// of a registration operation from a remote model.
	IsConsumerProxy bool

	// ConsumeVersion is incremented each time a new consumer proxy
	// is created for an offer.
	ConsumeVersion int

	// Macaroon is used for authentication on the offering side.
	Macaroon *macaroon.Macaroon
}

// AddOfferConnectionParams contains the parameters for adding an offer connection
// to the model.
type AddOfferConnectionParams struct {
	// SourceModelUUID is the UUID of the consuming model.
	SourceModelUUID string

	// OfferUUID is the UUID of the offer.
	OfferUUID string

	// Username is the name of the user who created this connection.
	Username string

	// RelationId is the id of the relation to which this offer pertains.
	RelationId int

	// RelationKey is the key of the relation to which this offer pertains.
	RelationKey string
}
