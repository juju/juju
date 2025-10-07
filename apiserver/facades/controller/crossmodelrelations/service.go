// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	crossmodelrelationservice "github.com/juju/juju/domain/crossmodelrelation/service"
)

// CrossModelRelationService provides access to cross-model relations.
type CrossModelRelationService interface {
	// GetApplicationUUIDByOfferUUID returns the application UUID for the given offer UUID.
	// Returns crossmodelrelationerrors.OfferNotFound if the offer or associated application is not found.
	GetApplicationUUIDByOfferUUID(ctx context.Context, offerUUID string) (coreapplication.UUID, error)

	// AddRemoteApplicationConsumer adds a new synthetic application representing
	// a remote relation on the consuming model, to this, the offering model.
	AddRemoteApplicationConsumer(ctx context.Context, args crossmodelrelationservice.AddRemoteApplicationConsumerArgs) error

	// GetApplicationRemoteRelationByConsumerRelationUUID retrieves the relation UUID
	// for a remote relation given the consumer relation UUID.
	GetApplicationRemoteRelationByConsumerRelationUUID(
		ctx context.Context,
		consumerRelationUUID string,
	) (corerelation.UUID, error)
}
