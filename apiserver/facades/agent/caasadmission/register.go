// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission

import (
	"reflect"

	jujuerrors "github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/facades"
)

// FacadesVersions returns the versions of the facades that this package
// implements.
func FacadesVersions() facades.NamedFacadeVersion {
	return facades.NamedFacadeVersion{
		Name:     "CAASAdmission",
		Versions: facades.FacadeVersion{1},
	}
}

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

	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}
	return &Facade{
		auth:                authorizer,
		ControllerConfigAPI: common.NewStateControllerConfig(systemState),
	}, nil
}
