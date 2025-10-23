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
	corerelation "github.com/juju/juju/core/relation"
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
	ModelRelationNetworkState
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

// WatchOffererRelations watches the changes to (remote) relations on the
// offering model and notifies the worker of any changes.
// This watcher watches both the relation and application_remote_consumer
// namespaces and maintains an internal cache to filter relation changes without
// hitting the database for each change event.
func (w *WatchableService) WatchOffererRelations(ctx context.Context) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	relationTable, initialQuery := w.modelState.InitialWatchStatementForOffererRelations()
	applicationRemoteConsumer := w.modelState.NamespaceRemoteApplicationConsumers()

	// Create a stateful mapper that maintains a cache of remote relation UUIDs.
	// The cache is rebuilt when consumer deletions occur since we cannot query
	// deleted consumers to determine their relation UUIDs.
	cache := make(map[string]bool)
	initialized := false

	mapper := func(ctx context.Context, changes []changestream.ChangeEvent) ([]string, error) {
		if len(changes) == 0 {
			return nil, nil
		}

		// On first call, initialize the cache from the initial query results.
		// The initial query returns all relation UUIDs that are remote.
		if !initialized {
			for _, change := range changes {
				if change.Namespace() == relationTable {
					cache[change.Changed()] = true
				}
			}
			initialized = true

			// Return all initial remote relation UUIDs.
			result := make([]string, 0, len(cache))
			for uuid := range cache {
				result = append(result, uuid)
			}
			return result, nil
		}

		// Separate changes by namespace.
		var consumerDeleted bool
		var consumerCreatedOrUpdated []string
		var relationChanges []string

		for _, change := range changes {
			switch change.Namespace() {
			case applicationRemoteConsumer:
				// Track consumer changes to update cache.
				if change.Type() == changestream.Deleted {
					// Consumer was deleted - we need to rebuild the cache since we
					// can't query the deleted consumer to find its relation.
					consumerDeleted = true
				} else {
					// Consumer was created or updated.
					consumerCreatedOrUpdated = append(consumerCreatedOrUpdated, change.Changed())
				}
			case relationTable:
				// Track relation changes to filter against cache.
				relationChanges = append(relationChanges, change.Changed())
			}
		}

		// Rebuild cache if any consumer was deleted, since we can't determine
		// which relation belonged to the deleted consumer without querying it
		// (which we can't do after deletion).
		if consumerDeleted {
			// Re-query all current offerer relations to rebuild the cache.
			relationUUIDs, err := w.modelState.GetAllOffererRelationUUIDs(ctx)
			if err != nil {
				w.logger.Errorf(ctx, "rebuilding cache after consumer deletion: %v", err)
				// Continue with old cache on error.
			} else {
				// Rebuild cache from scratch.
				cache = make(map[string]bool)
				for _, uuid := range relationUUIDs {
					cache[uuid] = true
				}
			}
		}

		// Update cache with new remote relation UUIDs from created/updated consumers.
		if len(consumerCreatedOrUpdated) > 0 {
			newRelationUUIDs, err := w.modelState.GetOffererRelationUUIDsForConsumers(ctx, consumerCreatedOrUpdated...)
			if err != nil {
				w.logger.Errorf(ctx, "getting relation UUIDs for consumers: %v", err)
				// Continue processing relation changes even if cache update fails.
			} else {
				for _, uuid := range newRelationUUIDs {
					cache[uuid] = true
				}
			}
		}

		// Filter relation changes based on cache.
		var result []string
		for _, uuid := range relationChanges {
			if cache[uuid] {
				result = append(result, uuid)
			}
		}

		return result, nil
	}

	return w.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		initialQuery,
		"offerer relations watcher",
		mapper,
		eventsource.NamespaceFilter(relationTable, changestream.All),
		eventsource.NamespaceFilter(applicationRemoteConsumer, changestream.All),
	)
}

// WatchRelationEgressNetworks watches for changes to the egress networks
// for the specified relation UUID. It returns a NotifyWatcher that emits
// events when there are insertions or deletions in the relation_network_egress
// table.
func (c *Service) WatchRelationEgressNetworks(ctx context.Context, relationUUID corerelation.UUID) (watcher.NotifyWatcher, error) {
	return nil, errors.Errorf("crossmodelrelation.WatchRelationEgressNetworks").Add(coreerrors.NotImplemented)
}

// WatchRelationIngressNetworks watches for changes to the ingress networks
// for the specified relation UUID. It returns a NotifyWatcher that emits
// events when there are insertions or deletions in the relation_network_ingress
// table.
func (w *WatchableService) WatchRelationIngressNetworks(ctx context.Context, relationUUID corerelation.UUID) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := relationUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	table := w.modelState.NamespaceForRelationIngressNetworksWatcher()

	return w.watcherFactory.NewNotifyWatcher(
		ctx,
		"relation ingress networks watcher",
		eventsource.PredicateFilter(table, changestream.All, eventsource.EqualsPredicate(relationUUID.String())),
	)
}

func ptr[T any](v T) *T {
	return &v
}
