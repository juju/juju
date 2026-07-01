// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SSHSession", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return makeFacade(ctx)
	}, reflect.TypeFor[*Facade]())
}

// makeFacade constructs a new SSH session Facade from a model context.
func makeFacade(ctx facade.ModelContext) (*Facade, error) {
	authorizer := ctx.Auth()

	// Only machine agents consume SSH connection requests.
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}

	domainServices := ctx.DomainServices()
	return newFacade(
		domainServices.SSH(),
		domainServices.ControllerConfig(),
		domainServices.SSHServerHostKey(),
		ctx.WatcherRegistry(),
	), nil
}
