// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllercharm

import (
	"fmt"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs/bootstrap"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ControllerCharm", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newControllerCharmAPI(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newControllerCharmAPI creates a new server-side ControllerCharmAPI facade.
func newControllerCharmAPI(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}

	// Check this is the controller application
	unitName := authorizer.GetAuthTag().Id()
	applicationName, err := names.UnitApplication(unitName)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid unit name %s", unitName)
	}
	if applicationName != bootstrap.ControllerApplicationName {
		return nil, fmt.Errorf("application name should be %q, received %q",
			bootstrap.ControllerApplicationName, applicationName)
	}

	return &API{
		state: ctx.State(),
	}, nil
}
