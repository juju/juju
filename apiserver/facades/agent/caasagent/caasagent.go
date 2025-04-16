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
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	corewatcher "github.com/juju/juju/core/watcher"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// StubService will be replaced once the implementation is finished.
type StubService interface {
	// CloudSpec returns the cloud spec for the model.
	CloudSpec(ctx context.Context) (environscloudspec.CloudSpec, error)
}

// ModelService represents the credential service provided by the
// provider.
type ModelService interface {
	WatchModelCloudCredential(ctx context.Context, modelUUID coremodel.UUID) (corewatcher.NotifyWatcher, error)
}

// ControllerConfigService is an interface that provides the controller
// configuration for the model.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// ExternalControllerService defines the methods that the controller
// facade needs from the controller state.
type ExternalControllerService interface {
	// ControllerForModel returns the controller record that's associated
	// with the modelUUID.
	ControllerForModel(ctx context.Context, modelUUID string) (*crossmodel.ControllerInfo, error)

	// UpdateExternalController persists the input controller
	// record.
	UpdateExternalController(ctx context.Context, ec crossmodel.ControllerInfo) error
}

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(ctx context.Context) (*config.Config, error)
	// Watch returns a watcher that returns keys for any changes to model
	// config.
	Watch() (corewatcher.StringsWatcher, error)
}

// ControllerConfigState defines the methods needed by
// ControllerConfigAPI
type ControllerConfigState interface {
	ModelExists(string) (bool, error)
	APIHostPortsForAgents(controller.Config) ([]network.SpaceHostPorts, error)
	CompletedMigrationForModel(string) (state.ModelMigration, error)
}

// NewFacadeV2 creates a new caas admission facade v2.
func NewFacadeV2(
	modelUUID coremodel.UUID,
	registry facade.WatcherRegistry,
	controllerConfigService ControllerConfigService,
	modelConfigService ModelConfigService,
	externalControllerService ExternalControllerService,
	controllerConfigState ControllerConfigState,
	cloudSpecGetter StubService,
	modelService ModelService,
) *FacadeV2 {
	return &FacadeV2{
		modelUUID:          modelUUID,
		registry:           registry,
		cloudSpecGetter:    cloudSpecGetter,
		modelService:       modelService,
		ModelConfigWatcher: commonmodel.NewModelConfigWatcher(modelConfigService, registry),
		ControllerConfigAPI: common.NewControllerConfigAPI(
			controllerConfigState,
			controllerConfigService,
			externalControllerService,
		),
	}
}

// FacadeV2 is the V2 facade of the caas agent.
type FacadeV2 struct {
	*commonmodel.ModelConfigWatcher
	*common.ControllerConfigAPI

	registry        facade.WatcherRegistry
	modelUUID       coremodel.UUID
	cloudSpecGetter StubService
	modelService    ModelService
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
		spec, err := f.cloudSpecGetter.CloudSpec(ctx)
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
		w, err := f.modelService.WatchModelCloudCredential(ctx, f.modelUUID)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		id, err := f.registry.Register(w)
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
