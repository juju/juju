// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// NewExternalFacade is used for API registration.
func NewExternalFacade(st *state.State, res facade.Resources, auth facade.Authorizer) (*Facade, error) {
	return NewFacade(backendShim{st}, res, auth)
}

type backendShim struct {
	st *state.State
}

func (shim backendShim) ActionByTag(tag names.ActionTag) (state.Action, error) {
	m, err := shim.st.Model()
	if err != nil {
		return nil, err
	}

	return m.ActionByTag(tag)
}

func (shim backendShim) FindEntity(tag names.Tag) (state.Entity, error) {
	return shim.st.FindEntity(tag)
}

func (shim backendShim) TagToActionReceiverFn(findEntity func(names.Tag) (state.Entity, error)) func(string) (state.ActionReceiver, error) {
	return common.TagToActionReceiverFn(findEntity)
}

func (shim backendShim) ConvertActions(ar state.ActionReceiver, fn common.GetActionsFn) ([]params.ActionResult, error) {
	return common.ConvertActions(ar, fn, true)
}
