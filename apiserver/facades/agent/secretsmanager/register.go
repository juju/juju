// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller/crossmodelsecrets"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
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
func NewSecretManagerAPI(context facade.Context) (*SecretsManagerAPI, error) {
	if !context.Auth().AuthUnitAgent() && !context.Auth().AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}
	model, err := context.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	leadershipChecker, err := context.LeadershipChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}
	secretBackendConfigGetter := func(backendIDs []string, wantAll bool) (*provider.ModelBackendConfigInfo, error) {
		return secrets.BackendConfigInfo(secrets.SecretsModel(model), true, backendIDs, wantAll, context.Auth().GetAuthTag(), leadershipChecker)
	}
	secretBackendAdminConfigGetter := func() (*provider.ModelBackendConfigInfo, error) {
		return secrets.AdminBackendConfigInfo(secrets.SecretsModel(model))
	}
	secretBackendDrainConfigGetter := func(backendID string) (*provider.ModelBackendConfigInfo, error) {
		return secrets.DrainBackendConfigInfo(backendID, secrets.SecretsModel(model), context.Auth().GetAuthTag(), leadershipChecker)
	}
	systemState, err := context.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerAPI := common.NewStateControllerConfig(systemState)
	remoteClientGetter := func(uri *coresecrets.URI) (CrossModelSecretsClient, error) {
		info, err := controllerAPI.ControllerAPIInfoForModels(params.Entities{Entities: []params.Entity{{
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
		authorizer:          context.Auth(),
		authTag:             context.Auth().GetAuthTag(),
		leadershipChecker:   leadershipChecker,
		secretsState:        state.NewSecrets(context.State()),
		resources:           context.Resources(),
		secretsTriggers:     context.State(),
		secretsConsumer:     context.State(),
		clock:               clock.WallClock,
		controllerUUID:      context.State().ControllerUUID(),
		modelUUID:           context.State().ModelUUID(),
		backendConfigGetter: secretBackendConfigGetter,
		adminConfigGetter:   secretBackendAdminConfigGetter,
		drainConfigGetter:   secretBackendDrainConfigGetter,
		remoteClientGetter:  remoteClientGetter,
		crossModelState:     context.State().RemoteEntities(),
	}, nil
}
