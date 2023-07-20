// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crosscontroller

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain"
	ccservice "github.com/juju/juju/domain/controllerconfig/service"
	ccstate "github.com/juju/juju/domain/controllerconfig/state"
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
	cc := ccservice.NewService(
		ccstate.NewState(changestream.NewTxnRunnerFactory(ctx.ControllerDB)),
		domain.NewWatcherFactory(
			ctx.ControllerDB,
			ctx.Logger().Child("controllerconfig"),
		),
	)

	return NewCrossControllerAPI(
		ctx.Resources(),
		func() ([]string, string, error) {
			return common.StateControllerInfo(st, cc)
		},
		func() (string, error) {
			config, err := cc.ControllerConfig(context.TODO())
			if err != nil {
				return "", errors.Trace(err)
			}
			return config.PublicDNSAddress(), nil
		},
		st.WatchAPIHostPortsForClients,
	)
}
