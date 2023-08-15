// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	stdContext "context"
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller/crossmodelsecrets"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/apicaller"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SecretsManager", 1, func(ctx facade.Context) (facade.Facade, error) {
		return NewSecretManagerAPIV1(ctx)
	}, reflect.TypeOf((*SecretsManagerAPIV1)(nil)))
	registry.MustRegister("SecretsManager", 2, func(ctx facade.Context) (facade.Facade, error) {
		return NewSecretManagerAPI(ctx)
	}, reflect.TypeOf((*SecretsManagerAPI)(nil)))
}

// NewSecretManagerAPIV1 creates a SecretsManagerAPIV1.
// TODO - drop when we no longer support juju 3.1.x
func NewSecretManagerAPIV1(context facade.Context) (*SecretsManagerAPIV1, error) {
	api, err := NewSecretManagerAPI(context)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &SecretsManagerAPIV1{SecretsManagerAPI: api}, nil
}

// NewSecretManagerAPI creates a SecretsManagerAPI.
func NewSecretManagerAPI(ctx facade.Context) (*SecretsManagerAPI, error) {
	if !ctx.Auth().AuthUnitAgent() && !ctx.Auth().AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}
	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	leadershipChecker, err := ctx.LeadershipChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}
	secretBackendConfigGetter := func(backendIDs []string, wantAll bool) (*provider.ModelBackendConfigInfo, error) {
		return secrets.BackendConfigInfo(secrets.SecretsModel(model), backendIDs, wantAll, ctx.Auth().GetAuthTag(), leadershipChecker)
	}
	secretBackendAdminConfigGetter := func() (*provider.ModelBackendConfigInfo, error) {
		return secrets.AdminBackendConfigInfo(secrets.SecretsModel(model))
	}
	secretBackendDrainConfigGetter := func(backendID string) (*provider.ModelBackendConfigInfo, error) {
		return secrets.DrainBackendConfigInfo(backendID, secrets.SecretsModel(model), ctx.Auth().GetAuthTag(), leadershipChecker)
	}
	controllerAPI := common.NewControllerConfigAPI(
		ctx.State(),
		ctx.ServiceFactory().ExternalController(),
	)
	remoteClientGetter := func(uri *coresecrets.URI) (CrossModelSecretsClient, error) {
		info, err := controllerAPI.ControllerAPIInfoForModels(stdContext.TODO(), params.Entities{Entities: []params.Entity{{
			Tag: names.NewModelTag(uri.SourceUUID).String(),
		}}})
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(info.Results) < 1 {
			return nil, errors.Errorf("no controller api for model %q", uri.SourceUUID)
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
		secretsState:        state.NewSecrets(ctx.State()),
		watcherRegistry:     ctx.WatcherRegistry(),
		secretsTriggers:     ctx.State(),
		secretsConsumer:     ctx.State(),
		clock:               clock.WallClock,
		modelUUID:           ctx.State().ModelUUID(),
		backendConfigGetter: secretBackendConfigGetter,
		adminConfigGetter:   secretBackendAdminConfigGetter,
		drainConfigGetter:   secretBackendDrainConfigGetter,
		remoteClientGetter:  remoteClientGetter,
		crossModelState:     ctx.State().RemoteEntities(),
		logger:              ctx.Logger().ChildWithLabels("secretsmanager", corelogger.SECRETS),
	}, nil
}
