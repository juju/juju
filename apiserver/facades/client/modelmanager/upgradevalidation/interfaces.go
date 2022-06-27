// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	// "github.com/juju/errors"
	// "github.com/juju/mgo/v2"
	"github.com/juju/names/v4"
	"github.com/juju/replicaset/v2"
	"github.com/juju/version/v2"

	"github.com/juju/juju/state"
)

// StatePool represents a point of use interface for getting the state from the
// pool.
type StatePool interface {
	// Get(string) (State, error)
	MongoVersion() (string, error)
}

// State represents a point of use interface for modelling a current model.
type State interface {
	// Model() (Model, error)
	HasUpgradeSeriesLocks() (bool, error)
	Release() bool
	AllModelUUIDs() ([]string, error)
	MachineCountForSeries(series ...string) (int, error)
	// MongoSession() MongoSession
	MongoCurrentStatus() (*replicaset.Status, error)
	SetModelAgentVersion(newVersion version.Number, stream *string, ignoreAgentVersions bool) error
	AbortCurrentUpgrade() error
}

// // MongoSession provides a way to get the status for the mongo replicaset.
// type MongoSession interface {
// 	CurrentStatus() (*replicaset.Status, error)
// }

// Model defines a point of use interface for the model from state.
type Model interface {
	IsControllerModel() bool
	AgentVersion() (version.Number, error)
	Owner() names.UserTag
	Name() string
	MigrationMode() state.MigrationMode
}

// type stateShim struct {
// 	*state.PooledState
// 	session MongoSession
// }

// func (s stateShim) Model() (Model, error) {
// 	model, err := s.PooledState.Model()
// 	if err != nil {
// 		return nil, errors.Trace(err)
// 	}
// 	return modelShim{
// 		Model: model,
// 	}, nil
// }

// func (s stateShim) MachineCountForSeries(series ...string) (int, error) {
// 	count, err := s.PooledState.MachineCountForSeries(series...)
// 	if err != nil {
// 		return 0, errors.Trace(err)
// 	}
// 	return count, nil
// }

// func (s stateShim) SetModelAgentVersion(newVersion version.Number, stream *string, ignoreAgentVersions bool) error {
// 	return s.PooledState.SetModelAgentVersion(newVersion, stream, ignoreAgentVersions)
// }

// func (s stateShim) AbortCurrentUpgrade() error {
// 	return s.PooledState.AbortCurrentUpgrade()
// }

// func (s stateShim) AllModelUUIDs() ([]string, error) {
// 	allModelUUIDs, err := s.PooledState.AllModelUUIDs()
// 	if err != nil {
// 		return nil, errors.Trace(err)
// 	}
// 	return allModelUUIDs, nil
// }

// func (s stateShim) MongoSession() MongoSession {
// 	if s.session == nil {
// 		s.session = mongoSessionShim{s.PooledState.MongoSession()}
// 	}
// 	return s.session
// }

// type modelShim struct {
// 	*state.Model
// }

// func (s modelShim) IsControllerModel() bool {
// 	return s.Model.IsControllerModel()
// }

// func (s modelShim) MigrationMode() state.MigrationMode {
// 	return s.Model.MigrationMode()
// }

// // mongoSessionShim wraps a *mgo.Session to conform to the
// // MongoSession interface.
// type mongoSessionShim struct {
// 	*mgo.Session
// }

// // CurrentStatus returns the current status of the replicaset.
// func (s mongoSessionShim) CurrentStatus() (*replicaset.Status, error) {
// 	return replicaset.CurrentStatus(s.Session)
// }
