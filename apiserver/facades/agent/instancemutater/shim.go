// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import "github.com/juju/juju/state"

type instanceMutaterStateShim struct {
	*state.State
}

func (i *instanceMutaterStateShim) Model() (Model, error) {
	model, err := i.State.Model()
	if err != nil {
		return nil, err
	}
	return &modelShim{Model: model}, nil
}

func (i *instanceMutaterStateShim) Unit(name string) (Unit, error) {
	unit, err := i.State.Unit(name)
	if err != nil {
		return nil, err
	}
	return &unitShim{Unit: unit}, nil
}

type modelShim struct {
	*state.Model
}

type unitShim struct {
	*state.Unit
}

func (u *unitShim) Application() (Application, error) {
	app, err := u.Unit.Application()
	if err != nil {
		return nil, err
	}
	return &applicationShim{Application: app}, nil
}

type applicationShim struct {
	*state.Application
}

func (a *applicationShim) Charm() (Charm, error) {
	ch, _, err := a.Application.Charm()
	if err != nil {
		return nil, err
	}
	return &charmShim{Charm: ch}, nil
}

type charmShim struct {
	*state.Charm
}

type machineShim struct {
	*state.Machine
}
