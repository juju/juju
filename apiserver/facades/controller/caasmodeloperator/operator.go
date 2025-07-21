// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.caasmodeloperator")

// TODO (manadart 2020-10-21): Remove the ModelUUID method
// from the next version of this facade.

// API represents the controller model operator facade.
type API struct {
	*common.APIAddresser
	*common.PasswordChanger

	auth      facade.Authorizer
	ctrlState CAASControllerState
	state     CAASModelOperatorState

	resources facade.Resources
}

// NewAPI is alternative means of constructing a controller model facade.
func NewAPI(
	authorizer facade.Authorizer,
	resources facade.Resources,
	ctrlSt CAASControllerState,
	st CAASModelOperatorState) (*API, error) {

	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	return &API{
		auth:            authorizer,
		APIAddresser:    common.NewAPIAddresser(ctrlSt, resources),
		PasswordChanger: common.NewPasswordChanger(st, common.AuthFuncForTagKind(names.ModelTagKind)),
		ctrlState:       ctrlSt,
		state:           st,
		resources:       resources,
	}, nil
}

// WatchModelOperatorProvisioningInfo provides a watcher for changes that affect the
// information returned by ModelOperatorProvisioningInfo.
func (a *API) WatchModelOperatorProvisioningInfo() (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}

	model, err := a.state.Model()
	if err != nil {
		return result, errors.Trace(err)
	}

	controllerConfigWatcher := a.ctrlState.WatchControllerConfig()
	controllerAPIHostPortsWatcher := a.ctrlState.WatchAPIHostPortsForAgents()
	modelConfigWatcher := model.WatchForModelConfigChanges()

	multiWatcher := common.NewMultiNotifyWatcher(controllerConfigWatcher, controllerAPIHostPortsWatcher, modelConfigWatcher)

	if _, ok := <-multiWatcher.Changes(); ok {
		result.NotifyWatcherId = a.resources.Register(multiWatcher)
	} else {
		return result, watcher.EnsureErr(multiWatcher)
	}

	return result, nil
}

// ModelOperatorProvisioningInfo returns the information needed for provisioning
// a new model operator into a caas cluster.
func (a *API) ModelOperatorProvisioningInfo() (params.ModelOperatorInfo, error) {
	logger.Infof("alvin2 ModelOperatorProvisioningInfo called")
	var result params.ModelOperatorInfo
	controllerConf, err := a.ctrlState.ControllerConfig()
	if err != nil {
		return result, err
	}

	model, err := a.state.Model()
	if err != nil {
		return result, errors.Trace(err)
	}
	modelConfig, err := model.ModelConfig()
	if err != nil {
		return result, errors.Trace(err)
	}

	vers, ok := modelConfig.AgentVersion()
	if !ok {
		return result, errors.NewNotValid(nil,
			fmt.Sprintf("agent version is missing in the model config %q",
				modelConfig.Name()))
	}

	apiAddresses, err := a.APIAddresses()
	if err != nil && apiAddresses.Error != nil {
		err = apiAddresses.Error
	}
	if err != nil {
		return result, errors.Annotate(err, "getting api addresses")
	}
	logger.Infof("alvin original modelConfig: %q", modelConfig)

	imageRepo, exists := modelConfig.CAASImageRepo()
	if !exists {
		imageRepo = controllerConf.CAASImageRepo()
		logger.Infof("alvin CAASImageRepo: %q", imageRepo)

		if imageRepo == "" {
			imageRepo = podcfg.JujudOCINamespace
		}

		if err := model.UpdateModelConfig(map[string]interface{}{config.ModelCAASImageRepo: imageRepo}, nil); err != nil {
			return result, errors.Trace(err)
		}
	}
	logger.Infof("alvin imageRepo: %q", imageRepo)
	logger.Infof("alvin later modelConfig: %q", modelConfig)

	imageRef, err := podcfg.GetJujuOCIImagePath(controllerConf, modelConfig, vers)
	if err != nil {
		return result, errors.Trace(err)
	}
	logger.Infof("alvin imageRef: %q", imageRef)

	imageRepoDetails, err := docker.NewImageRepoDetails(imageRepo)
	if err != nil {
		return result, errors.Annotatef(err, "parsing %s", imageRepo)
	}
	logger.Infof("alvin imageRepoDetails: %q", imageRepoDetails)

	imageInfo := params.NewDockerImageInfo(imageRepoDetails, imageRef)
	logger.Infof("alvin imageInfo: %q", imageInfo)

	logger.Tracef("image info %v", imageInfo)

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
func (a *API) ModelUUID() params.StringResult {
	return params.StringResult{Result: a.state.ModelUUID()}
}
