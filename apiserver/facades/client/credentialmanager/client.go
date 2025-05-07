// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager

import (
	"context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
)

type CredentialManagerAPI struct{}

// InvalidateModelCredential was removed as part of Juju 4.0 as there were no
// longer any client calls to this facade. There still does exist 3.6 clients
// that will call this facade from destroy and kill controller. As we don't
// support using 3.6 clients with 4.0 controllers to remove a controller we
// don't want this supported.
//
// This is left only to provide an error message to any would be callers.
func (*CredentialManagerAPI) InvalidateModelCredential(_ context.Context, _ string) error {
	return apiservererrors.ParamsErrorf(
		params.CodeNotImplemented, "Juju 4.0 does not support invalidating credentials via the client",
	)
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CredentialManager", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return &CredentialManagerAPI{}, nil
	}, reflect.TypeOf((*CredentialManagerAPI)(nil)))
}
