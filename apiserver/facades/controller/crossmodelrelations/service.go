// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"

	corerelation "github.com/juju/juju/core/relation"
	crossmodelrelationservice "github.com/juju/juju/domain/crossmodelrelation/service"
)

// CrossModelRelationService provides access to cross-model relations.
type CrossModelRelationService interface {
	// CheckOfferByUUID checks if an offer with the given UUID exists.
	// Returns crossmodelrelationerrors.OfferNotFound if the offer is not found.
	CheckOfferByUUID(ctx context.Context, offerUUID string) error

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
