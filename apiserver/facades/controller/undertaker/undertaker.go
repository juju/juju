// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/watcher"
)

// For testing.
var (
	GetProvider = provider.Provider
)

// UndertakerAPI implements the API used by the model undertaker worker.
type UndertakerAPI struct {
	*commonmodel.ModelConfigWatcher

	st        State
	resources facade.Resources

	modelUUID            coremodel.UUID
	modelProviderService ModelProviderService
	secretBackendService SecretBackendService
	modelInfoService     ModelInfoService
}

func newUndertakerAPI(
	modelUUID coremodel.UUID,
	st State,
	resources facade.Resources,
	authorizer facade.Authorizer,
	modelProviderService ModelProviderService,
	secretBackendService SecretBackendService,
	modelConfigService ModelConfigService,
	modelInfoService ModelInfoService,
	watcherRegistry facade.WatcherRegistry,
) (*UndertakerAPI, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	return &UndertakerAPI{
		st:                   st,
		resources:            resources,
		modelUUID:            modelUUID,
		secretBackendService: secretBackendService,
		modelInfoService:     modelInfoService,
		modelProviderService: modelProviderService,
		ModelConfigWatcher:   commonmodel.NewModelConfigWatcher(modelConfigService, watcherRegistry),
	}, nil
}

// CloudSpec returns the cloud spec used by the specified models.
func (u *UndertakerAPI) CloudSpec(ctx context.Context, args params.Entities) (params.CloudSpecResults, error) {
	results := params.CloudSpecResults{
		Results: make([]params.CloudSpecResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if tag.Id() != u.modelUUID.String() {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		spec, err := u.modelProviderService.GetCloudSpec(ctx)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = common.CloudSpecToParams(spec)
	}
	return results, nil
}

// ModelInfo returns information on the model needed by the undertaker worker.
func (u *UndertakerAPI) ModelInfo(ctx context.Context) (params.UndertakerModelInfoResult, error) {
	result := params.UndertakerModelInfoResult{}
	model, err := u.st.Model()
	if err != nil {
		return result, errors.Trace(err)
	}

	modelInfo, err := u.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Result = params.UndertakerModelInfo{
		UUID:           modelInfo.UUID.String(),
		Name:           modelInfo.Name,
		IsSystem:       u.st.IsController(),
		Life:           life.Value(model.Life().String()),
		ForceDestroyed: model.ForceDestroyed(),
		DestroyTimeout: model.DestroyTimeout(),
	}

	return result, nil
}

// ProcessDyingModel checks if a dying model has any machines or applications.
// If there are none, the model's life is changed from dying to dead.
func (u *UndertakerAPI) ProcessDyingModel(ctx context.Context) error {
	return u.st.ProcessDyingModel()
}

// RemoveModel removes any records of this model from Juju.
func (u *UndertakerAPI) RemoveModel(ctx context.Context) error {
	if err := u.removeModelSecrets(ctx); err != nil {
		return errors.Annotate(err, "removing model secrets")
	}
	return u.st.RemoveDyingModel()
}

// TODO(secret): all these logic should be moved to secret service.
func (u *UndertakerAPI) removeModelSecrets(ctx context.Context) error {
	modelInfo, err := u.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	secretBackendCfg, err := u.secretBackendService.GetSecretBackendConfigForAdmin(ctx, modelInfo.UUID)
	if errors.Is(err, secretbackenderrors.NotFound) || errors.Is(err, modelerrors.NotFound) {
		// If backends or settings are missing, then no secrets to remove.
		return nil
	}
	if err != nil {
		return errors.Annotate(err, "getting secrets backends config")
	}
	for _, cfg := range secretBackendCfg.Configs {
		if err := u.removeModelSecretsForBackend(ctx, &cfg); err != nil {
			return errors.Annotatef(err, "cleaning model from inactive secrets provider %q", cfg.BackendType)
		}
	}
	return nil
}

func (u *UndertakerAPI) removeModelSecretsForBackend(ctx context.Context, cfg *provider.ModelBackendConfig) error {
	p, err := GetProvider(cfg.BackendType)
	if err != nil {
		return errors.Trace(err)
	}
	return p.CleanupModel(ctx, cfg)
}

func (u *UndertakerAPI) modelEntitiesWatcher() params.NotifyWatchResult {
	var nothing params.NotifyWatchResult
	watch := u.st.WatchModelEntityReferences(u.st.ModelUUID())
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: u.resources.Register(watch),
		}
	}
	nothing.Error = apiservererrors.ServerError(watcher.EnsureErr(watch))
	return nothing
}

// WatchModelResources creates watchers for changes to the lifecycle of an
// model's machines and applications and storage.
func (u *UndertakerAPI) WatchModelResources(ctx context.Context) params.NotifyWatchResults {
	return params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			u.modelEntitiesWatcher(),
		},
	}
}

func (u *UndertakerAPI) modelWatcher() params.NotifyWatchResult {
	var nothing params.NotifyWatchResult
	model, err := u.st.Model()
	if err != nil {
		nothing.Error = apiservererrors.ServerError(err)
		return nothing
	}
	watch := model.Watch()
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: u.resources.Register(watch),
		}
	}
	nothing.Error = apiservererrors.ServerError(watcher.EnsureErr(watch))
	return nothing
}

// WatchModel creates a watcher for the current model.
func (u *UndertakerAPI) WatchModel(ctx context.Context) params.NotifyWatchResults {
	return params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			u.modelWatcher(),
		},
	}
}
