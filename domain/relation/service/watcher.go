// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

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
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/relation/internal"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// WatcherState represents the subset of the State interface required by
// the WatchableService.
type WatcherState interface {
	// GetPrincipalSubordinateApplicationUUIDs returns the Principal and
	// Subordinate application UUIDs for the given unit. The principal will
	// be the first UUID returned and the subordinate will be the second. If
	// the unit is not a subordinate, the second application UUID will be
	// empty.
	GetPrincipalSubordinateApplicationUUIDs(
		ctx context.Context,
		unitUUID unit.UUID,
	) (application.UUID, application.UUID, error)

	// GetMapperDataForWatchLifeSuspendedStatus returns data needed to evaluate
	// a relation uuid as part of WatchLifeSuspendedStatus eventmapper.
	GetMapperDataForWatchLifeSuspendedStatus(
		ctx context.Context,
		relUUID corerelation.UUID,
		appUUID application.UUID,
	) (relation.RelationLifeSuspendedData, error)

	// GetRelationEndpointScope returns the scope of the relation endpoint
	// at the intersection of the relationUUID and applicationUUID.
	GetRelationEndpointScope(
		ctx context.Context,
		relationUUID corerelation.UUID,
		applicationID application.UUID,
	) (charm.RelationScope, error)

	// GetOtherRelatedEndpointApplicationData returns an
	// OtherApplicationForWatcher struct for each Endpoint in a relation with
	// the given application UUID.
	GetOtherRelatedEndpointApplicationData(
		ctx context.Context,
		relUUID corerelation.UUID,
		applicationID application.UUID,
	) (relation.OtherApplicationForWatcher, error)

	// GetWatcherRelationUnitsData returns the data used to the RelationsUnits
	// watcher: relation endpoint UUID and namespaces.
	GetWatcherRelationUnitsData(
		context.Context,
		corerelation.UUID,
		application.UUID,
	) (internal.WatcherRelationUnitsData, error)

	// InitialWatchLifeSuspendedStatus returns the two tables to watch for
	// a relation's Life and Suspended status when the relation contains
	// the provided application and the initial namespace query.
	InitialWatchLifeSuspendedStatus(id application.UUID) (string, eventsource.NamespaceQuery)

	// InitialWatchRelatedUnits initializes a watch for changes related to the
	// specified unit in the given relation.
	InitialWatchRelatedUnits(
		ctx context.Context, unitUUID, relUUID string,
	) ([]string, eventsource.NamespaceQuery, eventsource.Mapper, error)

	// WatcherApplicationSettingsNamespace provides the table name to set up
	// watchers for relation application settings.
	WatcherApplicationSettingsNamespace() string
}

// WatcherFactory describes methods for creating watchers that are used by the
// WatchableService.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new watcher that receives changes from the
	// input base watcher's db/queue.
	NewNamespaceMapperWatcher(
		ctx context.Context,
		initialQuery eventsource.NamespaceQuery,
		summary string,
		mapper eventsource.Mapper,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)

	// NewNotifyMapperWatcher returns a new watcher that receives changes from the
	// input base watcher's db/queue.
	NewNotifyMapperWatcher(
		ctx context.Context,
		summary string,
		mapper eventsource.Mapper,
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)

	// NewNotifyWatcher returns a new watcher that filters changes from the input
	// base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		ctx context.Context,
		summary string,
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
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

// WatchRelationLifeSuspendedStatus returns a watcher that notifies when
// there are changes to the given relation's life or suspended status.
func (s *WatchableService) WatchRelationLifeSuspendedStatus(
	ctx context.Context,
	relationUUID corerelation.UUID,
) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := relationUUID.Validate(); err != nil {
		return nil, errors.Errorf(
			"watching relation life suspended status: %w", err).Add(relationerrors.RelationUUIDNotValid)
	}

	return s.watcherFactory.NewNotifyWatcher(
		ctx,
		fmt.Sprintf("watch relation life suspended status for %q", relationUUID),
		eventsource.PredicateFilter(
			s.st.GetRelationLifeSuspendedNameSpace(),
			changestream.All,
			eventsource.EqualsPredicate(relationUUID.String()),
		),
	)
}

