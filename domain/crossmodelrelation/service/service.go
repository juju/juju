// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/clock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/offer"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/secret"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/statushistory"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
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
	ModelSecretsState

	// InitialWatchStatementForConsumerRelations returns the namespace and the
	// initial query function for watching relation UUIDs that are associated with
	// remote offerer applications present in this model (i.e. consumer side).
	InitialWatchStatementForConsumerRelations() (string, eventsource.NamespaceQuery)

	// GetConsumerRelationUUIDs filters the provided relation UUIDs and returns
	// only those that are associated with remote offerer applications in this model.
	GetConsumerRelationUUIDs(ctx context.Context, relationUUIDs ...string) ([]string, error)
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
	// NewNamespaceWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. Change-log events will be emitted only if
	// the filter accepts them, and dispatching the notifications via the
	// Changes channel. A filter option is required, though additional filter
	// options can be provided.
	NewNamespaceWatcher(
		ctx context.Context,
		initialQuery eventsource.NamespaceQuery,
		summary string,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)
	// NewNamespaceMapperWatcher returns a new watcher that receives changes from
	// the input base watcher's db/queue. Filtering of values is done first by
	// the filter, and then by the mapper. Based on the mapper's logic a subset
	// of them (or none) may be emitted. A filter option is required, though
	// additional filter options can be provided.
	NewNamespaceMapperWatcher(
		ctx context.Context,
		initialQuery eventsource.NamespaceQuery,
		summary string,
		mapper eventsource.Mapper,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)
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

// WatchRemoteConsumedSecretsChanges watches secrets remotely consumed by any
// unit of the specified app and returns a watcher which notifies of secret URIs
// that have had a new revision added.
// Run on the offering model.
func (s *WatchableService) WatchRemoteConsumedSecretsChanges(ctx context.Context, appUUID coreapplication.UUID) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appUUID.Validate(); err != nil {
		return nil, errors.Errorf("validating application UUID: %w", err)
	}

	table, query := s.modelState.InitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide(appUUID.String())
	w, err := s.watcherFactory.NewNamespaceWatcher(
		ctx,
		query,
		fmt.Sprintf("remote consumed secrets watcher for application %q", appUUID),
		eventsource.NamespaceFilter(table, changestream.All),
	)
	if err != nil {
		return nil, errors.Capture(err)
	}
	processChanges := func(ctx context.Context, secretIDs ...string) ([]string, error) {
		return s.modelState.GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(ctx, appUUID.String(), secretIDs...)
	}
	return secret.NewSecretStringWatcher(w, s.logger, processChanges)
}

// RemoteApplications returns the current state for the named remote applications.
// Not implemented yet in the domain service.
func (w *WatchableService) RemoteApplications(ctx context.Context, applications []string) ([]params.RemoteApplicationResult, error) {
	return nil, errors.Errorf("crossmodelrelation.RemoteApplications").Add(coreerrors.NotImplemented)
}

// WatchRemoteRelations returns a disabled watcher for remote relations for now.
// Not implemented yet in the domain service.
func (w *WatchableService) WatchRemoteRelations(ctx context.Context) (watcher.StringsWatcher, error) {
	return nil, errors.Errorf("crossmodelrelation.WatchRemoteRelations").Add(coreerrors.NotImplemented)
}

// WatchConsumerRelations watches the changes to (remote) relations on the
// consuming model and notifies the worker of any changes.
// NOTE(nvinuesa): This watcher is less efficient than WatchOffererRelations,
// because it has to watch for all changes in the relation table and then filter
// with a db query on the application_remote_offerer table. We currently cannot
// watch both namespaces in a single watcher because the inserts in each
// respective tables do not happen in a single transaction.
func (w *WatchableService) WatchConsumerRelations(ctx context.Context) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	table, initialQuery := w.modelState.InitialWatchStatementForConsumerRelations()

	// Filter change events using the state layer to determine which relation
	// UUIDs are associated with remote offerer applications.
	mapper := func(ctx context.Context, changes []changestream.ChangeEvent) ([]string, error) {
		if len(changes) == 0 {
			return nil, nil
		}

		ids := make([]string, len(changes))
		for i, change := range changes {
			ids[i] = change.Changed()
		}

		filtered, err := w.modelState.GetConsumerRelationUUIDs(ctx, ids...)
		if err != nil {
			w.logger.Errorf(ctx, "filtering consumer relations: %v", err)
			return nil, nil
		}
		return filtered, nil
	}

	return w.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		initialQuery,
		"consumer relations watcher",
		mapper,
		eventsource.NamespaceFilter(table, changestream.All),
	)
}

func ptr[T any](v T) *T {
	return &v
}
