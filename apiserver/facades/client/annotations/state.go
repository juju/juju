// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/state"
)

type annotationAccess interface {
	ModelTag() names.ModelTag
	FindEntity(tag names.Tag) (state.Entity, error)
	Annotations(entity state.GlobalEntity) (map[string]string, error)
	SetAnnotations(entity state.GlobalEntity, annotations map[string]string) error
}

// TODO - CAAS(externalreality): After all relevant methods are moved from
// state.State to state.Model this stateShim will likely embed only state.Model
// and will be renamed.
type stateShim struct {
	// TODO(menn0) - once FindEntity goes to Model, the embedded State
	// can go from here. The ModelTag method below can also go.
	*state.State
	*state.Model
}

func (s stateShim) ModelTag() names.ModelTag {
	return s.Model.ModelTag()
}
