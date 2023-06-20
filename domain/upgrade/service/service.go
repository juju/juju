// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/juju/database"
	"github.com/juju/juju/domain"
	"github.com/juju/version/v2"
)

// State describes retrieval and persistence
// methods for upgrade info.
type State interface {
	CreateUpgrade(context.Context, version.Number, version.Number) (string, error)
	SetControllerReady(context.Context, string, string) error
	AllProvisionedControllersReady(context.Context, string) (bool, error)
	StartUpgrade(context.Context, string) error
	SetControllerDone(context.Context, string, string) error
	ActiveUpgrades(context.Context) ([]string, error)
}

// Service provides the API for working with upgrade info
type Service struct {
	st State
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State) *Service {
	return &Service{st}
}

// CreateUpgrade creates an upgrade to and from specified versions
// If an upgrade is already running/pending, return an error
func (s *Service) CreateUpgrade(ctx context.Context, previousVersion, targetVersion version.Number) (string, error) {
	if previousVersion.Compare(targetVersion) >= 0 {
		return "", errors.NotValidf("target version %q must be greater than current version %q", targetVersion, previousVersion)
	}
	upgradeUUID, err := s.st.CreateUpgrade(ctx, previousVersion, targetVersion)
	return upgradeUUID, domain.CoerceError(err)
}

// SetControllerReady marks the supplied controllerID as being ready
// to start it's upgrade. All provisioned controllers need to be ready
// before an upgrade can start
func (s *Service) SetControllerReady(ctx context.Context, upgradeUUID, controllerID string) error {
	err := s.st.SetControllerReady(ctx, upgradeUUID, controllerID)
	if database.IsErrConstraintForeignKey(err) {
		return errors.NotFoundf("upgrade %q", upgradeUUID)
	}
	return domain.CoerceError(err)
}

// AllProvisionedControllersReady returns true if and only if all controllers
// that have been started by the provisioner have called EnsureUpgradeInfo with
// matching versions.
func (s *Service) AllProvisionedControllersReady(ctx context.Context, upgradeUUID string) (bool, error) {
	allProvisioned, err := s.st.AllProvisionedControllersReady(ctx, upgradeUUID)
	return allProvisioned, domain.CoerceError(err)
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

// IsUpgrading returns true if an upgrade is currently in progress.
func (s *Service) ActiveUpgrades(ctx context.Context) ([]string, error) {
	activeUpgrades, err := s.st.ActiveUpgrades(ctx)
	return activeUpgrades, domain.CoerceError(err)
}
