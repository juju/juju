// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/internal/uuid"
)

// ModelDBState describes retrieval and persistence methods for cross model
// relations in the model database.
type ModelDBState interface {
	ModelDBOfferState
	ModelDBRemoteApplicationState
}

// ControllerDBState describes retrieval and persistence methods for cross
// model relation access in the controller database.
type ControllerDBState interface {
	// CreateOfferAccess give the offer owner AdminAccess and EveryoneUserName
	// ReadAccess for the provided offer.
	CreateOfferAccess(
		ctx context.Context,
		permissionUUID, offerUUID, ownerUUID uuid.UUID,
	) error

	// GetUsersForOfferUUIDs returns a map of offerUUIDs with a slice of users
	// whom are allowed to consume the offer. Only offers UUIDs provided are
	// returned.
	GetUsersForOfferUUIDs(context.Context, []string) (map[string][]crossmodelrelation.OfferUser, error)

	// GetOfferUUIDsForUsersWithConsume returns offer uuids for any of the given users
	// whom has consumer access or greater.
	GetOfferUUIDsForUsersWithConsume(
		ctx context.Context,
		userNames []string,
	) ([]string, error)

	// GetUserUUIDByName returns the UUID of the user provided exists, has not
	// been removed and is not disabled.
	GetUserUUIDByName(ctx context.Context, name user.Name) (uuid.UUID, error)
}

// WatcherFactory instances return watchers for a given namespace and UUID.
type WatcherFactory interface {
	// NewNotifyWatcher returns a new watcher that filters changes from the input
	// base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		ctx context.Context,
		summary string,
		filterOption eventsource.FilterOption,
		filterOptions ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// Service provides the API for working with cross model relations.
type Service struct {
	controllerState ControllerDBState
	modelState      ModelDBState
	logger          logger.Logger
}

// NewService returns a new service reference wrapping the input state.
func NewService(
	controllerState ControllerDBState,
	modelState ModelDBState,
	logger logger.Logger,
) *Service {
	return &Service{
		controllerState: controllerState,
		modelState:      modelState,
		logger:          logger,
	}
}

// WatchableService is a service that can be watched for changes.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new watchable service instance.
func NewWatchableService(
	controllerState ControllerDBState,
	modelState ModelDBState,
	watcherFactory WatcherFactory,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		Service: Service{
			controllerState: controllerState,
			modelState:      modelState,
			logger:          logger,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchRemoteApplicationConsumers watches the changes to remote
// application consumers and notifies the worker of any changes.
func (w *WatchableService) WatchRemoteApplicationConsumers(ctx context.Context) (watcher.NotifyWatcher, error) {
	return nil, errors.NotImplemented
}

// WatchRemoteApplicationOfferers watches the changes to remote
// application offerers and notifies the worker of any changes.
func (w *WatchableService) WatchRemoteApplicationOfferers(ctx context.Context) (watcher.NotifyWatcher, error) {
	return nil, errors.NotImplemented
}