// WatchRelationUnitApplicationLifeSuspendedStatus returns a watcher that notifies of
// changes to the life or suspended status any relation the unit's application
// is part of. If the unit is a subordinate, its principal application is
// watched. The watcher notifies with the relation keys.
func (s *WatchableService) WatchRelationUnitApplicationLifeSuspendedStatus(
	ctx context.Context,
	unitUUID unit.UUID,
) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitUUID.Validate(); err != nil {
		return nil, errors.Errorf(
			"watching relation life suspended status: %w", err).Add(applicationerrors.UnitUUIDNotValid)
	}

	principalID, subordinateID, err := s.st.GetPrincipalSubordinateApplicationUUIDs(ctx, unitUUID)
	if err != nil {
		return nil, errors.Errorf("finding principal and subordinate application UUIDs: %w", err)
	}

	var w namespaceMapper
	if subordinateID.IsEmpty() {
		w = newPrincipalLifeSuspendedStatusWatcher(s, principalID)
	} else {
		w = newSubordinateLifeSuspendedStatusWatcher(s, principalID, subordinateID)
	}
	return s.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		w.GetInitialQuery(),
		fmt.Sprintf("life suspended status watcher for unit %q", unitUUID),
		w.GetMapper(),
		w.GetFilterOption(),
	)
}

// WatchRelationsLifeSuspendedStatusForApplication returns a watcher that
// notifies of changes to the life or suspended status for any relation the
// application is part of. The watcher notifies with the relation UUIDs.
func (s *WatchableService) WatchRelationsLifeSuspendedStatusForApplication(
	ctx context.Context,
	applicationUUID application.UUID,
) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := applicationUUID.Validate(); err != nil {
		return nil, errors.Errorf(
			"%w:%w", applicationerrors.ApplicationUUIDNotValid, err)
	}

	// Check if the application exists before starting the watcher.
	// This prevents watching an application that has been removed or
	// never existed.
	if err := s.st.ApplicationExists(ctx, applicationUUID); err != nil {
		return nil, errors.Errorf("checking application exists: %w", err)
	}

	w := newApplicationLifeSuspendedStatusWatcher(s, applicationUUID)
	return s.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		w.GetInitialQuery(),
		fmt.Sprintf("life suspended status watcher for application %q", applicationUUID),
		w.GetMapper(),
		w.GetFilterOption(),
	)
}

// WatchRelatedUnits returns a watcher that notifies of changes to counterpart
// units in the relation.
func (s *WatchableService) WatchRelatedUnits(
	ctx context.Context,
	unitUUID unit.UUID,
	relationUUID corerelation.UUID,
) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
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
	return s.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		initialQuery,
		fmt.Sprintf("related units watcher for %q in %q", unitUUID, relationUUID),
		mapper,
		filters[0], filters[1:]...,
	)
}

// WatchRelationUnits returns a watcher for changes to the units
// in the given relation in the local model.
func (s *WatchableService) WatchRelationUnits(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationUUID application.UUID,
) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := applicationUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	if err := relationUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	watcherRelationUnitsData, err := s.st.GetWatcherRelationUnitsData(ctx, relationUUID, applicationUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	relationUnitUUIDs := set.NewStrings()
	mapper := func(ctx context.Context, events []changestream.ChangeEvent) ([]string, error) {
		var out []string
		for _, e := range events {
			var wantEvent bool
			switch e.Namespace() {
			case watcherRelationUnitsData.UnitSettingsHashNS:
				if relationUnitUUIDs.Contains(e.Changed()) {
					wantEvent = true
				}
			case watcherRelationUnitsData.ApplicationSettingsHashNS:
				wantEvent = true
			case watcherRelationUnitsData.RelationUnitNS:
				relUnitUUIDs, err := s.st.GetRelationUnitUUIDsByEndpointUUID(ctx, watcherRelationUnitsData.RelationEndpointUUID)
				if err != nil {
					return nil, errors.Capture(err)
				}
				relationUnitUUIDs = set.NewStrings(relUnitUUIDs...)

				wantEvent = true
			}
			if wantEvent {
				out = append(out, e.Changed())
			}
		}
		return out, nil
	}

	return s.watcherFactory.NewNotifyMapperWatcher(
		ctx,
		"WatchRelationUnits",
		mapper,
		eventsource.PredicateFilter(
			watcherRelationUnitsData.RelationUnitNS,
			changestream.All,
			eventsource.EqualsPredicate(watcherRelationUnitsData.RelationEndpointUUID),
		),
		eventsource.PredicateFilter(
			watcherRelationUnitsData.ApplicationSettingsHashNS,
			changestream.All,
			eventsource.EqualsPredicate(watcherRelationUnitsData.RelationEndpointUUID),
		),
		eventsource.NamespaceFilter(
			watcherRelationUnitsData.UnitSettingsHashNS,
			changestream.All,
		),
	)
}

// namespaceMapper represents methods required to be satisfy the arguments of
// NewNamespaceMapperWatcher.
type namespaceMapper interface {
	// GetInitialQuery returns a function to get the initial values of the
	// watcher.
	GetInitialQuery() eventsource.NamespaceQuery

	// GetMapper returns a function which maps the changes from the watcher
	// to the values to be returned by the watcher.
	GetMapper() eventsource.Mapper

	// GetFilterOption returns the first filter option to be passed to
	// NewNamespaceMapperWatcher.
	GetFilterOption() eventsource.FilterOption
}

