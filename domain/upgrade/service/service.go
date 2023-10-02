// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/database"
)

// State describes retrieval and persistence
// methods for upgrade info.
type State interface {
	CreateUpgrade(context.Context, version.Number, version.Number) (string, error)
	SetControllerReady(context.Context, string, string) error
	AllProvisionedControllersReady(context.Context, string) (bool, error)
	StartUpgrade(context.Context, string) error
	SetControllerDone(context.Context, string, string) error
	ActiveUpgrade(context.Context) (string, error)
	SetDBUpgradeCompleted(context.Context, string) error
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new namespace watcher
	// for events based on the input change mask.
	NewNamespaceWatcher(string, changestream.ChangeType, string) (watcher.StringsWatcher, error)
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
func (s *Service) CreateUpgrade(ctx context.Context, previousVersion, targetVersion version.Number) (string, error) {
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
func (s *Service) SetControllerReady(ctx context.Context, upgradeUUID, controllerID string) error {
	err := s.st.SetControllerReady(ctx, upgradeUUID, controllerID)
	if database.IsErrConstraintForeignKey(err) {
		return errors.NotFoundf("upgrade %q", upgradeUUID)
	}
	return domain.CoerceError(err)
}

// StartUpgrade starts the current upgrade if it exists
func (s *Service) StartUpgrade(ctx context.Context, upgradeUUID string) error {
	err := s.st.StartUpgrade(ctx, upgradeUUID)
	if database.IsErrNotFound(err) {
		return errors.NotFoundf("upgrade %q", upgradeUUID)
	}
	return domain.CoerceError(err)
}

// SetControllerDone marks the supplied controllerID as having
// completed its upgrades. When SetControllerDone is called by the
// last provisioned controller, the upgrade will be archived.
func (s *Service) SetControllerDone(ctx context.Context, upgradeUUID, controllerID string) error {
	return domain.CoerceError(s.st.SetControllerDone(ctx, upgradeUUID, controllerID))
}

// SetDBUpgradeCompleted marks the upgrade as completed in the database
func (s *Service) SetDBUpgradeCompleted(ctx context.Context, upgradeUUID string) error {
	err := s.st.SetDBUpgradeCompleted(ctx, upgradeUUID)
	return domain.CoerceError(err)
}

// ActiveUpgrade returns the uuid of the current active upgrade.
// If there are no active upgrades, return a NotFound error
func (s *Service) ActiveUpgrade(ctx context.Context) (string, error) {
	activeUpgrades, err := s.st.ActiveUpgrade(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.NotFoundf("active upgrade")
	}
	return activeUpgrades, domain.CoerceError(err)
}

// WatchForUpgradeReady creates a watcher which notifies when all controller
// nodes have been registered, meaning the upgrade is ready to start
func (s *Service) WatchForUpgradeReady(ctx context.Context, upgradeUUID string) (watcher.NotifyWatcher, error) {
	return NewUpgradeReadyWatcher(ctx, s.st, s.watcherFactory, upgradeUUID)
}
