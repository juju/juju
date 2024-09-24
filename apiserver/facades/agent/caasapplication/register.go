// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	applicationservice "github.com/juju/juju/domain/application/service"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASApplication", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStateFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newStateFacade provides the signature required for facade registration.
func newStateFacade(ctx facade.ModelContext) (*Facade, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	domainServices := ctx.DomainServices()

	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(model, domainServices.Cloud(), domainServices.Credential(), domainServices.Config())
	if err != nil {
		return nil, errors.Annotate(err, "getting caas client")
	}
	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	registry := stateenvirons.NewStorageProviderRegistry(broker)
	backendService := domainServices.SecretBackend()
	applicationService := domainServices.Application(applicationservice.ApplicationServiceParams{
		StorageRegistry: registry,
		Secrets: domainServices.Secret(
			secretservice.SecretServiceParams{
				BackendAdminConfigGetter: secretbackendservice.AdminBackendConfigGetterFunc(
					backendService, ctx.ModelUUID(),
				),
				BackendUserSecretConfigGetter: secretbackendservice.UserSecretBackendConfigGetterFunc(
					backendService, ctx.ModelUUID(),
				),
			},
		),
	})

	return NewFacade(
		resources,
		authorizer,
		systemState,
		&stateShim{State: st},
		domainServices.ControllerConfig(),
		applicationService,
		domainServices.Config(),
		broker,
		ctx.StatePool().Clock(),
		ctx.Logger().Child("caasapplication"),
	)
}
