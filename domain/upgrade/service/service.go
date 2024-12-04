// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/version/v2"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coreupgrade "github.com/juju/juju/core/upgrade"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/upgrade"
	upgradeerrors "github.com/juju/juju/domain/upgrade/errors"
	"github.com/juju/juju/internal/errors"
)

// State describes retrieval and persistence methods for upgrade info.
type State interface {
	CreateUpgrade(context.Context, version.Number, version.Number) (upgrade.UUID, error)
	SetControllerReady(context.Context, upgrade.UUID, string) error
	AllProvisionedControllersReady(context.Context, upgrade.UUID) (bool, error)
	StartUpgrade(context.Context, upgrade.UUID) error
	SetControllerDone(context.Context, upgrade.UUID, string) error
	ActiveUpgrade(context.Context) (upgrade.UUID, error)
	SetDBUpgradeCompleted(context.Context, upgrade.UUID) error
	SetDBUpgradeFailed(context.Context, upgrade.UUID) error
	UpgradeInfo(context.Context, upgrade.UUID) (coreupgrade.Info, error)
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewValueMapperWatcher returns a new namespace watcher
	// for events based on the input change mask and predicate.
	NewValueMapperWatcher(string, string, changestream.ChangeType, eventsource.Mapper) (watcher.NotifyWatcher, error)
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
func (s *Service) CreateUpgrade(ctx context.Context, previousVersion, targetVersion version.Number) (upgrade.UUID, error) {
	if previousVersion.Compare(targetVersion) >= 0 {
		return "", errors.Errorf("target version %q must be greater than current version %q %w", targetVersion, previousVersion, coreerrors.NotValid)
	}
	return s.st.CreateUpgrade(ctx, previousVersion, targetVersion)
}

// SetControllerReady marks the supplied controllerID as being ready
// to start its upgrade. All provisioned controllers need to be ready
// before an upgrade can start
func (s *Service) SetControllerReady(ctx context.Context, upgradeUUID upgrade.UUID, controllerID string) error {
	if err := upgradeUUID.Validate(); err != nil {
		return errors.Capture(err)
	}
	err := s.st.SetControllerReady(ctx, upgradeUUID, controllerID)
	return err
}

// StartUpgrade starts the current upgrade if it exists
func (s *Service) StartUpgrade(ctx context.Context, upgradeUUID upgrade.UUID) error {
	if err := upgradeUUID.Validate(); err != nil {
		return errors.Capture(err)
	}
	return s.st.StartUpgrade(ctx, upgradeUUID)
}

// SetControllerDone marks the supplied controllerID as having
// completed its upgrades. When SetControllerDone is called by the
// last provisioned controller, the upgrade will be archived.
func (s *Service) SetControllerDone(ctx context.Context, upgradeUUID upgrade.UUID, controllerID string) error {
	if err := upgradeUUID.Validate(); err != nil {
		return errors.Capture(err)
	}
	return s.st.SetControllerDone(ctx, upgradeUUID, controllerID)
}

// SetDBUpgradeCompleted marks the upgrade as completed in the database
func (s *Service) SetDBUpgradeCompleted(ctx context.Context, upgradeUUID upgrade.UUID) error {
	if err := upgradeUUID.Validate(); err != nil {
		return errors.Capture(err)
	}
	return s.st.SetDBUpgradeCompleted(ctx, upgradeUUID)
}

// SetDBUpgradeFailed marks the upgrade as failed in the database.
// Manual intervention will be required if this has been invoked.
func (s *Service) SetDBUpgradeFailed(ctx context.Context, upgradeUUID upgrade.UUID) error {
	if err := upgradeUUID.Validate(); err != nil {
		return errors.Capture(err)
	}
	return s.st.SetDBUpgradeFailed(ctx, upgradeUUID)
}

// ActiveUpgrade returns the uuid of the current active upgrade.
// If there are no active upgrades, return a NotFound error
func (s *Service) ActiveUpgrade(ctx context.Context) (upgrade.UUID, error) {
	return s.st.ActiveUpgrade(ctx)
}

// UpgradeInfo returns the upgrade info for the supplied upgradeUUID
func (s *Service) UpgradeInfo(ctx context.Context, upgradeUUID upgrade.UUID) (coreupgrade.Info, error) {
	if err := upgradeUUID.Validate(); err != nil {
		return coreupgrade.Info{}, errors.Capture(err)
	}
	return s.st.UpgradeInfo(ctx, upgradeUUID)
}

// IsUpgrading returns true if there is an upgrade in progress.
// This essentially asks is there any upgrades that are not in the terminal
// states (completed or failed)
func (s *Service) IsUpgrading(ctx context.Context) (bool, error) {
	_, err := s.ActiveUpgrade(ctx)
	if err == nil {
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

	mask := changestream.Create | changestream.Update
	mapper := func(ctx context.Context, db coredatabase.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
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
	return s.watcherFactory.NewValueMapperWatcher("upgrade_info_controller_node", upgradeUUID.String(), mask, mapper)
}

// WatchForUpgradeState creates a watcher which notifies when the upgrade
// has reached the given state.
func (s *WatchableService) WatchForUpgradeState(ctx context.Context, upgradeUUID upgrade.UUID, state coreupgrade.State) (watcher.NotifyWatcher, error) {
	if err := upgradeUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	mask := changestream.Create | changestream.Update
	mapper := func(ctx context.Context, db coredatabase.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
		info, err := s.st.UpgradeInfo(ctx, upgradeUUID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		if info.State == state {
			return changes, nil
		}
		return nil, nil
	}
	return s.watcherFactory.NewValueMapperWatcher("upgrade_info", upgradeUUID.String(), mask, mapper)
}
