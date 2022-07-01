// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crosscontroller

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/v2/apiserver/common"
	"github.com/juju/juju/v2/apiserver/facade"
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
	return NewCrossControllerAPI(
		ctx.Resources(),
		func() ([]string, string, error) {
			return common.StateControllerInfo(st)
		},
		func() (string, error) {
			config, err := st.ControllerConfig()
			if err != nil {
				return "", errors.Trace(err)
			}
			return config.PublicDNSAddress(), nil
		},
		st.WatchAPIHostPortsForClients,
	)
}
