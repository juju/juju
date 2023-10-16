// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/upgrade"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	domainupgrade "github.com/juju/juju/domain/upgrade"
	"github.com/juju/juju/internal/database"
)

// State describes retrieval and persistence methods for upgrade info.
type State interface {
	CreateUpgrade(context.Context, version.Number, version.Number) (domainupgrade.UUID, error)
	SetControllerReady(context.Context, domainupgrade.UUID, string) error
	AllProvisionedControllersReady(context.Context, domainupgrade.UUID) (bool, error)
	StartUpgrade(context.Context, domainupgrade.UUID) error
	SetControllerDone(context.Context, domainupgrade.UUID, string) error
	ActiveUpgrade(context.Context) (domainupgrade.UUID, error)
	SetDBUpgradeCompleted(context.Context, domainupgrade.UUID) error
	SetDBUpgradeFailed(context.Context, domainupgrade.UUID) error
	UpgradeInfo(context.Context, domainupgrade.UUID) (upgrade.Info, error)
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewValuePredicateWatcher returns a new namespace watcher
	// for events based on the input change mask and predicate.
	NewValuePredicateWatcher(string, string, changestream.ChangeType, eventsource.Predicate) (watcher.NotifyWatcher, error)
}

// Service provides the API for working with upgrade info
type Service struct {
	st             State
	watcherFactory WatcherFactory
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State, wf WatcherFactory) *Service {
	return &Service{st: st, watcherFactory: wf}
}

// CreateUpgrade creates an upgrade to and from specified versions
// If an upgrade is already running/pending, return an AlreadyExists err
func (s *Service) CreateUpgrade(ctx context.Context, previousVersion, targetVersion version.Number) (domainupgrade.UUID, error) {
	if previousVersion.Compare(targetVersion) >= 0 {
		return "", errors.NotValidf("target version %q must be greater than current version %q", targetVersion, previousVersion)
	}
	upgradeUUID, err := s.st.CreateUpgrade(ctx, previousVersion, targetVersion)
	if database.IsErrConstraintUnique(err) {
		return "", errors.AlreadyExistsf("active upgrade")
	}
	return upgradeUUID, domain.CoerceError(err)
}

// SetControllerReady marks the supplied controllerID as being ready
// to start its upgrade. All provisioned controllers need to be ready
// before an upgrade can start
func (s *Service) SetControllerReady(ctx context.Context, upgradeUUID domainupgrade.UUID, controllerID string) error {
	if err := upgradeUUID.Validate(); err != nil {
		return errors.Trace(err)
	}
	err := s.st.SetControllerReady(ctx, upgradeUUID, controllerID)
	if database.IsErrConstraintForeignKey(err) {
		return errors.NotFoundf("upgrade %q", upgradeUUID)
	}
	return domain.CoerceError(err)
}

// StartUpgrade starts the current upgrade if it exists
func (s *Service) StartUpgrade(ctx context.Context, upgradeUUID domainupgrade.UUID) error {
	if err := upgradeUUID.Validate(); err != nil {
		return errors.Trace(err)
	}
	err := s.st.StartUpgrade(ctx, upgradeUUID)
	if database.IsErrNotFound(err) {
		return errors.NotFoundf("upgrade %q", upgradeUUID)
	}
	return domain.CoerceError(err)
}

// SetControllerDone marks the supplied controllerID as having
// completed its upgrades. When SetControllerDone is called by the
// last provisioned controller, the upgrade will be archived.
func (s *Service) SetControllerDone(ctx context.Context, upgradeUUID domainupgrade.UUID, controllerID string) error {
	if err := upgradeUUID.Validate(); err != nil {
		return errors.Trace(err)
	}
	return domain.CoerceError(s.st.SetControllerDone(ctx, upgradeUUID, controllerID))
}

// SetDBUpgradeCompleted marks the upgrade as completed in the database
func (s *Service) SetDBUpgradeCompleted(ctx context.Context, upgradeUUID domainupgrade.UUID) error {
	if err := upgradeUUID.Validate(); err != nil {
		return errors.Trace(err)
	}
	err := s.st.SetDBUpgradeCompleted(ctx, upgradeUUID)
	return domain.CoerceError(err)
}

// SetDBUpgradeFailed marks the upgrade as completed in the database
func (s *Service) SetDBUpgradeFailed(ctx context.Context, upgradeUUID domainupgrade.UUID) error {
	if err := upgradeUUID.Validate(); err != nil {
		return errors.Trace(err)
	}
	err := s.st.SetDBUpgradeFailed(ctx, upgradeUUID)
	return domain.CoerceError(err)
}

// ActiveUpgrade returns the uuid of the current active upgrade.
// If there are no active upgrades, return a NotFound error
func (s *Service) ActiveUpgrade(ctx context.Context) (domainupgrade.UUID, error) {
	activeUpgrade, err := s.st.ActiveUpgrade(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.NotFoundf("active upgrade")
	}
	return activeUpgrade, domain.CoerceError(err)
}

// WatchForUpgradeReady creates a watcher which notifies when all controller
// nodes have been registered, meaning the upgrade is ready to start.
func (s *Service) WatchForUpgradeReady(ctx context.Context, upgradeUUID domainupgrade.UUID) (watcher.NotifyWatcher, error) {
	if err := upgradeUUID.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	mask := changestream.Create | changestream.Update
	predicate := func(ctx context.Context, db coredatabase.TxnRunner, changes []changestream.ChangeEvent) (bool, error) {
		return s.st.AllProvisionedControllersReady(ctx, upgradeUUID)
	}
	return s.watcherFactory.NewValuePredicateWatcher("upgrade_info_controller_node", upgradeUUID.String(), mask, predicate)
}

// WatchForUpgradeState creates a watcher which notifies when the upgrade
// has reached the given state.
func (s *Service) WatchForUpgradeState(ctx context.Context, upgradeUUID domainupgrade.UUID, state upgrade.State) (watcher.NotifyWatcher, error) {
	if err := upgradeUUID.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	mask := changestream.Create | changestream.Update
	predicate := func(ctx context.Context, db coredatabase.TxnRunner, changes []changestream.ChangeEvent) (bool, error) {
		info, err := s.st.UpgradeInfo(ctx, upgradeUUID)
		if err != nil {
			return false, errors.Trace(err)
		}
		return info.State == state, nil
	}
	return s.watcherFactory.NewValuePredicateWatcher("upgrade_info", upgradeUUID.String(), mask, predicate)
}

// UpgradeInfo returns the upgrade info for the supplied upgradeUUID
func (s *Service) UpgradeInfo(ctx context.Context, upgradeUUID domainupgrade.UUID) (upgrade.Info, error) {
	if err := upgradeUUID.Validate(); err != nil {
		return upgrade.Info{}, errors.Trace(err)
	}
	upgradeInfo, err := s.st.UpgradeInfo(ctx, upgradeUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return upgrade.Info{}, errors.NotFoundf("upgrade %q", upgradeUUID)
	}
	return upgradeInfo, domain.CoerceError(err)
}
