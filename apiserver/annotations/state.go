// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
)

type annotationAccess interface {
	FindEntity(tag names.Tag) (state.Entity, error)
	GetAnnotations(entity state.GlobalEntity) (map[string]string, error)
	SetAnnotations(entity state.GlobalEntity, annotations map[string]string) error
	ModelTag() names.ModelTag
}

type stateShim struct {
	state *state.State
}

func (s stateShim) FindEntity(tag names.Tag) (state.Entity, error) {
	return s.state.FindEntity(tag)
}

func (s stateShim) GetAnnotations(entity state.GlobalEntity) (map[string]string, error) {
	return s.state.Annotations(entity)
}

func (s stateShim) SetAnnotations(entity state.GlobalEntity, annotations map[string]string) error {
	return s.state.SetAnnotations(entity, annotations)
}

func (s stateShim) ModelTag() names.ModelTag {
	return s.state.ModelTag()
}
