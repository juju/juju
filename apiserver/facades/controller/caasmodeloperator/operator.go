// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/controller"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/rpc/params"
)

// TODO (manadart 2020-10-21): Remove the ModelUUID method
// from the next version of this facade.

// API represents the controller model operator facade.
type API struct {
	*common.APIAddresser
	*common.PasswordChanger

	auth                    facade.Authorizer
	ctrlState               CAASControllerState
	controllerConfigService ControllerConfigService
	modelConfigService      ModelConfigService
	logger                  corelogger.Logger

	resources facade.Resources
	modelUUID model.UUID
}

// NewAPI is alternative means of constructing a controller model facade.
func NewAPI(
	authorizer facade.Authorizer,
	resources facade.Resources,
	ctrlSt CAASControllerState,
	st CAASModelOperatorState,
	agentPasswordService AgentPasswordService,
	controllerConfigService ControllerConfigService,
	modelConfigService ModelConfigService,
	logger corelogger.Logger,
	modelUUID model.UUID,
) (*API, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	return &API{
		auth:                    authorizer,
		APIAddresser:            common.NewAPIAddresser(ctrlSt, resources),
		PasswordChanger:         common.NewPasswordChanger(agentPasswordService, st, common.AuthFuncForTagKind(names.ModelTagKind)),
		ctrlState:               ctrlSt,
		controllerConfigService: controllerConfigService,
		modelConfigService:      modelConfigService,
		logger:                  logger,
		resources:               resources,
		modelUUID:               modelUUID,
	}, nil
}

// WatchModelOperatorProvisioningInfo provides a watcher for changes that affect the
// information returned by ModelOperatorProvisioningInfo.
func (a *API) WatchModelOperatorProvisioningInfo(ctx context.Context) (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}

	controllerConfigWatcher, err := a.controllerConfigService.WatchControllerConfig()
	if err != nil {
		return result, errors.Trace(err)
	}
	controllerConfigNotifyWatcher, err := eventsource.NewStringsNotifyWatcher(controllerConfigWatcher)
	if err != nil {
		return result, errors.Trace(err)
	}

	controllerAPIHostPortsWatcher := a.ctrlState.WatchAPIHostPortsForAgents()

	modelConfigWatcher, err := a.modelConfigService.Watch()
	if err != nil {
		return result, errors.Trace(err)
	}
	modelConfigNotifyWatcher, err := eventsource.NewStringsNotifyWatcher(modelConfigWatcher)
	if err != nil {
		return result, errors.Trace(err)
	}

	multiWatcher, err := eventsource.NewMultiNotifyWatcher(ctx,
		controllerConfigNotifyWatcher,
		controllerAPIHostPortsWatcher,
		modelConfigNotifyWatcher,
	)

	if err != nil {
		return result, errors.Trace(err)
	}

	if _, err := internal.FirstResult[struct{}](ctx, multiWatcher); err != nil {
		return result, errors.Trace(err)
	}

	result.NotifyWatcherId = a.resources.Register(multiWatcher)
	return result, nil
}

// ModelOperatorProvisioningInfo returns the information needed for provisioning
// a new model operator into a caas cluster.
func (a *API) ModelOperatorProvisioningInfo(ctx context.Context) (params.ModelOperatorInfo, error) {
	var result params.ModelOperatorInfo
	controllerConfig, err := a.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return result, err
	}

	modelConfig, err := a.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	vers, ok := modelConfig.AgentVersion()
	if !ok {
		return result, errors.NewNotValid(nil,
			fmt.Sprintf("agent version is missing in the model config %q",
				modelConfig.Name()))
	}

	apiAddresses, err := a.APIAddresses(ctx)
	if err != nil && apiAddresses.Error != nil {
		err = apiAddresses.Error
	}
	if err != nil {
		return result, errors.Annotate(err, "getting api addresses")
	}

	registryPath, err := podcfg.GetJujuOCIImagePath(controllerConfig, vers)
	if err != nil {
		return result, errors.Trace(err)
	}

	imageRepoDetails, err := docker.NewImageRepoDetails(controllerConfig.CAASImageRepo())
	if err != nil {
		return result, errors.Annotatef(err, "parsing %s", controller.CAASImageRepo)
	}
	imageInfo := params.NewDockerImageInfo(docker.ConvertToResourceImageDetails(imageRepoDetails), registryPath)
	a.logger.Tracef(ctx, "image info %v", imageInfo)

	result = params.ModelOperatorInfo{
		APIAddresses: apiAddresses.Result,
		ImageDetails: imageInfo,
		Version:      vers,
	}
	return result, nil
}

// ModelUUID returns the model UUID that this facade is used to operate.
// It is implemented here directly as a result of removing it from
// embedded APIAddresser *without* bumping the facade version.
// It should be blanked when this facade version is next incremented.
func (a *API) ModelUUID(ctx context.Context) params.StringResult {
	return params.StringResult{Result: a.modelUUID.String()}
}

// APIHostPorts returns the API server addresses.
func (u *API) APIHostPorts(ctx context.Context) (result params.APIHostPortsResult, err error) {
	controllerConfig, err := u.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	return u.APIAddresser.APIHostPorts(ctx, controllerConfig)
}

// APIAddresses returns the list of addresses used to connect to the API.
func (u *API) APIAddresses(ctx context.Context) (result params.StringsResult, err error) {
	controllerConfig, err := u.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	return u.APIAddresser.APIAddresses(ctx, controllerConfig)
}
