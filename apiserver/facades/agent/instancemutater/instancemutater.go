// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.apiserver.instancemutater")

type InstanceMutaterAPI struct {
	*common.ModelMachinesWatcher
	*common.UnitsWatcher

	st          InstanceMutaterState
	getAuthFunc common.GetAuthFunc
}

func NewInstanceMutaterFacade(ctx facade.Context) (*InstanceMutaterAPI, error) {
	st := &instanceMutaterShim{State: ctx.State()}
	return NewInstanceMutaterAPI(st, ctx.Resources(), ctx.Auth())
}

func NewInstanceMutaterAPI(st InstanceMutaterState, resources facade.Resources, authorizer facade.Authorizer) (*InstanceMutaterAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthController() {
		return nil, common.ErrPerm
	}
	//getAuthFunc := func() (common.AuthFunc, error) {
	//	isModelManager := authorizer.AuthController()
	//	isMachineAgent := authorizer.AuthMachineAgent()
	//	authEntityTag := authorizer.GetAuthTag()
	//
	//	return func(tag names.Tag) bool {
	//		if isMachineAgent && tag == authEntityTag {
	//			// A machine agent can always access its own machine.
	//			return true
	//		}
	//		switch tag := tag.(type) {
	//		case names.MachineTag:
	//			parentId := state.ParentId(tag.Id())
	//			if parentId == "" {
	//				// All top-level machines are accessible by the controller.
	//				return isModelManager
	//			}
	//			// All containers with the authenticated machine as a
	//			// parent are accessible by it.
	//			// TODO(dfc) sometimes authEntity tag is nil, which is fine because nil is
	//			// only equal to nil, but it suggests someone is passing an authorizer
	//			// with a nil tag.
	//			return isMachineAgent && names.NewMachineTag(parentId) == authEntityTag
	//		default:
	//			return false
	//		}
	//	}, nil
	//}
	getCanWatch := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	return &InstanceMutaterAPI{
		ModelMachinesWatcher: common.NewModelMachinesWatcher(st, resources, authorizer),
		UnitsWatcher:         common.NewUnitsWatcher(st, resources, getCanWatch),
		st:                   st,
		//getAuthFunc:          getAuthFunc,
	}, nil
}
