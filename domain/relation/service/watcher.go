// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// WatcherFactory describes methods for creating watchers that are used by the
// WatchableService.
type WatcherFactory interface {
	// NewNotifyWatcher returns a new watcher that receives changes from the
	// input base watcher's db/queue.
	NewNotifyWatcher(
		ctx context.Context,
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)

	// NewNamespaceWatcher returns a new watcher that receives changes from the
	// input base watcher's db/queue.
	NewNamespaceMapperWatcher(
		ctx context.Context,
		initialQuery eventsource.NamespaceQuery,
		mapper eventsource.Mapper,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)
}

// WatchableService provides the API for working with applications and the
// ability to create watchers.
type WatchableService struct {
	*LeadershipService
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new watchable service reference wrapping the input state.
func NewWatchableService(
	st State,
	watcherFactory WatcherFactory,
	leaderEnsurer leadership.Ensurer,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		LeadershipService: NewLeadershipService(st, leaderEnsurer, logger),
		watcherFactory:    watcherFactory,
	}
}

// WatchLifeSuspendedStatus returns a watcher that notifies of changes to
// the life or suspended status any relation the unit's application is part
// of. If the unit is a subordinate, its principal application is watched.
func (s *WatchableService) WatchLifeSuspendedStatus(
	ctx context.Context,
	unitUUID unit.UUID,
) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitUUID.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w:%w", relationerrors.UnitUUIDNotValid, err)
	}

	principalID, subordinateID, err := s.st.GetPrincipalSubordinateApplicationIDs(ctx, unitUUID)
	if err != nil {
		return nil, errors.Errorf("finding principal and subordinate application ids: %w", err)
	}

	var w namespaceMapperWatcherMethods
	if subordinateID != "" {
		w = newSubordinateLifeSuspendedStatusWatcher(s, principalID, subordinateID)
	} else {
		w = newPrincipalLifeSuspendedStatusWatcher(s, principalID)
	}
	return s.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		w.GetInitialQuery(),
		w.GetMapper(),
		w.GetFirstFilterOption(),
		w.GetFilterOptions()...,
	)
}

// namespaceMapperWatcherMethods represents methods required to be satisfy
// the arguments of NewNamespaceMapperWatcher.
type namespaceMapperWatcherMethods interface {
	GetInitialQuery() eventsource.NamespaceQuery
	GetMapper() eventsource.Mapper
	GetFirstFilterOption() eventsource.FilterOption
	GetFilterOptions() []eventsource.FilterOption
}

// lifeSuspendedStatusWatcher implements the functionality common to both
// the principal and subordinate versions of the LifeSuspendedStatus Watcher.
type lifeSuspendedStatusWatcher struct {
	s *WatchableService

	// appID is the application ID of the application whose relations are
	// being watched for life and being suspended. It is a subordinate
	// application.
	appID application.ID
	// currentRelations holds the life and suspended status of each relation
	// being watched, to check if the values have changed when the Mapper is
	// triggered.
	currentRelations map[corerelation.UUID]relation.RelationLifeSuspendedData

	// processChange is used by GetMapper to decide if the watcher should
	// emit an event for the provided relation UUID.
	processChange func(
		ctx context.Context,
		relUUID corerelation.UUID,
		relationsIgnored set.Strings,
	) (corerelation.Key, error)

	// lifeNameSpace is the namespace where the relation's life can be found.
	lifeNameSpace string
	// suspendedNameSpace is the namespace where relation suspension can be found.
	suspendedNameSpace string
	initialQuery       eventsource.NamespaceQuery
}

// GetInitialQuery returns a function to get the initial results of the
// watcher and setups data to decide whether future notification of those
// relations should be made.
func (w *lifeSuspendedStatusWatcher) GetInitialQuery() eventsource.NamespaceQuery {
	return func(ctx context.Context, txn database.TxnRunner) ([]string, error) {
		ctx, span := trace.Start(ctx, trace.NameFromFunc())
		defer span.End()

		relationUUIDStrings, err := w.initialQuery(ctx, txn)
		if err != nil {
			return nil, errors.Capture(err)
		}

		var initialResults []string
		for _, relUUID := range relationUUIDStrings {
			relUUID := corerelation.UUID(relUUID)
			relationData, err := w.s.st.GetMapperDataForWatchLifeSuspendedStatus(ctx, relUUID, w.appID)
			if errors.Is(err, relationerrors.ApplicationNotFoundForRelation) {
				continue
			} else if err != nil {
				return nil, errors.Capture(err)
			}
			w.currentRelations[relUUID] = relationData
			key, err := corerelation.NewKey(relationData.EndpointIdentifiers)
			if err != nil {
				return nil, errors.Capture(err)
			}
			initialResults = append(initialResults, key.String())
		}

		return initialResults, nil
	}
}

