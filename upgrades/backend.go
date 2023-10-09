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
	EnsureInitalRefCountForExternalSecretBackends() error
	EnsureApplicationCharmOriginsHaveRevisions() error
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

func (s stateBackend) EnsureInitalRefCountForExternalSecretBackends() error {
	return state.EnsureInitalRefCountForExternalSecretBackends(s.pool)
}

func (s stateBackend) EnsureApplicationCharmOriginsHaveRevisions() error {
	return state.EnsureApplicationCharmOriginsHaveRevisions(s.pool)
}
