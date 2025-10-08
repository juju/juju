// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/offer"
	crossmodelrelationservice "github.com/juju/juju/domain/crossmodelrelation/service"
)

// CrossModelRelationService provides access to cross-model relations.
type CrossModelRelationService interface {
	// GetApplicationNameAndUUIDByOfferUUID returns the application name and UUID
	// for the given offer UUID.
	// Returns crossmodelrelationerrors.OfferNotFound if the offer or associated
	// application is not found.
	GetApplicationNameAndUUIDByOfferUUID(ctx context.Context, offerUUID offer.UUID) (string, coreapplication.UUID, error)

	// AddRemoteApplicationConsumer adds a new synthetic application representing
	// a remote relation on the consuming model, to this, the offering model.
	AddRemoteApplicationConsumer(ctx context.Context, args crossmodelrelationservice.AddRemoteApplicationConsumerArgs) error
}
