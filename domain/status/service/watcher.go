// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/offer"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/internal/errors"
)

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNotifyMapperWatcher returns a new watcher that receives changes from the
	// input base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided. Filtering of values is done first
	// by the filter, and then subsequently by the mapper. Based on the mapper's
	// logic a subset of them (or none) may be emitted.
	NewNotifyMapperWatcher(
		ctx context.Context,
		summary string,
		mapper eventsource.Mapper,
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// WatchableService is a status service that can be used to watch status changes.
type WatchableService struct {
	*Service
	watcherFactory WatcherFactory
}

// NewWatchableService creates a new WatchableService.
func NewWatchableService(
	modelState ModelState,
	controllerState ControllerState,
	watcherFactory WatcherFactory,
	statusHistory StatusHistory,
	statusHistoryReaderFn StatusHistoryReaderFunc,
	clock clock.Clock,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		Service: NewService(
			modelState,
			controllerState,
			statusHistory,
			statusHistoryReaderFn,
			clock,
			logger,
		),
		watcherFactory: watcherFactory,
	}
}

// WatchOfferStatus watches the changes to the derived display status of
// the specified application.
//
// the following errors may be returned:
//   - [crossmodelrelationerrors.OfferNotFound] if the offer doesn't exist.
func (s *WatchableService) WatchOfferStatus(ctx context.Context, offerUUID offer.UUID) (watcher.NotifyWatcher, error) {
	uuid, err := s.modelState.GetApplicationUUIDForOffer(ctx, offerUUID.String())
	if err != nil {
		return nil, errors.Capture(err)
	}
	appUUID, err := coreapplication.ParseUUID(uuid)
	if err != nil {
		return nil, errors.Errorf("parsing application UUID: %w", err)
	}

	appStatusCache, err := s.getApplicationDisplayStatus(ctx, appUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	offerNamespace, applicationNamespace, unitAgentNamespace, unitWorkloadNamespace, unitPodNamespace :=
		s.modelState.NamespacesForWatchOfferStatus()

	var mapper eventsource.Mapper = func(ctx context.Context, events []changestream.ChangeEvent) ([]string, error) {
		mappedEvents := transform.Slice(events, func(c changestream.ChangeEvent) string {
			return c.Changed()
		})

		// If any of our events are from the offer namespace, this means the
		// offer has been deleted. In this case, we should always emit the
		// events.
		for _, event := range events {
			if event.Namespace() == offerNamespace {
				return mappedEvents, nil
			}
		}

		// Otherwise, only emit the events if they have lead to a change in the
		// application's status.
		currentStatus, err := s.getApplicationDisplayStatus(ctx, appUUID)
		if err != nil {
			return nil, errors.Errorf("getting application status: %w", err)
		}
		if reflect.DeepEqual(currentStatus, appStatusCache) {
			return nil, nil
		}
		appStatusCache = currentStatus
		return mappedEvents, nil
	}

	var unitForApplicationPredicate eventsource.Predicate = func(changed string) bool {
		ret, err := s.modelState.IsUnitForApplication(ctx, changed, appUUID.String())
		if err != nil {
			s.logger.Warningf(ctx, "checking if unit %q is a unit of application %q: %v", changed, appUUID, err)
			return false
		}
		return ret
	}

	return s.watcherFactory.NewNotifyMapperWatcher(
		ctx,
		fmt.Sprintf("application status watcher for %q", appUUID),
		mapper,
		eventsource.PredicateFilter(
			offerNamespace,
			changestream.Deleted,
			eventsource.EqualsPredicate(offerUUID.String()),
		),
		eventsource.PredicateFilter(
			applicationNamespace,
			changestream.All,
			eventsource.EqualsPredicate(appUUID.String()),
		),
		eventsource.PredicateFilter(
			unitAgentNamespace,
			changestream.All,
			unitForApplicationPredicate,
		),
		eventsource.PredicateFilter(
			unitWorkloadNamespace,
			changestream.All,
			unitForApplicationPredicate,
		),
		eventsource.PredicateFilter(
			unitPodNamespace,
			changestream.All,
			unitForApplicationPredicate,
		),
	)
}
