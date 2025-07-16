// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type backendShim struct {
	st *state.State
}

func (shim backendShim) ActionByTag(tag names.ActionTag) (state.Action, error) {
	return shim.st.ActionByTag(tag)
}

func (shim backendShim) TagToActionReceiverFn(findEntity func(names.Tag) (state.Entity, error)) func(string) (state.ActionReceiver, error) {
	return common.TagToActionReceiverFn(findEntity)
}

func (shim backendShim) ConvertActions(ar state.ActionReceiver, fn common.GetActionsFn) ([]params.ActionResult, error) {
	return common.ConvertActions(ar, fn)
}
