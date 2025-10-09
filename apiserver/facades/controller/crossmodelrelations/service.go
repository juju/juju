// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/offer"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	crossmodelrelationservice "github.com/juju/juju/domain/crossmodelrelation/service"
)

// CrossModelRelationService provides access to cross-model relations.
type CrossModelRelationService interface {
	// GetApplicationNameAndUUIDByOfferUUID returns the application name and UUID
	// for the given offer UUID.
	// Returns crossmodelrelationerrors.OfferNotFound if the offer or associated
	// application is not found.
	GetApplicationNameAndUUIDByOfferUUID(ctx context.Context, offerUUID offer.UUID) (string, application.UUID, error)

	// AddRemoteApplicationConsumer adds a new synthetic application representing
	// a remote relation on the consuming model, to this, the offering model.
	AddRemoteApplicationConsumer(ctx context.Context, args crossmodelrelationservice.AddRemoteApplicationConsumerArgs) error
}

type CrossModelRelationsService interface {
	// GetApplicationUUIDForOffer returns the UUID of the application that the
	// specified offer belongs to.
	GetApplicationUUIDForOffer(context.Context, offer.UUID) (application.UUID, error)
}

type StatusService interface {
	// GetOfferStatus returns the status of the specified offer. This status shadows
	// the status of the application that the offer belongs to, except in the case
	// where the application or offer has been removed. Then a Terminated status is
	// returned.
	GetOfferStatus(context.Context, offer.UUID) (status.StatusInfo, error)

	// WatchOfferStatus watches the changes to the derived display status of
	// the specified application.
	WatchOfferStatus(context.Context, offer.UUID) (watcher.NotifyWatcher, error)
}
