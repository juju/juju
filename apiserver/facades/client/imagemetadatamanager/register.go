// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager

import (
	"context"
	"fmt"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ImageMetadataManager", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		api, err := makeAPI(stdCtx, ctx)
		if err != nil {
			return nil, fmt.Errorf("making ImageMetadataManager facade: %w", err)
		}

		return api, nil
	}, reflect.TypeOf((*API)(nil)))
}

// makeAPI is responsible for constructing a new [API] from the provided model
// context.
func makeAPI(ctx context.Context, modelctx facade.ModelContext) (*API, error) {
	authorizer := modelctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	controllerTag := names.NewControllerTag(modelctx.ControllerUUID())
	err := authorizer.HasPermission(ctx, permission.SuperuserAccess, controllerTag)
	if err != nil {
		return nil, err
	}

	domainServices := modelctx.DomainServices()
	if err != nil {
		return nil, errors.Trace(err)
	}
	newEnviron := func() (environs.Environ, error) {
		return stateenvirons.GetNewEnvironFunc(environs.New)(domainServices.ModelInfo(), domainServices.Cloud(), domainServices.Credential(), domainServices.Config())
	}
	return newAPI(
		domainServices.CloudImageMetadata(),
		domainServices.Config(),
		domainServices.ModelInfo(),
		newEnviron,
	), nil
}
