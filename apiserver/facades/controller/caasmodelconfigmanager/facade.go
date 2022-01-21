// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
)

// Facade allows model config manager clients to watch controller config changes and fetch controller config.
type Facade struct {
	auth                facade.Authorizer
	controllerConfigAPI *common.ControllerConfigAPI
}

// NewFacade creates a new authorized Facade.
func NewFacade(ctx facade.Context) (*Facade, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return &Facade{
		auth:                authorizer,
		controllerConfigAPI: common.NewStateControllerConfig(ctx.State()),
	}, nil
}

func (f *Facade) ControllerConfig() (params.ControllerConfigResult, error) {
	return f.controllerConfigAPI.ControllerConfig()
}