// GetMapper returns a function which decides which relations
// the watcher should notify on for future events.
func (w *lifeSuspendedStatusWatcher) GetMapper() eventsource.Mapper {
	// relationsIgnored is the set of relations which are not relevant to
	// this unit. No need to evaluate them again.
	relationsIgnored := set.NewStrings()
	return func(ctx context.Context, changes []changestream.ChangeEvent) ([]string, error) {
		ctx, span := trace.Start(ctx, trace.NameFromFunc())
		defer span.End()

		// If there are no changes, return no changes.
		if len(changes) == 0 {
			return nil, nil
		}

		changeEvents, err := w.filterChangeEvents(
			ctx,
			changes,
			relationsIgnored,
		)
		if err != nil {
			return nil, errors.Capture(err)
		}
		return changeEvents, nil
	}
}

// continueError indicates that the caller should continue in the
// loop rather than error or assume the happy case.
const continueError = errors.ConstError("continue")

func (w *lifeSuspendedStatusWatcher) filterChangeEvents(
	ctx context.Context,
	changes []changestream.ChangeEvent,
	relationsIgnored set.Strings,
) ([]string, error) {
	var changeEvents []string

	// 2 tables can trigger and report the same relation.
	// Data is gathered from both tables at once, ensure
	// to only check the data and report a change once for
	// each relation.
	changedRelations := make(map[corerelation.UUID]struct{})
	for _, change := range changes {
		changed := change.Changed()
		if relationsIgnored.Contains(changed) {
			continue
		}
		relUUID := corerelation.UUID(changed)
		changedRelations[relUUID] = struct{}{}
	}
	for relUUID := range changedRelations {
		key, err := w.processChange(ctx, relUUID, relationsIgnored)
		if errors.Is(err, continueError) {
			continue
		} else if err != nil {
			return nil, errors.Capture(err)
		}
		changeEvents = append(changeEvents, key.String())
	}

	return changeEvents, nil
}

// GetFirstFilterOption returns a predicate filter for the lifeTableNamespace.
// Relations the Mapper has chosen to ignore will be filtered out of future
// calls to the Mapper.
func (w *lifeSuspendedStatusWatcher) GetFirstFilterOption() eventsource.FilterOption {
	return eventsource.NamespaceFilter(w.lifeNameSpace, changestream.All)
}

// GetFilterOptions returns a predicate filter for the suspendedNameSpace.
// Relations the Mapper has chosen to ignore will be filtered out of future
// calls to the Mapper.
func (w *lifeSuspendedStatusWatcher) GetFilterOptions() []eventsource.FilterOption {
	return []eventsource.FilterOption{eventsource.NamespaceFilter(w.suspendedNameSpace, changestream.All)}
}

// principalLifeSuspendedStatusWatcher implements the processChange method
// unique to watching LifeSuspendedStatus for a principal application.
type principalLifeSuspendedStatusWatcher struct {
	lifeSuspendedStatusWatcher
}

func newPrincipalLifeSuspendedStatusWatcher(s *WatchableService, appID application.ID) namespaceMapperWatcherMethods {
	w := &principalLifeSuspendedStatusWatcher{}
	w.lifeSuspendedStatusWatcher = lifeSuspendedStatusWatcher{
		s:                s,
		appID:            appID,
		currentRelations: make(map[corerelation.UUID]relation.RelationLifeSuspendedData),
		processChange:    w.processChange,
	}
	// returns a set of relation keys if the life or suspended status has changed
	// for any relation this application is part of.
	w.lifeNameSpace, w.suspendedNameSpace, w.initialQuery = s.st.InitialWatchLifeSuspendedStatus(appID)

	return w
}

