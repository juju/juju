// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Cloud", 7, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV7(ctx) // Do not set error if forcing credential update.
	}, reflect.TypeOf((*CloudAPI)(nil)))
}

// newFacadeV7 is used for API registration.
func newFacadeV7(context facade.Context) (*CloudAPI, error) {
	st := NewStateBackend(context.State())
	pool := NewModelPoolBackend(context.StatePool())
	systemState, err := pool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	serviceFactory := context.ServiceFactory()

	ctlrSt := NewStateBackend(systemState)
	return NewCloudAPI(
		st, ctlrSt, pool,
		serviceFactory.ControllerConfig(),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		context.Auth(), context.Logger().Child("cloud"),
	)
}
