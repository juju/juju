// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v2"
	"github.com/juju/replicaset/v2"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/space"
	"github.com/juju/juju/state"
)

type spaceStateShim struct {
	common.ModelManagerBackend
}

func (s spaceStateShim) AllSpaces() ([]space.Space, error) {
	spaces, err := s.ModelManagerBackend.AllSpaces()
	if err != nil {
		return nil, errors.Trace(err)
	}

	results := make([]space.Space, len(spaces))
	for i, space := range spaces {
		results[i] = space
	}
	return results, nil
}

func (s spaceStateShim) AddSpace(name string, providerId network.Id, subnetIds []string, public bool) (space.Space, error) {
	result, err := s.ModelManagerBackend.AddSpace(name, providerId, subnetIds, public)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
}

func (s spaceStateShim) ConstraintsBySpaceName(name string) ([]space.Constraints, error) {
	constraints, err := s.ModelManagerBackend.ConstraintsBySpaceName(name)
	if err != nil {
		return nil, errors.Trace(err)
	}

	results := make([]space.Constraints, len(constraints))
	for i, constraint := range constraints {
		results[i] = constraint
	}
	return results, nil
}

type statePoolShim struct {
	*state.StatePool
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

func (s stateShim) MachineCountForSeries(series ...string) (int, error) {
	count, err := s.PooledState.MachineCountForSeries(series...)
	if err != nil {
		return 0, errors.Trace(err)
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

type modelShim struct {
	*state.Model
}

func (s modelShim) IsControllerModel() bool {
	return s.Model.IsControllerModel()
}

func (s modelShim) MigrationMode() state.MigrationMode {
	return s.Model.MigrationMode()
}
