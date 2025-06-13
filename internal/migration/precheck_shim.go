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
	return modelShim{model}, errors.Trace(err)
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
	return modelShim{model}, func() { ph.Release() }, nil
}

type modelShim struct {
	*state.Model
}

func (m modelShim) MigrationMode() (state.MigrationMode, error) {
	return m.State().MigrationMode()
}
