// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crosscontroller

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CrossController", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateCrossControllerAPI(ctx)
	}, reflect.TypeOf((*CrossControllerAPI)(nil)))
}

// newStateCrossControllerAPI creates a new server-side CrossModelRelations API facade
// backed by global state.
func newStateCrossControllerAPI(ctx facade.Context) (*CrossControllerAPI, error) {
	st := ctx.State()
	serviceFactory := ctx.ServiceFactory()
	controllerConfigService := serviceFactory.ControllerConfig()

	return NewCrossControllerAPI(
		ctx.Resources(),
		func(ctx context.Context) ([]string, string, error) {
			controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
			if err != nil {
				return nil, "", errors.Trace(err)
			}
			return common.StateControllerInfo(st, controllerConfig)
		},
		func(ctx context.Context) (string, error) {
			controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
			if err != nil {
				return "", errors.Trace(err)
			}
			return controllerConfig.PublicDNSAddress(), nil
		},
		st.WatchAPIHostPortsForClients,
	)
}
