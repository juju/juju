// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
)

type Facade struct {
	auth facade.Authorizer
	*common.ControllerConfigAPI
}

func NewStateFacade(ctx facade.Context) (*Facade, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}

	return &Facade{
		auth:                authorizer,
		ControllerConfigAPI: common.NewStateControllerConfig(ctx.State()),
	}, nil
}
