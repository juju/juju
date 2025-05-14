// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/trace"
	coreupgrade "github.com/juju/juju/core/upgrade"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/upgrade"
	upgradeerrors "github.com/juju/juju/domain/upgrade/errors"
	"github.com/juju/juju/internal/errors"
)

// State describes retrieval and persistence methods for upgrade info.
type State interface {
	CreateUpgrade(context.Context, semversion.Number, semversion.Number) (upgrade.UUID, error)
	SetControllerReady(context.Context, upgrade.UUID, string) error
	AllProvisionedControllersReady(context.Context, upgrade.UUID) (bool, error)
	StartUpgrade(context.Context, upgrade.UUID) error
	SetControllerDone(context.Context, upgrade.UUID, string) error
	ActiveUpgrade(context.Context) (upgrade.UUID, error)
	SetDBUpgradeCompleted(context.Context, upgrade.UUID) error
	SetDBUpgradeFailed(context.Context, upgrade.UUID) error
	UpgradeInfo(context.Context, upgrade.UUID) (coreupgrade.Info, error)
	NamespaceForWatchUpgradeReady() string
	NamespaceForWatchUpgradeState() string
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNotifyMapperWatcher returns a new watcher that receives changes from
	// the input base watcher's db/queue. A single filter option is required,
	// though additional filter options can be provided. Filtering of values is
	// done first by the filter, and then subsequently by the mapper. Based on
	// the mapper's logic a subset of them (or none) may be emitted.
	NewNotifyMapperWatcher(
		mapper eventsource.Mapper,
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// Service provides the API for working with upgrade info
type Service struct {
	st State
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State, wf WatcherFactory) *Service {
	return &Service{st: st}
}

// CreateUpgrade creates an upgrade to and from specified versions
// If an upgrade is already running/pending, return an AlreadyExists err
func (s *Service) CreateUpgrade(ctx context.Context, previousVersion, targetVersion semversion.Number) (_ upgrade.UUID, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	if previousVersion.Compare(targetVersion) >= 0 {
		return "", errors.Errorf("target version %q must be greater than current version %q %w", targetVersion, previousVersion, coreerrors.NotValid)
	}
	return s.st.CreateUpgrade(ctx, previousVersion, targetVersion)
}

// SetControllerReady marks the supplied controllerID as being ready
// to start its upgrade. All provisioned controllers need to be ready
// before an upgrade can start
func (s *Service) SetControllerReady(ctx context.Context, upgradeUUID upgrade.UUID, controllerID string) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := upgradeUUID.Validate(); err != nil {
		return errors.Capture(err)
	}
	return s.st.SetControllerReady(ctx, upgradeUUID, controllerID)
}

// StartUpgrade starts the current upgrade if it exists
func (s *Service) StartUpgrade(ctx context.Context, upgradeUUID upgrade.UUID) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := upgradeUUID.Validate(); err != nil {
		return errors.Capture(err)
	}
	return s.st.StartUpgrade(ctx, upgradeUUID)
}

// SetControllerDone marks the supplied controllerID as having
// completed its upgrades. When SetControllerDone is called by the
// last provisioned controller, the upgrade will be archived.
func (s *Service) SetControllerDone(ctx context.Context, upgradeUUID upgrade.UUID, controllerID string) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := upgradeUUID.Validate(); err != nil {
		return errors.Capture(err)
	}
	return s.st.SetControllerDone(ctx, upgradeUUID, controllerID)
}

// SetDBUpgradeCompleted marks the upgrade as completed in the database
func (s *Service) SetDBUpgradeCompleted(ctx context.Context, upgradeUUID upgrade.UUID) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := upgradeUUID.Validate(); err != nil {
		return errors.Capture(err)
	}
	return s.st.SetDBUpgradeCompleted(ctx, upgradeUUID)
}

// SetDBUpgradeFailed marks the upgrade as failed in the database.
// Manual intervention will be required if this has been invoked.
func (s *Service) SetDBUpgradeFailed(ctx context.Context, upgradeUUID upgrade.UUID) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := upgradeUUID.Validate(); err != nil {
		return errors.Capture(err)
	}
	return s.st.SetDBUpgradeFailed(ctx, upgradeUUID)
}

// ActiveUpgrade returns the uuid of the current active upgrade.
// If there are no active upgrades, return a NotFound error
func (s *Service) ActiveUpgrade(ctx context.Context) (_ upgrade.UUID, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return s.st.ActiveUpgrade(ctx)
}

// UpgradeInfo returns the upgrade info for the supplied upgradeUUID
func (s *Service) UpgradeInfo(ctx context.Context, upgradeUUID upgrade.UUID) (_ coreupgrade.Info, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	if err := upgradeUUID.Validate(); err != nil {
		return coreupgrade.Info{}, errors.Capture(err)
	}
	return s.st.UpgradeInfo(ctx, upgradeUUID)
}

// IsUpgrading returns true if there is an upgrade in progress.
// This essentially asks is there any upgrades that are not in the terminal
// states (completed or failed)
func (s *Service) IsUpgrading(ctx context.Context) (_ bool, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if _, err := s.ActiveUpgrade(ctx); err == nil {
		return true, nil
	}
	if errors.Is(err, upgradeerrors.NotFound) {
		return false, nil
	}

	return false, errors.Capture(err)
}

// WatchableService provides the API for working with upgrade info
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new WatchableService for interacting with the underlying state.
func NewWatchableService(st State, wf WatcherFactory) *WatchableService {
	return &WatchableService{
		Service: Service{
			st: st,
		},
		watcherFactory: wf,
	}
}

// WatchForUpgradeReady creates a watcher which notifies when all controller
// nodes have been registered, meaning the upgrade is ready to start.
func (s *WatchableService) WatchForUpgradeReady(ctx context.Context, upgradeUUID upgrade.UUID) (watcher.NotifyWatcher, error) {

	if err := upgradeUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	mapper := func(ctx context.Context, changes []changestream.ChangeEvent) (_ []changestream.ChangeEvent, err error) {
		ctx, span := trace.Start(ctx, trace.NameFromFunc())
		defer span.End()

		ready, err := s.st.AllProvisionedControllersReady(ctx, upgradeUUID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		// Only dispatch if all controllers are ready.
		if ready {
			return changes, nil
		}
		return nil, nil
	}
	return s.watcherFactory.NewNotifyMapperWatcher(
		mapper,
		eventsource.PredicateFilter(
			s.st.NamespaceForWatchUpgradeReady(),
			changestream.Changed,
			eventsource.EqualsPredicate(upgradeUUID.String()),
		),
	)
}

// WatchForUpgradeState creates a watcher which notifies when the upgrade
// has reached the given state.
func (s *WatchableService) WatchForUpgradeState(ctx context.Context, upgradeUUID upgrade.UUID, state coreupgrade.State) (_ watcher.NotifyWatcher, err error) {
	if err := upgradeUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	mapper := func(ctx context.Context, changes []changestream.ChangeEvent) (_ []changestream.ChangeEvent, err error) {
		ctx, span := trace.Start(ctx, trace.NameFromFunc())
		defer span.End()

		info, err := s.st.UpgradeInfo(ctx, upgradeUUID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		if info.State == state {
			return changes, nil
		}
		return nil, nil
	}
	return s.watcherFactory.NewNotifyMapperWatcher(
		mapper,
		eventsource.PredicateFilter(
			s.st.NamespaceForWatchUpgradeState(),
			changestream.Changed,
			eventsource.EqualsPredicate(upgradeUUID.String()),
		),
	)
}
