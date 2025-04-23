// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/errors"

	"github.com/juju/juju/state"
)

// instanceMutaterStateShim is used as a shim for state.State to enable better
// mock testing.
type instanceMutaterStateShim struct {
	*state.State
}

func (s *instanceMutaterStateShim) Application(appName string) (Application, error) {
	app, err := s.State.Application(appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &applicationShim{
		Application: app,
	}, nil
}

func (s *instanceMutaterStateShim) Unit(unitName string) (Unit, error) {
	unit, err := s.State.Unit(unitName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &unitShim{
		Unit: unit,
	}, nil
}

func (s instanceMutaterStateShim) Machine(machineId string) (Machine, error) {
	m, err := s.State.Machine(machineId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &machineShim{
		Machine: m,
	}, nil
}

// unitShim is used as a shim for a state Unit to enable better
// mock testing.
type unitShim struct {
	*state.Unit
}

func (u *unitShim) Application() (Application, error) {
	app, err := u.Unit.Application()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &applicationShim{app}, nil
}

// applicationShim is used as a shim for a state Application to enable better
// mock testing.
type applicationShim struct {
	*state.Application
}

func (a *applicationShim) CharmURL() *string {
	curl, _ := a.Application.CharmURL()
	return curl
}

// machineShim is used as a shim for a state Machine to enable better
// mock testing.
type machineShim struct {
	*state.Machine
}

func (m *machineShim) Units() ([]Unit, error) {
	units, err := m.Machine.Units()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]Unit, len(units))
	for k, v := range units {
		result[k] = &unitShim{
			Unit: v,
		}
	}
	return result, nil
}