// lifeSuspendedStatusKey allows either corerelation.Key or corerelation.UUID
// to be used as the type for the lifeSuspendedStatusWatcher.
type lifeSuspendedStatusKey interface {
	corerelation.Key | corerelation.UUID
	fmt.Stringer
}

// lifeSuspendedStatusWatcher implements the functionality common to both the
// principal, subordinate and application versions of the LifeSuspendedStatus
// Watcher.
type lifeSuspendedStatusWatcher[T lifeSuspendedStatusKey] struct {
	s *WatchableService

	// appUUID is the application UUID of the application whose relations are
	// being watched for life and being suspended. It is a subordinate
	// application.
	appUUID application.UUID
	// currentRelations holds the life and suspended status of each relation
	// being watched, to check if the values have changed when the Mapper is
	// triggered.
	currentRelations map[corerelation.UUID]relation.RelationLifeSuspendedData

	// processInitialChange is used by GetInitialQuery to process the
	// initial set of relations the application is part of.
	processInitialChange func(
		ctx context.Context,
		relUUID corerelation.UUID,
		data relation.RelationLifeSuspendedData,
	) (T, error)

	// processChange is used by GetMapper to decide if the watcher should
	// emit an event for the provided relation UUID.
	processChange func(
		ctx context.Context,
		relUUID corerelation.UUID,
		relationsIgnored set.Strings,
	) (T, error)

	// relationNameSpace is the namespace where the relation's can be found.
	relationNameSpace string
	initialQuery      eventsource.NamespaceQuery
}

// GetInitialQuery returns a function to get the initial results of the
// watcher and setups data to decide whether future notification of those
// relations should be made.
func (w *lifeSuspendedStatusWatcher[T]) GetInitialQuery() eventsource.NamespaceQuery {
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
			relationData, err := w.s.st.GetMapperDataForWatchLifeSuspendedStatus(ctx, relUUID, w.appUUID)
			if errors.Is(err, relationerrors.ApplicationNotFoundForRelation) {
				continue
			} else if err != nil {
				return nil, errors.Capture(err)
			}
			w.currentRelations[relUUID] = relationData
			change, err := w.processInitialChange(ctx, relUUID, relationData)
			if err != nil {
				return nil, errors.Capture(err)
			}
			initialResults = append(initialResults, change.String())
		}

		return initialResults, nil
	}
}

// GetMapper returns a function which decides which relations
// the watcher should notify on for future events.
func (w *lifeSuspendedStatusWatcher[T]) GetMapper() eventsource.Mapper {
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

func (w *lifeSuspendedStatusWatcher[T]) filterChangeEvents(
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
		change, err := w.processChange(ctx, relUUID, relationsIgnored)
		if errors.Is(err, continueError) {
			continue
		} else if err != nil {
			return nil, errors.Capture(err)
		}
		changeEvents = append(changeEvents, change.String())
	}

	return changeEvents, nil
}

// GetFilterOption returns a predicate filter for the relation namespace.
// Relations the Mapper has chosen to ignore will be filtered out of future
// calls to the Mapper.
func (w *lifeSuspendedStatusWatcher[T]) GetFilterOption() eventsource.FilterOption {
	return eventsource.NamespaceFilter(w.relationNameSpace, changestream.All)
}

// principalLifeSuspendedStatusWatcher implements the processChange method
// unique to watching LifeSuspendedStatus for a principal application.
type principalLifeSuspendedStatusWatcher struct {
	lifeSuspendedStatusWatcher[corerelation.Key]
}

func newPrincipalLifeSuspendedStatusWatcher(s *WatchableService, appUUID application.UUID) namespaceMapper {
	w := &principalLifeSuspendedStatusWatcher{}
	w.lifeSuspendedStatusWatcher = lifeSuspendedStatusWatcher[corerelation.Key]{
		s:                    s,
		appUUID:              appUUID,
		currentRelations:     make(map[corerelation.UUID]relation.RelationLifeSuspendedData),
		processChange:        w.processChange,
		processInitialChange: w.processInitialChange,
	}
	// returns a set of relation keys if the life or suspended status has
	// changed for any relation this application is part of.
	w.relationNameSpace, w.initialQuery = s.st.InitialWatchLifeSuspendedStatus(appUUID)

	return w
}

// processInitialChange returns the relation key for the initial set of
// relations the application is part of.
func (w *principalLifeSuspendedStatusWatcher) processInitialChange(
	ctx context.Context,
	relUUID corerelation.UUID,
	data relation.RelationLifeSuspendedData,
) (corerelation.Key, error) {
	return corerelation.NewKey(data.EndpointIdentifiers)
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
	changedRelationData, err := w.s.st.GetMapperDataForWatchLifeSuspendedStatus(ctx, relUUID, w.appUUID)
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
	lifeSuspendedStatusWatcher[corerelation.Key]

	// parentAppID is the application UUID of the parent or principal application
	// of the appUUID.
	parentAppID application.UUID
}

