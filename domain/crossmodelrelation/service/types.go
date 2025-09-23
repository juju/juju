// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/domain/application/charm"
)

// AddRemoteApplicationOffererArgs contains the parameters required to add a new
// remote application offerer.
type AddRemoteApplicationOffererArgs struct {
	// OfferUUID is the UUID of the offer that the remote application is
	// consuming.
	OfferUUID string

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
