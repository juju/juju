// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Registry describes the API facades exposed by some API server.
type Registry interface {
	// MustRegister adds a single named facade at a given version to the
	// registry.
	// Factory will be called when someone wants to instantiate an object of
	// this facade, and facadeType defines the concrete type that the returned
	// object will be.
	// The Type information is used to define what methods will be exported in
	// the API, and it must exactly match the actual object returned by the
	// factory.
	MustRegister(string, int, facade.Factory, reflect.Type)
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry Registry) {
	registry.MustRegister("KeyUpdater", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newKeyUpdaterAPI(ctx)
	}, reflect.TypeOf((*KeyUpdaterAPI)(nil)))
}

// newKeyUpdaterAPI creates a new server-side keyupdater API end point.
func newKeyUpdaterAPI(ctx facade.Context) (*KeyUpdaterAPI, error) {
	authorizer := ctx.Auth()
	// Only machine agents have access to the keyupdater service.
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}
	// No-one else except the machine itself can only read a machine's own credentials.
	getCanRead := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &KeyUpdaterAPI{
		state:      st,
		model:      m,
		resources:  ctx.Resources(),
		authorizer: authorizer,
		getCanRead: getCanRead,
	}, nil
}
