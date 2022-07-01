// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	"reflect"

	"github.com/juju/juju/v2/apiserver/common"
	apiservererrors "github.com/juju/juju/v2/apiserver/errors"
	"github.com/juju/juju/v2/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASModelConfigManager", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newFacade creates a new authorized Facade.
func newFacade(ctx facade.Context) (*Facade, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return &Facade{
		auth:                authorizer,
		controllerConfigAPI: common.NewStateControllerConfig(ctx.State()),
	}, nil
}
