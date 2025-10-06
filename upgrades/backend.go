// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/caas"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// StateBackend provides an interface for upgrading the global state database.
type StateBackend interface {
	AddVirtualHostKeys() error
	SplitMigrationStatusMessages() error
	PopulateApplicationStorageUniqueID() error
}

// Model is an interface providing access to the details of a model within the
// controller.
type Model interface {
	Config() (*config.Config, error)
	CloudSpec() (environscloudspec.CloudSpec, error)
}

// NewStateBackend returns a new StateBackend using a *state.StatePool object.
func NewStateBackend(pool *state.StatePool) StateBackend {
	return stateBackend{
		pool: pool,
		getBrokerFunc: func(model *state.Model) (caas.Broker, error) {
			return stateenvirons.GetNewCAASBrokerFunc(caas.New)(model)
		},
	}
}

type stateBackend struct {
	pool          *state.StatePool
	getBrokerFunc func(model *state.Model) (caas.Broker, error)
}

// AddVirtualHostKeys runs an upgrade to
// create missing virtual host keys.
func (s stateBackend) AddVirtualHostKeys() error {
	return state.AddVirtualHostKeys(s.pool)
}

// SplitMigrationStatusMessages runs an upgrade to
// split migration status messages.
func (s stateBackend) SplitMigrationStatusMessages() error {
	return state.SplitMigrationStatusMessages(s.pool)
}

func (s stateBackend) PopulateApplicationStorageUniqueID() error {
	return state.PopulateApplicationStorageUniqueID(s.pool, s.getBrokerFunc)
}