// processChange returns a relation key when the relation change should
// trigger a notify event from the LifeSuspendedStatusWatcher. For
// principal units, this is when either it's life value has changed, or the
// relation has been suspended. Notify on any new relation for the application
// and continue watching it.
func (w *principalLifeSuspendedStatusWatcher) processChange(
	ctx context.Context,
	relUUID corerelation.UUID,
	relationsIgnored set.Strings,
) (corerelation.Key, error) {
	changedRelationData, err := w.s.st.GetMapperDataForWatchLifeSuspendedStatus(ctx, relUUID, w.appID)
	if errors.Is(err, relationerrors.ApplicationNotFoundForRelation) {
		relationsIgnored.Add(relUUID.String())
		return nil, continueError
	} else if errors.Is(err, relationerrors.RelationNotFound) {
		delete(w.currentRelations, relUUID)
		return nil, continueError
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	// If this is a known relation where neither the Life nor
	// Suspended value have changed, do not notify.
	currentRelationData, ok := w.currentRelations[relUUID]
	if ok && changedRelationData.Life == currentRelationData.Life &&
		changedRelationData.Suspended == currentRelationData.Suspended {
		return nil, continueError
	}

	w.currentRelations[relUUID] = changedRelationData
	key, err := corerelation.NewKey(changedRelationData.EndpointIdentifiers)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return key, nil
}

// subordinateLifeSuspendedStatusWatcher implements the processChange method
// unique to watching LifeSuspendedStatus for a subordinate application.
type subordinateLifeSuspendedStatusWatcher struct {
	lifeSuspendedStatusWatcher

	// parentAppID is the application ID of the parent or principal application
	// of the appID.
	parentAppID application.ID
}

func newSubordinateLifeSuspendedStatusWatcher(s *WatchableService, subordinateID, principalID application.ID) namespaceMapperWatcherMethods {
	w := &subordinateLifeSuspendedStatusWatcher{
		parentAppID: principalID,
	}
	w.lifeSuspendedStatusWatcher = lifeSuspendedStatusWatcher{
		s:                s,
		appID:            subordinateID,
		currentRelations: make(map[corerelation.UUID]relation.RelationLifeSuspendedData),
		processChange:    w.processChange,
	}
	// returns a set of relation keys if the life or suspended status has changed
	// for any relation this application is part of.
	w.lifeNameSpace, w.suspendedNameSpace, w.initialQuery = s.st.InitialWatchLifeSuspendedStatus(subordinateID)
	return w
}

// processChange returns a relation key when the relation change should
// trigger a notify event from the LifeSuspendedStatusWatcher. For
// subordinate units, this is when either it's life value has changed,
// or the relation has been suspended. When a new relation for the
// application is seen, watchNewRelation is used to determine if it should
// be watched as well.
func (w *subordinateLifeSuspendedStatusWatcher) processChange(
	ctx context.Context,
	relUUID corerelation.UUID,
	relationsIgnored set.Strings,
) (corerelation.Key, error) {
	changedRelationData, err := w.s.st.GetMapperDataForWatchLifeSuspendedStatus(ctx, relUUID, w.appID)
	if errors.Is(err, relationerrors.ApplicationNotFoundForRelation) {
		relationsIgnored.Add(relUUID.String())
		return nil, continueError
	} else if errors.Is(err, relationerrors.RelationNotFound) {
		delete(w.currentRelations, relUUID)
		return nil, continueError
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	key, err := corerelation.NewKey(changedRelationData.EndpointIdentifiers)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// If this is a known relation where neither the Life nor
	// Suspended value have changed, do not notify.
	currentRelationData, ok := w.currentRelations[relUUID]
	if ok && (changedRelationData.Life != currentRelationData.Life ||
		changedRelationData.Suspended != currentRelationData.Suspended) {
		w.currentRelations[relUUID] = changedRelationData
		return key, nil
	} else if ok {
		// This relation has been seen before, however neither life
		// has changed nor has its suspended status changed.
		return nil, continueError
	}

	// There is a new relation, check whether to send a notification.
	send, err := w.watchNewRelation(ctx, relUUID)
	if err != nil {
		return nil, errors.Capture(err)
	} else if !send {
		relationsIgnored.Add(relUUID.String())
		return nil, continueError
	}

	w.currentRelations[relUUID] = changedRelationData
	return key, nil
}

// watchNewRelation returns true if the filterChangeEvents
// should emit the event. An event should be emitted if:
//   - The subordinate app's endpoint in the relation is global scoped
//   - If the other app in the relation is the principal app or
//     is a subordinate application.
func (w *subordinateLifeSuspendedStatusWatcher) watchNewRelation(
	ctx context.Context,
	relUUID corerelation.UUID,
) (bool, error) {
	// Relation endpoint for subordinate - is it global? yes - send event
	scope, err := w.s.st.GetRelationEndpointScope(ctx, relUUID, w.appID)
	if errors.Is(err, relationerrors.RelationNotFound) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	if scope == charm.ScopeGlobal {
		return true, nil
	}

	// Only allow container relations if the other end is our
	// principal or the other end is a subordinate.
	otherApp, err := w.s.st.GetOtherRelatedEndpointApplicationData(ctx, relUUID, w.appID)
	if err != nil {
		return false, errors.Capture(err)
	}
	return otherApp.ApplicationID == w.parentAppID || otherApp.Subordinate, nil
}

// WatchRelatedUnits returns a watcher that notifies of changes to counterpart units in
// the relation.
func (s *WatchableService) WatchRelatedUnits(
	ctx context.Context,
	unitUUID unit.UUID,
	relationUUID corerelation.UUID,
) (watcher.StringsWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	if err := relationUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	namespaces, initialQuery, mapper, err := s.st.InitialWatchRelatedUnits(ctx, unitUUID.String(), relationUUID.String())
	if err != nil {
		return nil, errors.Capture(err)
	}

	if len(namespaces) < 1 {
		// This is an error while updating underlying function. It shouldn't happen.
		return nil, errors.New("no namespaces found")
	}
	filters := transform.Slice(namespaces, func(ns string) eventsource.FilterOption {
		return eventsource.NamespaceFilter(ns, changestream.All)
	})
	return s.watcherFactory.NewNamespaceMapperWatcher(ctx, initialQuery, mapper, filters[0], filters[1:]...)
}
