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
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/crossmodel"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/rpc/params"
)

// ControllerAPIInfoGetter returns controller API connection details for
// models. It is implemented by *common.ControllerConfigAPI.
type ControllerAPIInfoGetter interface {
	// ControllerAPIInfoForModels returns the controller api connection details for the specified models.
	ControllerAPIInfoForModels(ctx context.Context, args params.Entities) (params.ControllerAPIInfoResults, error)
}

var _ ControllerAPIInfoGetter = (*common.ControllerConfigAPI)(nil)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SecretsManager", 4, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewSecretManagerAPI(stdCtx, ctx)
	}, reflect.TypeFor[*SecretsManagerAPI]())
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
		domainServices.ControllerConfig(),
		domainServices.ControllerNode(),
		domainServices.ExternalController(),
		domainServices.Model(),
	)
	logger := ctx.Logger().Child("secretsmanager", corelogger.SECRETS)
	remoteClientGetter := newRemoteSecretsClientGetter(
		controllerAPI,
		domainServices.ExternalController(),
		apicaller.NewExternalControllerConnection,
		logger,
	)

	return &SecretsManagerAPI{
		authTag:                   ctx.Auth().GetAuthTag(),
		authorizer:                ctx.Auth(),
		leadershipChecker:         leadershipChecker,
		watcherRegistry:           ctx.WatcherRegistry(),
		secretBackendService:      backendService,
		secretService:             secretService,
		secretsTriggers:           secretService,
		secretsConsumer:           secretService,
		applicationService:        domainServices.Application(),
		crossModelRelationService: domainServices.CrossModelRelation(),
		clock:                     ctx.Clock(),
		controllerUUID:            ctx.ControllerUUID(),
		modelUUID:                 ctx.ModelUUID().String(),
		remoteClientGetter:        remoteClientGetter,
		logger:                    logger,
	}, nil
}

// newRemoteSecretsClientGetter returns a getter that connects to the
// controller hosting the source model of a cross-model secret. If that
// model has been migrated to another controller, the connection redirect
// is followed and the model's new controller location is persisted (best
// effort) so subsequent connections go directly to the new controller.
func newRemoteSecretsClientGetter(
	controllerAPI ControllerAPIInfoGetter,
	externalControllers crossmodel.ExternalControllerUpdater,
	openConnection apicaller.NewExternalControllerConnectionFunc,
	logger corelogger.Logger,
) func(ctx context.Context, uri *coresecrets.URI) (CrossModelSecretsClient, error) {
	return func(stdCtx context.Context, uri *coresecrets.URI) (CrossModelSecretsClient, error) {
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
		conn, redirect, err := crossmodel.ConnectWithRedirect(stdCtx, &apiInfo, openConnection)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if redirect != nil && redirect.ControllerTag.Id() != "" {
			// The source model was migrated to another controller; persist
			// its new location so future secret accesses connect directly.
			// Best effort: the connection is valid either way.
			if err := crossmodel.SaveMigratedModelController(
				stdCtx, externalControllers, redirect, uri.SourceUUID,
			); err != nil {
				logger.Infof(stdCtx, "failed to update external controller for model %s: %v", uri.SourceUUID, err)
			}
		}
		return crossmodelsecrets.NewClient(conn), nil
	}
}
