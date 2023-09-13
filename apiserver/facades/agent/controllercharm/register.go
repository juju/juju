// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllercharm

import (
	"fmt"
	"reflect"

	"github.com/alecthomas/repr"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs/bootstrap"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ControllerCharm", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newAPI(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newAPI creates a new server-side controllercharm API facade.
func newAPI(ctx facade.Context) (*API, error) {
	if err := checkAuth(ctx.Auth()); err != nil {
		return nil, err
	}

	return &API{
		state: stateShim{ctx.State()},
	}, nil
}

// Check if the given client is authorized to access this facade.
func checkAuth(authorizer facade.Authorizer) error {
	// TODO: remove this
	loggo.GetLogger("juju.apiserver.controllercharm").Debugf("authorizer: %s", repr.String(authorizer))

	if !authorizer.AuthUnitAgent() {
		return apiservererrors.ErrPerm
	}

	// Check this is the controller application
	unitName := authorizer.GetAuthTag().Id()
	applicationName, err := names.UnitApplication(unitName)
	if err != nil {
		return errors.Annotatef(err, "invalid unit name %s", unitName)
	}
	if applicationName != bootstrap.ControllerApplicationName {
		return fmt.Errorf("application name should be %q, received %q",
			bootstrap.ControllerApplicationName, applicationName)
	}
	return nil
}
