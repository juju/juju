// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"golang.org/x/net/context"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller/crossmodelsecrets"
	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/rpc/params"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SecretsManager", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewSecretManagerAPI(stdCtx, ctx)
	}, reflect.TypeOf((*SecretsManagerAPI)(nil)))
}

// NewSecretManagerAPI creates a SecretsManagerAPI.
func NewSecretManagerAPI(_ context.Context, ctx facade.ModelContext) (*SecretsManagerAPI, error) {
	if !ctx.Auth().AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}
	domainServices := ctx.DomainServices()
	leadershipChecker, err := ctx.LeadershipChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}

	backendService := domainServices.SecretBackend()
	secretService := domainServices.Secret()

	controllerAPI := common.NewControllerConfigAPI(
		ctx.State(),
		domainServices.ControllerConfig(),
		domainServices.ExternalController(),
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
		conn, err := apicaller.NewExternalControllerConnection(stdCtx, &apiInfo)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return crossmodelsecrets.NewClient(conn), nil
	}

	return &SecretsManagerAPI{
		authTag:              ctx.Auth().GetAuthTag(),
		authorizer:           ctx.Auth(),
		leadershipChecker:    leadershipChecker,
		watcherRegistry:      ctx.WatcherRegistry(),
		secretBackendService: backendService,
		secretService:        secretService,
		secretsTriggers:      secretService,
		secretsConsumer:      secretService,
		clock:                ctx.Clock(),
		controllerUUID:       ctx.ControllerUUID(),
		modelUUID:            ctx.ModelUUID().String(),
		remoteClientGetter:   remoteClientGetter,
		crossModelState:      commoncrossmodel.GetBackend(ctx.State()),
		logger:               ctx.Logger().Child("secretsmanager", corelogger.SECRETS),
	}, nil
}
