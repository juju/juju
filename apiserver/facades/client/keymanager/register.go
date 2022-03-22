// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/names/v4"
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
	registry.MustRegister("KeyManager", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newKeyManagerAPI(ctx)
	}, reflect.TypeOf((*KeyManagerAPI)(nil)))
}

// newKeyManagerAPI creates a new server-side keyupdater API end point.
func newKeyManagerAPI(ctx facade.Context) (*KeyManagerAPI, error) {
	// Only clients can access the key manager service.
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &KeyManagerAPI{
		state:      st,
		model:      m,
		resources:  ctx.Resources(),
		authorizer: authorizer,
		apiUser:    authorizer.GetAuthTag().(names.UserTag),
		check:      common.NewBlockChecker(st),
	}, nil
}
