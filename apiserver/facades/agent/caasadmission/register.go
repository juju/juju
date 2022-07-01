// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission

import (
	"reflect"

	"github.com/juju/juju/v2/apiserver/common"
	"github.com/juju/juju/v2/apiserver/errors"
	"github.com/juju/juju/v2/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASAdmission", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

func newStateFacade(ctx facade.Context) (*Facade, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthMachineAgent() {
		return nil, errors.ErrPerm
	}

	return &Facade{
		auth:                authorizer,
		ControllerConfigAPI: common.NewStateControllerConfig(ctx.State()),
	}, nil
}
