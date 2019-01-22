// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import "github.com/juju/juju/state"

type modelGenerationStateShim struct {
	*state.State
}

func (m *modelGenerationStateShim) Model() (GenerationModel, error) {
	model, err := m.State.Model()
	if err != nil {
		return nil, err
	}
	return &generationModelShim{Model: model}, nil
}

type generationModelShim struct {
	*state.Model
}

func (g *generationModelShim) NextGeneration() (Generation, error) {
	return g.Model.NextGeneration()
}
