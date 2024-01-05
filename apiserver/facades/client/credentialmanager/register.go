// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CredentialManager", 1, func(stdCtx context.Context, ctx facade.Context) (facade.Facade, error) {
		return NewCredentialManagerAPI(ctx)
	}, reflect.TypeOf((*CredentialManagerAPI)(nil)))
}

// NewCredentialManagerAPI creates a new CredentialManager API endpoint on server-side.
func NewCredentialManagerAPI(ctx facade.Context) (*CredentialManagerAPI, error) {
	if ctx.Auth().GetAuthTag() == nil || !ctx.Auth().AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	st, err := newStateShim(ctx.State())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return internalNewCredentialManagerAPI(
		st,
		ctx.ServiceFactory().Credential(),
		ctx.Resources(), ctx.Auth())
}
