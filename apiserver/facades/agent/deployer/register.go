// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	applicationservice "github.com/juju/juju/domain/application/service"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/storage"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Deployer", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewDeployerFacade(ctx)
	}, reflect.TypeOf((*DeployerAPI)(nil)))
}

// NewDeployerFacade creates a new server-side DeployerAPI facade.
func NewDeployerFacade(ctx facade.ModelContext) (*DeployerAPI, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}

	st := ctx.State()
	resources := ctx.Resources()
	leadershipRevoker, err := ctx.LeadershipRevoker()
	if err != nil {
		return nil, errors.Annotate(err, "getting leadership client")
	}

	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	controllerConfigGetter := ctx.DomainServices().ControllerConfig()
	domainServices := ctx.DomainServices()
	backendService := domainServices.SecretBackend()
	secretService := domainServices.Secret(
		secretservice.SecretServiceParams{
			BackendAdminConfigGetter:      secretservice.NotImplementedBackendConfigGetter,
			BackendUserSecretConfigGetter: secretservice.NotImplementedBackendUserSecretConfigGetter,
		},
	)
	applicationService := ctx.DomainServices().Application(applicationservice.ApplicationServiceParams{
		StorageRegistry: storage.NotImplementedProviderRegistry{},
		BackendAdminConfigGetter: secretbackendservice.AdminBackendConfigGetterFunc(
			backendService, ctx.ModelUUID(),
		),
		SecretBackendReferenceDeleter: secretService,
	})

	return NewDeployerAPI(controllerConfigGetter, applicationService, authorizer, st, ctx.ObjectStore(), resources, leadershipRevoker, systemState)
}
