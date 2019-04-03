// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import "github.com/juju/juju/state"

type modelGenerationStateShim struct {
	*state.State
}

func (m *modelGenerationStateShim) Model() (Model, error) {
	model, err := m.State.Model()
	if err != nil {
		return nil, err
	}
	return &generationModelShim{Model: model}, nil
}

func (m *modelGenerationStateShim) Application(name string) (Application, error) {
	return m.State.Application(name)
}

type generationModelShim struct {
	*state.Model
}

func (g *generationModelShim) Branch(name string) (Generation, error) {
	return g.Model.Branch(name)
}
