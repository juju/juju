// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	"github.com/juju/names/v5"
	"github.com/juju/replicaset/v3"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/tools"
)

// StatePool represents a point of use interface for getting the state from the
// pool.
type StatePool interface {
	MongoVersion() (string, error)
}

// State represents a point of use interface for modelling a current model.
type State interface {
	AllCharmURLs() ([]*string, error)
	HasUpgradeSeriesLocks() (bool, error)
	MachineCountForBase(base ...state.Base) (map[string]int, error)
	MongoCurrentStatus() (*replicaset.Status, error)
	AllMachines() ([]Machine, error)
}

// Machine represents a point of use interface for modelling a machine from
// state.
type Machine interface {
	Id() string
	AgentTools() (*tools.Tools, error)
	Life() state.Life
	Status() (status.StatusInfo, error)
	InstanceStatus() (status.StatusInfo, error)
	ShouldRebootOrShutdown() (state.RebootAction, error)
	Containers() ([]string, error)
}

// Model defines a point of use interface for the model from state.
type Model interface {
	Name() string
	Owner() names.UserTag
	AgentVersion() (version.Number, error)
	MigrationMode() state.MigrationMode
	Config() (*config.Config, error)
}
