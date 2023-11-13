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

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
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
