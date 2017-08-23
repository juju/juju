// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/state"
)

// PrecheckShim wraps a *state.State to implement PrecheckBackend.
func PrecheckShim(st *state.State) (PrecheckBackend, error) {
	rSt, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &precheckShim{
		State:       st,
		resourcesSt: rSt,
	}, nil
}

// precheckShim is untested, but is simple enough to be verified by
// inspection.
type precheckShim struct {
	*state.State
	resourcesSt state.Resources
}

// Model implements PrecheckBackend.
func (s *precheckShim) Model() (PrecheckModel, error) {
	model, err := s.State.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return model, nil
}

// IsMigrationActive implements PrecheckBackend.
func (s *precheckShim) IsMigrationActive(modelUUID string) (bool, error) {
	return state.IsMigrationActive(s.State, modelUUID)
}

// AgentVersion implements PrecheckBackend.
func (s *precheckShim) AgentVersion() (version.Number, error) {
	cfg, err := s.State.ModelConfig()
	if err != nil {
		return version.Zero, errors.Trace(err)
	}
	vers, ok := cfg.AgentVersion()
	if !ok {
		return version.Zero, errors.New("no model agent version")
	}
	return vers, nil
}

// AllMachines implements PrecheckBackend.
func (s *precheckShim) AllMachines() ([]PrecheckMachine, error) {
	machines, err := s.State.AllMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]PrecheckMachine, 0, len(machines))
	for _, machine := range machines {
		out = append(out, machine)
	}
	return out, nil
}

// AllApplications implements PrecheckBackend.
func (s *precheckShim) AllApplications() ([]PrecheckApplication, error) {
	apps, err := s.State.AllApplications()
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]PrecheckApplication, 0, len(apps))
	for _, app := range apps {
		out = append(out, &precheckAppShim{app})
	}
	return out, nil
}

// ListPendingResources implements PrecheckBackend.
func (s *precheckShim) ListPendingResources(app string) ([]resource.Resource, error) {
	resources, err := s.resourcesSt.ListPendingResources(app)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return resources, nil
}

// ControllerBackend implements PrecheckBackend.
func (s *precheckShim) ControllerBackend() (PrecheckBackendCloser, error) {
	st, err := s.State.ForModel(s.State.ControllerModelTag())
	if err != nil {
		return nil, errors.Trace(err)
	}
	rSt, err := st.Resources()
	if err != nil {
		st.Close()
		return nil, errors.Trace(err)
	}
	return &precheckShim{
		State:       st,
		resourcesSt: rSt,
	}, nil
}

// PoolShim wraps a state.StatePool to produce a Pool.
func PoolShim(pool *state.StatePool) Pool {
	return &poolShim{pool}
}

type poolShim struct {
	pool *state.StatePool
}

func (p *poolShim) GetModel(uuid string) (PrecheckModel, func(), error) {
	model, release, err := p.pool.GetModel(uuid)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return model, func() { release() }, nil
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
	out := make([]PrecheckUnit, 0, len(units))
	for _, unit := range units {
		out = append(out, unit)
	}
	return out, nil
}
