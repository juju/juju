// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

type EntityStatusSetter struct {
	*common.StatusSetter
}

type EntityEntityFinder struct {
	st state.EntityFinder
}

func (e *EntityEntityFinder) FindEntity(tag names.Tag) (state.Entity, error) {
	entity, err := e.st.FindEntity(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	_, ok := tag.(names.UnitTag)
	if !ok {
		return entity, err
	}
	unit, ok := entity.(*state.Unit)
	if !ok {
		return nil, errors.Errorf("cannot use %T as unit", entity)
	}
	return unit.Agent(), nil
}

// NewEntityStatusSetter returns a new StatusSetter. The GetAuthFunc will be
// used on each invocation of SetStatus to determine current
// permissions.
func NewEntityStatusSetter(st state.EntityFinder, getCanModify common.GetAuthFunc) *EntityStatusSetter {
	//var _ state.EntityFinder = st
	finder := &EntityEntityFinder{st: st}
	statusSetter := common.NewStatusSetter(finder, getCanModify)
	return &EntityStatusSetter{statusSetter}
}
