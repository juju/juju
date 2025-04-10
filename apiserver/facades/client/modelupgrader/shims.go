// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v6"
	"github.com/juju/replicaset/v3"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/state"
)

// StatePool represents a point of use interface for getting the state from the
// pool.
type StatePool interface {
	Get(string) (State, error)
}

// State represents a point of use interface for modelling a current model.
type State interface {
	Model() (Model, error)
	Release() bool
	AllModelUUIDs() ([]string, error)
	MachineCountForBase(base ...state.Base) (map[string]int, error)
	AllMachinesCount() (int, error)
	SetModelAgentVersion(newVersion semversion.Number, stream *string, ignoreAgentVersions bool, upgrader state.Upgrader) error
}

type SystemState interface {
	ControllerModel() (Model, error)
}

// Model defines a point of use interface for the model from state.
type Model interface {
	IsControllerModel() bool

	Owner() names.UserTag
	Name() string
	MigrationMode() state.MigrationMode
	Type() state.ModelType
	Life() state.Life
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

func (s stateShim) AllMachinesCount() (int, error) {
	count, err := s.PooledState.AllMachinesCount()
	if err != nil {
		return 0, errors.Trace(err)
	}
	return count, nil
}

func (s stateShim) SetModelAgentVersion(newVersion semversion.Number, stream *string, ignoreAgentVersions bool, upgrader state.Upgrader) error {
	return s.PooledState.SetModelAgentVersion(newVersion, stream, ignoreAgentVersions, upgrader)
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

type modelShim struct {
	*state.Model
}

func (s modelShim) IsControllerModel() bool {
	return s.Model.IsControllerModel()
}

func (s modelShim) MigrationMode() state.MigrationMode {
	return s.Model.MigrationMode()
}
