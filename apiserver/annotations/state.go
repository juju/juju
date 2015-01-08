// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
	"github.com/juju/juju/state"
	"github.com/juju/names"
)

type annotationAccess interface {
	FindEntity(tag names.Tag) (state.Entity, error)
	GetAnnotations(entity state.GlobalEntity) (map[string]string, error)
	SetAnnotations(entity state.GlobalEntity, annotations map[string]string) error
}

type stateShim struct {
	state *state.State
}

func (s stateShim) FindEntity(tag names.Tag) (state.Entity, error) {
	return s.state.FindEntity(tag)
}

func (s stateShim) GetAnnotations(entity state.GlobalEntity) (map[string]string, error) {
	return state.Annotations(entity, s.state)
}

func (s stateShim) SetAnnotations(entity state.GlobalEntity, annotations map[string]string) error {
	return state.SetAnnotations(entity, s.state, annotations)
}
