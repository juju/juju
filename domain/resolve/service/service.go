// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/resolve"
	"github.com/juju/juju/internal/errors"
)

// State describes retrieval and persistence methods to resolve units.
type State interface {
	// GetUnitUUID returns the UUID of the unit with the given name, returning
	// an error satisfying [resolveerrors.UnitNotFound] if the unit does not
	// exist.
	GetUnitUUID(context.Context, coreunit.Name) (coreunit.UUID, error)

	// UnitResolveMode returns the resolve mode for the given unit. If no resolved
	// marker is found for the unit, an error satisfying [resolveerrors.UnitNotResolved]
	// is returned.
	UnitResolveMode(context.Context, coreunit.UUID) (resolve.ResolveMode, error)

	// ResolveUnit marks the unit as resolved. If no agent status is found for the
	// specified unit uuid, an error satisfying [resolveerrors.UnitAgentStatusNotFound]
	// is returned. If the unit is not in error status, an error satisfying
	// [resolveerrors.UnitNotInErrorStatus] is returned.
	ResolveUnit(context.Context, coreunit.UUID, resolve.ResolveMode) error

	// ResolveAllUnits marks all units as resolved.
	ResolveAllUnits(context.Context, resolve.ResolveMode) error

	// ClearResolved removes any resolved marker from the unit.
	ClearResolved(context.Context, coreunit.UUID) error

	// NamespaceForWatchUnitResolveMode returns the namespace for watching
	// changes to the resolve mode of a unit.
	NamespaceForWatchUnitResolveMode() string
}

// WatcherFactory instances return watchers for a given namespace and UUID.
type WatcherFactory interface {
	// NewNotifyWatcher returns a new watcher that filters changes from the input
	// base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		summary string,
		filterOption eventsource.FilterOption,
		filterOptions ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// Service provides the API for resolving units.
type Service struct {
	st State
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// UnitResolveMode returns the resolve mode for the given unit. If no unit is found
// with the given name, an error satisfying [resolveerrors.UnitNotFound] is returned.
// if no resolved marker is found for the unit, an error satisfying
// [resolveerrors.UnitNotResolved] is returned.
func (s *Service) UnitResolveMode(ctx context.Context, unitName coreunit.Name) (resolve.ResolveMode, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return "", err
	}
	unitUUID, err := s.st.GetUnitUUID(ctx, unitName)
	if err != nil {
		return "", errors.Errorf("getting unit UUID for %q: %w", unitName, err)
	}
	return s.st.UnitResolveMode(ctx, unitUUID)
}

// ResolveUnit marks the unit as resolved. If the unit is not found, an error
// satisfying [resolveerrors.UnitNotFound] is returned. If the unit is not in
// error state, an error satisfying [resolveerrors.UnitNotInErrorState] is
// returned.
func (s *Service) ResolveUnit(ctx context.Context, unitName coreunit.Name, mode resolve.ResolveMode) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return err
	}
	unitUUID, err := s.st.GetUnitUUID(ctx, unitName)
	if err != nil {
		return errors.Errorf("getting unit UUID for %q: %w", unitName, err)
	}
	return s.st.ResolveUnit(ctx, unitUUID, mode)
}

// ResolveAllUnits marks all units as resolved.
func (s *Service) ResolveAllUnits(ctx context.Context, mode resolve.ResolveMode) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return s.st.ResolveAllUnits(ctx, mode)
}

// ClearResolved removes any resolved marker from the unit. If the unit is not
// found, an error satisfying [resolveerrors.UnitNotFound] is returned.
func (s *Service) ClearResolved(ctx context.Context, unitName coreunit.Name) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return err
	}
	unitUUID, err := s.st.GetUnitUUID(ctx, unitName)
	if err != nil {
		return errors.Errorf("getting unit UUID for %q: %w", unitName, err)
	}
	return s.st.ClearResolved(ctx, unitUUID)
}

// WatchableService provides the API for resolving unit and the ability
// to create watchers that watch for changes to the resolve mode of units.
type WatchableService struct {
	*Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new watchable service reference wrapping the
// input state.
func NewWatchableService(st State, watcherFactory WatcherFactory) *WatchableService {
	return &WatchableService{
		Service:        NewService(st),
		watcherFactory: watcherFactory,
	}
}

// WatchUnitResolveMode returns a watcher that emits notification when the resolve
// mode of the specified unit changes.
//
// If the unit does not exist an error satisfying [resolveerrors.UnitNotFound]
// will be returned.
func (s *WatchableService) WatchUnitResolveMode(ctx context.Context, unitName coreunit.Name) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return nil, err
	}
	unitUUID, err := s.st.GetUnitUUID(ctx, unitName)
	if err != nil {
		return nil, errors.Errorf("getting unit UUID for %q: %w", unitName, err)
	}

	resolveNamespace := s.st.NamespaceForWatchUnitResolveMode()
	return s.watcherFactory.NewNotifyWatcher(
		fmt.Sprintf("unit resolve mode watcher for %q", unitName),
		eventsource.PredicateFilter(
			resolveNamespace,
			changestream.All,
			eventsource.EqualsPredicate(unitUUID.String()),
		),
	)
}
