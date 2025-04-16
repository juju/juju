// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent

import (
	"context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASAgent", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewFacadeV2AuthCheck(ctx)
	}, reflect.TypeOf((*FacadeV2)(nil)))
}

// NewFacadeV2AuthCheck provides the signature required for facade registration of
// caas agent v2.
func NewFacadeV2AuthCheck(ctx facade.ModelContext) (*FacadeV2, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthModelAgent() {
		return nil, apiservererrors.ErrPerm
	}

	domainServices := ctx.DomainServices()
	return NewFacadeV2(
		ctx.ModelUUID(),
		ctx.WatcherRegistry(),
		domainServices.ControllerConfig(),
		domainServices.Config(),
		domainServices.ExternalController(),
		ctx.State(),
		domainServices.Stub(),
		domainServices.Model(),
	), nil
}
