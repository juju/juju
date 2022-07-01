// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/v2/apiserver/common"
	apiservererrors "github.com/juju/juju/v2/apiserver/errors"
	"github.com/juju/juju/v2/apiserver/facade"
	"github.com/juju/names/v4"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
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
