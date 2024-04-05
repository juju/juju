// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"golang.org/x/net/context"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller/crossmodelsecrets"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/rpc/params"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SecretsManager", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewSecretManagerAPIV1(stdCtx, ctx)
	}, reflect.TypeOf((*SecretsManagerAPIV1)(nil)))
	registry.MustRegister("SecretsManager", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewSecretManagerAPI(stdCtx, ctx)
	}, reflect.TypeOf((*SecretsManagerAPI)(nil)))
}

// NewSecretManagerAPIV1 creates a SecretsManagerAPIV1.
// TODO - drop when we no longer support juju 3.1.x
func NewSecretManagerAPIV1(stdCtx context.Context, context facade.ModelContext) (*SecretsManagerAPIV1, error) {
	api, err := NewSecretManagerAPI(stdCtx, context)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &SecretsManagerAPIV1{SecretsManagerAPI: api}, nil
}

// NewSecretManagerAPI creates a SecretsManagerAPI.
func NewSecretManagerAPI(stdCtx context.Context, ctx facade.ModelContext) (*SecretsManagerAPI, error) {
	if !ctx.Auth().AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}
	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	serviceFactory := ctx.ServiceFactory()

	leadershipChecker, err := ctx.LeadershipChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudService := serviceFactory.Cloud()
	credentialSerivce := serviceFactory.Credential()

	backendService := serviceFactory.SecretBackend(model.ControllerUUID(), provider.Provider)
	secretBackendAdminConfigGetter := func(stdCtx context.Context) (*provider.ModelBackendConfigInfo, error) {
		return backendService.GetSecretBackendConfigForAdmin(stdCtx, coremodel.UUID(model.UUID()))
	}
	secretService := serviceFactory.Secret(secretBackendAdminConfigGetter)
	secretBackendConfigGetter := func(stdCtx context.Context, backendIDs []string, wantAll bool) (*provider.ModelBackendConfigInfo, error) {
		// TODO: this method in backend service is a TODO.
		return secrets.BackendConfigInfo(stdCtx, secrets.SecretsModel(model), true, secretService, cloudService, credentialSerivce, backendIDs, wantAll, ctx.Auth().GetAuthTag(), leadershipChecker)
	}
	secretBackendDrainConfigGetter := func(stdCtx context.Context, backendID string) (*provider.ModelBackendConfigInfo, error) {
		// TODO: this method in backend service is a TODO.
		return secrets.DrainBackendConfigInfo(stdCtx, backendID, secrets.SecretsModel(model), secretService, cloudService, credentialSerivce, ctx.Auth().GetAuthTag(), leadershipChecker)
	}
	controllerAPI := common.NewControllerConfigAPI(
		ctx.State(),
		serviceFactory.ControllerConfig(),
		serviceFactory.ExternalController(),
	)
	remoteClientGetter := func(stdCtx context.Context, uri *coresecrets.URI) (CrossModelSecretsClient, error) {
		info, err := controllerAPI.ControllerAPIInfoForModels(stdCtx, params.Entities{Entities: []params.Entity{{
			Tag: names.NewModelTag(uri.SourceUUID).String(),
		}}})
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(info.Results) < 1 {
			return nil, errors.Errorf("no controller api for model %q", uri.SourceUUID)
		}
		if err := info.Results[0].Error; err != nil {
			return nil, errors.Trace(err)
		}
		apiInfo := api.Info{
			Addrs:    info.Results[0].Addresses,
			CACert:   info.Results[0].CACert,
			ModelTag: names.NewModelTag(uri.SourceUUID),
		}
		apiInfo.Tag = names.NewUserTag(api.AnonymousUsername)
		conn, err := apicaller.NewExternalControllerConnection(&apiInfo)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return crossmodelsecrets.NewClient(conn), nil
	}

	return &SecretsManagerAPI{
		authTag:             ctx.Auth().GetAuthTag(),
		authorizer:          ctx.Auth(),
		leadershipChecker:   leadershipChecker,
		watcherRegistry:     ctx.WatcherRegistry(),
		secretService:       secretService,
		secretsTriggers:     secretService,
		secretsConsumer:     secretService,
		clock:               clock.WallClock,
		controllerUUID:      ctx.State().ControllerUUID(),
		modelUUID:           ctx.State().ModelUUID(),
		backendConfigGetter: secretBackendConfigGetter,
		drainConfigGetter:   secretBackendDrainConfigGetter,
		remoteClientGetter:  remoteClientGetter,
		crossModelState:     ctx.State().RemoteEntities(),
		logger:              ctx.Logger().ChildWithTags("secretsmanager", corelogger.SECRETS),
	}, nil
}
