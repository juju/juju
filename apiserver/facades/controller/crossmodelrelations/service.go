// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	crossmodelrelationservice "github.com/juju/juju/domain/crossmodelrelation/service"
	"github.com/juju/juju/domain/relation"
)

// CrossModelRelationService provides access to cross-model relations.
type CrossModelRelationService interface {
	// GetApplicationNameAndUUIDByOfferUUID returns the application name and UUID
	// for the given offer UUID.
	// Returns crossmodelrelationerrors.OfferNotFound if the offer or associated
	// application is not found.
	GetApplicationNameAndUUIDByOfferUUID(ctx context.Context, offerUUID string) (string, coreapplication.UUID, error)

	// AddRemoteApplicationConsumer adds a new synthetic application representing
	// a remote relation on the consuming model, to this, the offering model.
	AddRemoteApplicationConsumer(ctx context.Context, args crossmodelrelationservice.AddRemoteApplicationConsumerArgs) error
}

// RelationService defines the methods that the facade assumes from the
// Relation service.
type RelationService interface {
	// GetRelationDetails returns the relation details requested by the uniter
	// for a relation.
	GetRelationDetails(ctx context.Context, relationUUID corerelation.UUID) (relation.RelationDetails, error)
}
