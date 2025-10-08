// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/offer"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/internal/statushistory"
	"github.com/juju/juju/internal/uuid"
)

// StatusHistory records status information into a generalized way.
type StatusHistory interface {
	// RecordStatus records the given status information.
	// If the status data cannot be marshalled, it will not be recorded, instead
	// the error will be logged under the data_error key.
	RecordStatus(context.Context, statushistory.Namespace, corestatus.StatusInfo) error
}

// ModelState describes retrieval and persistence methods for cross model
// relations in the model database.
type ModelState interface {
	ModelOfferState
	ModelRemoteApplicationState
}

// ControllerState describes retrieval and persistence methods for cross
// model relation access in the controller database.
type ControllerState interface {
	// CreateOfferAccess give the offer owner AdminAccess and EveryoneUserName
	// ReadAccess for the provided offer.
	CreateOfferAccess(
		ctx context.Context,
		permissionUUID uuid.UUID,
		offerUUID offer.UUID,
		ownerUUID uuid.UUID,
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
	controllerState ControllerState
	modelState      ModelState
	statusHistory   StatusHistory
	clock           clock.Clock
	logger          logger.Logger
}

// NewService returns a new service reference wrapping the input state.
func NewService(
	controllerState ControllerState,
	modelState ModelState,
	statusHistory StatusHistory,
	clock clock.Clock,
	logger logger.Logger,
) *Service {
	return &Service{
		controllerState: controllerState,
		modelState:      modelState,
		statusHistory:   statusHistory,
		clock:           clock,
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
	controllerState ControllerState,
	modelState ModelState,
	watcherFactory WatcherFactory,
	statusHistory StatusHistory,
	clock clock.Clock,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		Service: Service{
			controllerState: controllerState,
			modelState:      modelState,
			statusHistory:   statusHistory,
			clock:           clock,
			logger:          logger,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchRemoteApplicationConsumers watches the changes to remote
// application consumers and notifies the worker of any changes.
func (w *WatchableService) WatchRemoteApplicationConsumers(ctx context.Context) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	table := w.modelState.NamespaceRemoteApplicationConsumers()

	return w.watcherFactory.NewNotifyWatcher(
		ctx,
		"watch remote application consumer",
		eventsource.NamespaceFilter(table, changestream.All),
	)
}

// WatchRemoteApplicationOfferers watches the changes to remote application
// offerer applications and notifies the worker of any changes.
func (w *WatchableService) WatchRemoteApplicationOfferers(ctx context.Context) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	table := w.modelState.NamespaceRemoteApplicationOfferers()

	return w.watcherFactory.NewNotifyWatcher(
		ctx,
		"watch remote application offerer",
		eventsource.NamespaceFilter(table, changestream.All),
	)
}

func ptr[T any](v T) *T {
	return &v
}
