// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/offer"
	corerelation "github.com/juju/juju/core/relation"
	coresecrets "github.com/juju/juju/core/secrets"
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

	// GetOfferUUIDByRelationUUID returns the offer UUID corresponding to
	// the cross model relation UUID.
	GetOfferUUIDByRelationUUID(ctx context.Context, relationUUID corerelation.UUID) (offer.UUID, error)

	// WatchRemoteConsumedSecretsChanges watches secrets remotely consumed by any
	// unit of the specified app and returns a watcher which notifies of secret URIs
	// that have had a new revision added.
	WatchRemoteConsumedSecretsChanges(ctx context.Context, appUUID application.UUID) (watcher.StringsWatcher, error)
}

// SecretService provides access to secrets.
type SecretService interface {
	// GetLatestRevisions returns the latest secret revisions for the specified URIs.
	GetLatestRevisions(ctx context.Context, uris []*coresecrets.URI) (map[string]int, error)
}

// StatusService provides access to the status service.
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
