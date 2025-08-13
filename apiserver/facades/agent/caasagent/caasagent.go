// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	coremodel "github.com/juju/juju/core/model"
	corewatcher "github.com/juju/juju/core/watcher"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// ModelProviderService providers access to the model provider service.
type ModelProviderService interface {
	// GetCloudSpec returns the cloud spec for the model.
	GetCloudSpec(ctx context.Context) (environscloudspec.CloudSpec, error)
}

// ModelService defines a service used to watch a model's cloud and credential.
type ModelService interface {
	// WatchModelCloudCredential returns a new NotifyWatcher watching for changes that
	// result in the cloud spec for a model changing.
	WatchModelCloudCredential(ctx context.Context, modelUUID coremodel.UUID) (corewatcher.NotifyWatcher, error)
}

// NewFacadeV2 creates a new caas admission facade v2.
func NewFacadeV2(
	modelUUID coremodel.UUID,
	registry facade.WatcherRegistry,
	modelConfigWatcher *commonmodel.ModelConfigWatcher,
	controllerConfigAPI *common.ControllerConfigAPI,
	cloudSpecGetter ModelProviderService,
	modelCredentialWatcher func(stdCtx context.Context) (corewatcher.NotifyWatcher, error),
) *FacadeV2 {
	return &FacadeV2{
		modelUUID:              modelUUID,
		registry:               registry,
		cloudSpecGetter:        cloudSpecGetter,
		modelCredentialWatcher: modelCredentialWatcher,
		ModelConfigWatcher:     modelConfigWatcher,
		ControllerConfigAPI:    controllerConfigAPI,
	}
}

// FacadeV2 is the V2 facade of the caas agent.
type FacadeV2 struct {
	*commonmodel.ModelConfigWatcher
	*common.ControllerConfigAPI

	registry               facade.WatcherRegistry
	modelUUID              coremodel.UUID
	cloudSpecGetter        ModelProviderService
	modelCredentialWatcher func(stdCtx context.Context) (corewatcher.NotifyWatcher, error)
}

// CloudSpec returns the cloud spec used by the specified models.
func (f *FacadeV2) CloudSpec(ctx context.Context, args params.Entities) (params.CloudSpecResults, error) {
	results := params.CloudSpecResults{
		Results: make([]params.CloudSpecResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if tag.Id() != f.modelUUID.String() {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		spec, err := f.cloudSpecGetter.GetCloudSpec(ctx)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result := params.CloudSpecResult{
			Error: apiservererrors.ServerError(err),
		}
		if err == nil {
			result.Result = common.CloudSpecToParams(spec)
		}
		results.Results[i] = result
	}
	return results, nil
}

// WatchCloudSpecsChanges returns a watcher for cloud spec changes.
func (f *FacadeV2) WatchCloudSpecsChanges(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	gotWantedModel := false
	for i, arg := range args.Entities {
		tag, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if tag.Id() != f.modelUUID.String() {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		// This is being paranoid - the caller is expected to just pass in
		// the one entity arg with the model uuid to watch. In case they
		// pass it in more than once, we'll error for any duplicates.
		if gotWantedModel {
			dupeErr := errors.Errorf("duplicate model %q", f.modelUUID)
			results.Results[i].Error = apiservererrors.ServerError(dupeErr)
			continue
		}
		gotWantedModel = true
		w, err := f.modelCredentialWatcher(ctx)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		id, err := f.registry.Register(ctx, w)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// Consume the initial result for the API.
		_, err = internal.FirstResult[struct{}](ctx, w)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].NotifyWatcherId = id
	}
	return results, nil
}
