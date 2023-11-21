// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// StateBackend provides an interface for upgrading the global state database.
type StateBackend interface {
	RemoveOrphanedSecretPermissions() error
	MigrateApplicationOpenedPortsToUnitScope() error
	EnsureInitalRefCountForExternalSecretBackends() error
	EnsureApplicationCharmOriginsNormalised() error
	FixOwnerConsumedSecretInfo() error
}

// Model is an interface providing access to the details of a model within the
// controller.
type Model interface {
	Config() (*config.Config, error)
	CloudSpec() (environscloudspec.CloudSpec, error)
}

// NewStateBackend returns a new StateBackend using a *state.StatePool object.
func NewStateBackend(pool *state.StatePool) StateBackend {
	return stateBackend{pool}
}

type stateBackend struct {
	pool *state.StatePool
}

func (s stateBackend) RemoveOrphanedSecretPermissions() error {
	return state.RemoveOrphanedSecretPermissions(s.pool)
}

func (s stateBackend) MigrateApplicationOpenedPortsToUnitScope() error {
	return state.MigrateApplicationOpenedPortsToUnitScope(s.pool)
}

func (s stateBackend) EnsureInitalRefCountForExternalSecretBackends() error {
	return state.EnsureInitalRefCountForExternalSecretBackends(s.pool)
}

func (s stateBackend) EnsureApplicationCharmOriginsNormalised() error {
	return state.EnsureApplicationCharmOriginsNormalised(s.pool)
}

func (s stateBackend) FixOwnerConsumedSecretInfo() error {
	return state.FixOwnerConsumedSecretInfo(s.pool)
}
