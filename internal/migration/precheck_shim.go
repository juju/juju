// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/errors"
	"github.com/juju/replicaset/v3"

	"github.com/juju/juju/state"
)

// PrecheckShim wraps a pair of *state.States to implement PrecheckBackend.
func PrecheckShim(modelState, controllerState *state.State) (PrecheckBackend, error) {
	return &precheckShim{
		State:           modelState,
		controllerState: controllerState,
	}, nil
}

// precheckShim is untested, but is simple enough to be verified by
// inspection.
type precheckShim struct {
	*state.State
	controllerState *state.State
}

// Model implements PrecheckBackend.
func (s *precheckShim) Model() (PrecheckModel, error) {
	model, err := s.State.Model()
	return model, errors.Trace(err)
}

// IsMigrationActive implements PrecheckBackend.
func (s *precheckShim) IsMigrationActive(modelUUID string) (bool, error) {
	return state.IsMigrationActive(s.State, modelUUID)
}

// AllMachines implements PrecheckBackend.
func (s *precheckShim) AllMachines() ([]PrecheckMachine, error) {
	machines, err := s.State.AllMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]PrecheckMachine, len(machines))
	for i, machine := range machines {
		out[i] = machine
	}
	return out, nil
}

// AllApplications implements PrecheckBackend.
func (s *precheckShim) AllApplications() ([]PrecheckApplication, error) {
	apps, err := s.State.AllApplications()
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]PrecheckApplication, len(apps))
	for i, app := range apps {
		out[i] = &precheckAppShim{app}
	}
	return out, nil
}

func (s *precheckShim) AllRelations() ([]PrecheckRelation, error) {
	rels, err := s.State.AllRelations()
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]PrecheckRelation, len(rels))
	for i, rel := range rels {
		out[i] = &precheckRelationShim{rel}
	}
	return out, nil
}

// ControllerBackend implements PrecheckBackend.
func (s *precheckShim) ControllerBackend() (PrecheckBackend, error) {
	return PrecheckShim(s.controllerState, s.controllerState)
}

func (s precheckShim) MongoCurrentStatus() (*replicaset.Status, error) {
	return nil, errors.NotImplementedf("this is not used but just for implementing the interface")
}

// PoolShim wraps a state.StatePool to produce a Pool.
func PoolShim(pool *state.StatePool) Pool {
	return &poolShim{pool}
}

type poolShim struct {
	pool *state.StatePool
}

func (p *poolShim) GetModel(uuid string) (PrecheckModel, func(), error) {
	model, ph, err := p.pool.GetModel(uuid)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return model, func() { ph.Release() }, nil
}

// precheckAppShim implements PrecheckApplication.
type precheckAppShim struct {
	*state.Application
}

// AllUnits implements PrecheckApplication.
func (s *precheckAppShim) AllUnits() ([]PrecheckUnit, error) {
	units, err := s.Application.AllUnits()
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]PrecheckUnit, len(units))
	for i, unit := range units {
		out[i] = unit
	}
	return out, nil
}

// precheckRelationShim implements PrecheckRelation.
type precheckRelationShim struct {
	*state.Relation
}

// Unit implements PreCheckRelation.
func (s *precheckRelationShim) Unit(pu PrecheckUnit) (PrecheckRelationUnit, error) {
	u, ok := pu.(*state.Unit)
	if !ok {
		return nil, errors.Errorf("got %T instead of *state.Unit", pu)
	}
	ru, err := s.Relation.Unit(u)
	return ru, errors.Trace(err)
}

// AllRemoteUnits implements PreCheckRelation.
func (s *precheckRelationShim) AllRemoteUnits(appName string) ([]PrecheckRelationUnit, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

// RemoteApplication implements PreCheckRelation.
func (s *precheckRelationShim) RemoteApplication() (string, bool, error) {
	// todo(gfouillet): cross model relations are disabled until backend
	//   functionality is moved to domain, so we just return false there until it
	//   is done.
	return "", false, nil
}
