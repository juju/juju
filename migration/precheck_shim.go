// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/state"
)

// PrecheckShim wraps a *state.State to implement PrecheckBackend.
func PrecheckShim(st *state.State) PrecheckBackend {
	return &precheckShim{st}
}

// precheckShim is untested, but is simple enough to be verified by
// inspection.
type precheckShim struct {
	*state.State
}

// Model implements PrecheckBackend.
func (s *precheckShim) Model() (PrecheckModel, error) {
	model, err := s.State.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return model, nil
}

// AllModels implements PrecheckBackend.
func (s *precheckShim) AllModels() ([]PrecheckModel, error) {
	models, err := s.State.AllModels()
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]PrecheckModel, 0, len(models))
	for _, model := range models {
		out = append(out, model)
	}
	return out, nil
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

// ControllerBackend implements PrecheckBackend.
func (s *precheckShim) ControllerBackend() (PrecheckBackend, error) {
	model, err := s.State.ControllerModel()
	if err != nil {
		return nil, errors.Trace(err)
	}
	st, err := s.State.ForModel(model.ModelTag())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return PrecheckShim(st), nil
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
