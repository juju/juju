// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/docker/registry"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Client", 8, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV8(ctx)
	}, reflect.TypeOf((*Client)(nil)))
}

// newFacadeV8 returns a new Client facade (v8).
func newFacadeV8(ctx facade.ModelContext) (*Client, error) {
	st := ctx.State()
	resources := ctx.Resources()
	authorizer := ctx.Auth()
	presence := ctx.Presence()

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	serviceFactory := ctx.ServiceFactory()

	configGetter := stateenvirons.EnvironConfigGetter{
		Model:             model,
		CloudService:      serviceFactory.Cloud(),
		CredentialService: serviceFactory.Credential(),
	}
	newEnviron := common.EnvironFuncForModel(model, serviceFactory.Cloud(), serviceFactory.Credential(), configGetter)

	leadershipReader, err := ctx.LeadershipReader()
	if err != nil {
		return nil, errors.Trace(err)
	}

	storageAccessor, err := getStorageState(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewClient(
		&stateShim{
			State:                    st,
			model:                    model,
			session:                  nil,
			configSchemaSourceGetter: environs.ProviderConfigSchemaSource(serviceFactory.Cloud()),
		},
		ctx.ServiceFactory().ModelInfo(),
		storageAccessor,
		serviceFactory.BlockDevice(),
		serviceFactory.ControllerConfig(),
		resources,
		authorizer,
		presence,
		newEnviron,
		common.NewBlockChecker(st),
		leadershipReader,
		ctx.ServiceFactory().Network(),
		registry.New,
	)
}
