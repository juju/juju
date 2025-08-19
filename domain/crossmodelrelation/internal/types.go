// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"maps"
	"slices"

	"github.com/juju/juju/domain/crossmodelrelation"
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
func MakeCreateOfferArgs(in crossmodelrelation.ApplicationOfferArgs, offerUUID uuid.UUID) CreateOfferArgs {
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

// OfferFilter is used to query applications offered
// by this model.
type OfferFilter struct {
	// OfferUUIDs is a list of offerUUIDs to find based on the
	// crossmodelrelation.OfferFilter.AllowedConsumers.
	OfferUUIDs []string

	// OfferName is the name of the offer.
	OfferName string

	// ApplicationName is the name of the application to which the offer pertains.
	ApplicationName string

	// ApplicationDescription is a description of the application's functionality,
	// typically copied from the charm metadata.
	ApplicationDescription string

	// Endpoint contains an endpoint filter criteria.
	Endpoints []crossmodelrelation.EndpointFilterTerm
}

// Empty checks to see if the filter has any values. An empty
// filter indicates all offers should be found.
func (f OfferFilter) Empty() bool {
	return f.OfferName == "" &&
		f.ApplicationName == "" &&
		f.ApplicationDescription == "" &&
		len(f.Endpoints) == 0 &&
		len(f.OfferUUIDs) == 0
}

// EmptyModuloEndpoints does the same as Empty, but not including endpoints.
func (f OfferFilter) EmptyModuloEndpoints() bool {
	return f.OfferName == "" &&
		f.ApplicationName == "" &&
		f.ApplicationDescription == ""
}
