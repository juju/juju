// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spacenamer

import (
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

type spaceNamerStateShim struct {
	*state.State
}

func (s *spaceNamerStateShim) Model() (Model, error) {
	m, err := s.State.Model()
	if err != nil {
		return nil, err
	}
	return &modelShim{Model: m}, nil
}

func (s *spaceNamerStateShim) SpaceByID(id string) (Space, error) {
	sp, err := s.State.SpaceByID(id)
	if err != nil {
		return nil, err
	}
	return &spaceShim{Space: sp}, nil
}

type spaceShim struct {
	*state.Space
}

type modelShim struct {
	*state.Model
}

func (m *modelShim) Config() (Config, error) {
	cfg, err := m.Model.Config()
	if err != nil {
		return nil, err
	}
	return &configShim{Config: cfg}, nil
}

type configShim struct {
	*config.Config
}
