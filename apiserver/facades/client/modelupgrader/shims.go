// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v5"
	"github.com/juju/replicaset/v3"
	"github.com/juju/version/v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// StatePool represents a point of use interface for getting the state from the
// pool.
type StatePool interface {
	Get(string) (State, error)
	ControllerModel() (Model, error)
	MongoVersion() (string, error)
}

// State represents a point of use interface for modelling a current model.
type State interface {
	Model() (Model, error)
	HasUpgradeSeriesLocks() (bool, error)
	Release() bool
	AllModelUUIDs() ([]string, error)
	MachineCountForBase(base ...state.Base) (map[string]int, error)
	MongoCurrentStatus() (*replicaset.Status, error)
	SetModelAgentVersion(newVersion version.Number, stream *string, ignoreAgentVersions bool) error
	AbortCurrentUpgrade() error
	ControllerConfig() (controller.Config, error)
	AllCharmURLs() ([]*string, error)
}

type SystemState interface {
	ControllerModel() (Model, error)
}

// Model defines a point of use interface for the model from state.
type Model interface {
	IsControllerModel() bool
	AgentVersion() (version.Number, error)
	Owner() names.UserTag
	Name() string
	MigrationMode() state.MigrationMode
	Type() state.ModelType
	Life() state.Life
	ModelTag() names.ModelTag
	ControllerUUID() string
	Config() (*config.Config, error)
	Cloud() (cloud.Cloud, error)
	CloudRegion() string
	CloudCredential() (state.Credential, bool, error)
}

type statePoolShim struct {
	*state.StatePool
}

func (s statePoolShim) ControllerModel() (Model, error) {
	st, err := s.StatePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelShim{
		Model: model,
	}, nil
}

func (s statePoolShim) Get(uuid string) (State, error) {
	st, err := s.StatePool.Get(uuid)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return stateShim{
		PooledState: st,
	}, nil
}

func (s statePoolShim) MongoVersion() (string, error) {
	st, err := s.StatePool.SystemState()
	if err != nil {
		return "", errors.Trace(err)
	}
	return st.MongoVersion()
}

type stateShim struct {
	*state.PooledState
	mgosession *mgo.Session
}

func (s stateShim) Model() (Model, error) {
	model, err := s.PooledState.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelShim{
		Model: model,
	}, nil
}

func (s stateShim) MachineCountForBase(base ...state.Base) (map[string]int, error) {
	count, err := s.PooledState.MachineCountForBase(base...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return count, nil
}

func (s stateShim) SetModelAgentVersion(newVersion version.Number, stream *string, ignoreAgentVersions bool) error {
	return s.PooledState.SetModelAgentVersion(newVersion, stream, ignoreAgentVersions)
}

func (s stateShim) AbortCurrentUpgrade() error {
	return s.PooledState.AbortCurrentUpgrade()
}

func (s stateShim) AllModelUUIDs() ([]string, error) {
	allModelUUIDs, err := s.PooledState.AllModelUUIDs()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return allModelUUIDs, nil
}

func (s stateShim) MongoCurrentStatus() (*replicaset.Status, error) {
	if s.mgosession == nil {
		s.mgosession = s.PooledState.MongoSession()
	}
	return replicaset.CurrentStatus(s.mgosession)
}

func (s stateShim) AllCharmURLs() ([]*string, error) {
	return s.PooledState.AllCharmURLs()
}

type modelShim struct {
	*state.Model
}

func (s modelShim) IsControllerModel() bool {
	return s.Model.IsControllerModel()
}

func (s modelShim) MigrationMode() state.MigrationMode {
	return s.Model.MigrationMode()
}
