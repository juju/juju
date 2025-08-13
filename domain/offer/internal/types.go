// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"maps"
	"slices"

	"github.com/juju/juju/domain/offer"
	"github.com/juju/juju/internal/uuid"
)

// CreateOfferArgs contains parameters used to create an offer.
type CreateOfferArgs struct {
	// UUID is the unique identifier of the new offer.
	UUID uuid.UUID

	// ApplicationName is the name of the application to which the offer pertains.
	ApplicationName string

	// Endpoints is the collection of endpoint names offered.
	Endpoints []string

	// OfferName is the name of the offer.
	OfferName string
}

// MakeCreateOfferArgs returns a CreateOfferArgs from the given
// ApplicationOfferArgs and uuid.
func MakeCreateOfferArgs(in offer.ApplicationOfferArgs, offerUUID uuid.UUID) CreateOfferArgs {
	return CreateOfferArgs{
		UUID:            offerUUID,
		ApplicationName: in.ApplicationName,
		// There was an original intention to allow for endpoint aliases,
		// however it was never implemented. Just use the maps keys from
		// here.
		Endpoints: slices.Collect(maps.Keys(in.Endpoints)),
		OfferName: in.OfferName,
	}
}

// UpdateOfferArgs contains parameters used to update an offer.
type UpdateOfferArgs struct {
	// Endpoints is the collection of endpoint names offered.
	// The map allows for advertised endpoint names to be aliased.
	Endpoints map[string]string

	// OfferName is the name of the offer.
	OfferName string
}