func newSubordinateLifeSuspendedStatusWatcher(s *WatchableService, subordinateID, principalID application.UUID) namespaceMapper {
	w := &subordinateLifeSuspendedStatusWatcher{
		parentAppID: principalID,
	}
	w.lifeSuspendedStatusWatcher = lifeSuspendedStatusWatcher[corerelation.Key]{
		s:                    s,
		appUUID:              subordinateID,
		currentRelations:     make(map[corerelation.UUID]relation.RelationLifeSuspendedData),
		processChange:        w.processChange,
		processInitialChange: w.processInitialChange,
	}
	// returns a set of relation keys if the life or suspended status has
	// changed for any relation this application is part of.
	w.relationNameSpace, w.initialQuery = s.st.InitialWatchLifeSuspendedStatus(subordinateID)
	return w
}

// processInitialChange returns the relation key for the initial set of
// relations the application is part of.
func (w *subordinateLifeSuspendedStatusWatcher) processInitialChange(
	ctx context.Context,
	relUUID corerelation.UUID,
	data relation.RelationLifeSuspendedData,
) (corerelation.Key, error) {
	return corerelation.NewKey(data.EndpointIdentifiers)
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
	changedRelationData, err := w.s.st.GetMapperDataForWatchLifeSuspendedStatus(ctx, relUUID, w.appUUID)
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
	scope, err := w.s.st.GetRelationEndpointScope(ctx, relUUID, w.appUUID)
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
	otherApp, err := w.s.st.GetOtherRelatedEndpointApplicationData(ctx, relUUID, w.appUUID)
	if err != nil {
		return false, errors.Capture(err)
	}
	return otherApp.ApplicationID == w.parentAppID || otherApp.Subordinate, nil
}

// applicationLifeSuspendedStatusWatcher implements the processChange method
// unique to watching LifeSuspendedStatus for a application.
type applicationLifeSuspendedStatusWatcher struct {
	lifeSuspendedStatusWatcher[corerelation.UUID]
}

func newApplicationLifeSuspendedStatusWatcher(s *WatchableService, appUUID application.UUID) namespaceMapper {
	w := &applicationLifeSuspendedStatusWatcher{}
	w.lifeSuspendedStatusWatcher = lifeSuspendedStatusWatcher[corerelation.UUID]{
		s:                    s,
		appUUID:              appUUID,
		currentRelations:     make(map[corerelation.UUID]relation.RelationLifeSuspendedData),
		processChange:        w.processChange,
		processInitialChange: w.processInitialChange,
	}
	// returns a set of relation keys if the life or suspended status has changed
	// for any relation this application is part of.
	w.relationNameSpace, w.initialQuery = s.st.InitialWatchLifeSuspendedStatus(appUUID)

	return w
}

// processInitialChange returns the relation UUID for the initial set of
// relations the application is part of.
func (w *applicationLifeSuspendedStatusWatcher) processInitialChange(
	ctx context.Context,
	relUUID corerelation.UUID,
	data relation.RelationLifeSuspendedData,
) (corerelation.UUID, error) {
	return relUUID, nil
}

// processChange returns a relation key when the relation change should
// trigger a notify event from the LifeSuspendedStatusWatcher. For
// principal units, this is when either it's life value has changed, or the
// relation has been suspended. Notify on any new relation for the application
// and continue watching it.
func (w *applicationLifeSuspendedStatusWatcher) processChange(
	ctx context.Context,
	relUUID corerelation.UUID,
	relationsIgnored set.Strings,
) (corerelation.UUID, error) {
	changedRelationData, err := w.s.st.GetMapperDataForWatchLifeSuspendedStatus(ctx, relUUID, w.appUUID)
	if errors.Is(err, relationerrors.ApplicationNotFoundForRelation) {
		relationsIgnored.Add(relUUID.String())
		return "", continueError
	} else if errors.Is(err, relationerrors.RelationNotFound) {
		delete(w.currentRelations, relUUID)
		return "", continueError
	} else if err != nil {
		return "", errors.Capture(err)
	}

	// If this is a known relation where neither the Life nor
	// Suspended value have changed, do not notify.
	currentRelationData, ok := w.currentRelations[relUUID]
	if ok && changedRelationData.Life == currentRelationData.Life &&
		changedRelationData.Suspended == currentRelationData.Suspended {
		return "", continueError
	}

	w.currentRelations[relUUID] = changedRelationData
	return relUUID, nil
}
