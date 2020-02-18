// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
)

func NewInstanceMutaterAPIForTest(st InstanceMutaterState,
	model ModelCache,
	resources facade.Resources,
	authorizer facade.Authorizer,
	machineFunc EntityMachineFunc,
) (*InstanceMutaterAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthController() {
		return nil, common.ErrPerm
	}

	getAuthFunc := common.AuthFuncForMachineAgent(authorizer)
	return &InstanceMutaterAPI{
		LifeGetter:  common.NewLifeGetter(st, getAuthFunc),
		st:          st,
		model:       model,
		resources:   resources,
		authorizer:  authorizer,
		getAuthFunc: getAuthFunc,
		machineFunc: machineFunc,
	}, nil
}
