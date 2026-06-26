// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"context"

	"github.com/juju/errors"

	modeloperatorapi "github.com/juju/juju/api/controller/caasmodeloperator"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/docker"
)

// ControllerConfigService provides access to controller configuration.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
	WatchControllerConfig(context.Context) (watcher.StringsWatcher, error)
}

// ModelConfigService provides access to model configuration.
type ModelConfigService interface {
	ModelConfig(context.Context) (*config.Config, error)
	Watch(context.Context) (watcher.StringsWatcher, error)
}

// ControllerNodeService provides access to controller node information.
type ControllerNodeService interface {
	GetAllAPIAddressesForAgents(ctx context.Context) ([]string, error)
	WatchControllerAPIAddresses(context.Context) (watcher.NotifyWatcher, error)
}

// ControllerService provides access to controller information.
type ControllerService interface {
	GetControllerAgentInfo(ctx context.Context) (controller.ControllerAgentInfo, error)
}

// AgentPasswordService provides access to agent password operations.
type AgentPasswordService interface {
	SetModelPassword(ctx context.Context, password string) error
}

// modelOperatorAPIAdapter implements ModelOperatorAPI using domain services.
type modelOperatorAPIAdapter struct {
	ctrlConfigSvc  ControllerConfigService
	modelConfigSvc ModelConfigService
	ctrlNodeSvc    ControllerNodeService
	ctrlSvc        ControllerService
	agentPwdSvc    AgentPasswordService
}

// SetPassword implements ModelOperatorAPI.
func (a *modelOperatorAPIAdapter) SetPassword(ctx context.Context, password string) error {
	return a.agentPwdSvc.SetModelPassword(ctx, password)
}

// ModelOperatorProvisioningInfo implements ModelOperatorAPI.
func (a *modelOperatorAPIAdapter) ModelOperatorProvisioningInfo(ctx context.Context) (modeloperatorapi.ModelOperatorProvisioningInfo, error) {
	controllerConfig, err := a.ctrlConfigSvc.ControllerConfig(ctx)
	if err != nil {
		return modeloperatorapi.ModelOperatorProvisioningInfo{}, errors.Annotate(err, "getting controller config")
	}

	modelConfig, err := a.modelConfigSvc.ModelConfig(ctx)
	if err != nil {
		return modeloperatorapi.ModelOperatorProvisioningInfo{}, errors.Annotate(err, "getting model config")
	}

	vers, ok := modelConfig.AgentVersion()
	if !ok {
		return modeloperatorapi.ModelOperatorProvisioningInfo{}, errors.New("agent version not set in model config")
	}

	apiAddresses, err := a.ctrlNodeSvc.GetAllAPIAddressesForAgents(ctx)
	if err != nil {
		return modeloperatorapi.ModelOperatorProvisioningInfo{}, errors.Annotate(err, "getting API addresses")
	}

	controllerAgentInfo, err := a.ctrlSvc.GetControllerAgentInfo(ctx)
	if err != nil {
		return modeloperatorapi.ModelOperatorProvisioningInfo{}, errors.Annotate(err, "getting controller agent info")
	}

	registryPath, err := podcfg.GetJujuOCIImagePathFromControllerCfg(controllerConfig, vers)
	if err != nil {
		return modeloperatorapi.ModelOperatorProvisioningInfo{}, errors.Annotate(err, "getting OCI image path")
	}

	imageRepoDetails, err := docker.NewImageRepoDetails(controllerConfig.CAASImageRepo())
	if err != nil {
		return modeloperatorapi.ModelOperatorProvisioningInfo{}, errors.Annotate(err, "parsing image repo details")
	}

	imageDetails := convertToDockerImageDetails(docker.ConvertToResourceImageDetails(imageRepoDetails), registryPath)

	return modeloperatorapi.ModelOperatorProvisioningInfo{
		APIAddresses:         apiAddresses,
		ImageDetails:         imageDetails,
		Version:              vers,
		ControllerCert:       controllerAgentInfo.Cert,
		ControllerPrivateKey: controllerAgentInfo.PrivateKey,
		CAPrivateKey:         controllerAgentInfo.CAPrivateKey,
	}, nil
}

// WatchModelOperatorProvisioningInfo implements ModelOperatorAPI.
func (a *modelOperatorAPIAdapter) WatchModelOperatorProvisioningInfo(ctx context.Context) (watcher.NotifyWatcher, error) {
	controllerConfigWatcher, err := a.ctrlConfigSvc.WatchControllerConfig(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "watching controller config")
	}
	controllerConfigNotifyWatcher, err := eventsource.NewStringsNotifyWatcher(controllerConfigWatcher)
	if err != nil {
		return nil, errors.Annotate(err, "creating controller config notify watcher")
	}

	controllerAPIHostPortsWatcher, err := a.ctrlNodeSvc.WatchControllerAPIAddresses(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "watching controller API addresses")
	}

	modelConfigWatcher, err := a.modelConfigSvc.Watch(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "watching model config")
	}
	modelConfigNotifyWatcher, err := eventsource.NewStringsNotifyWatcher(modelConfigWatcher)
	if err != nil {
		return nil, errors.Annotate(err, "creating model config notify watcher")
	}

	multiWatcher, err := eventsource.NewMultiNotifyWatcher(ctx,
		controllerConfigNotifyWatcher,
		controllerAPIHostPortsWatcher,
		modelConfigNotifyWatcher,
	)
	if err != nil {
		return nil, errors.Annotate(err, "creating multi watcher")
	}

	return multiWatcher, nil
}

// convertToDockerImageDetails converts resource image repo details to
// DockerImageDetails with the given registry path.
func convertToDockerImageDetails(repoDetails resource.ImageRepoDetails, registryPath string) resource.DockerImageDetails {
	return resource.DockerImageDetails{
		RegistryPath:     registryPath,
		ImageRepoDetails: repoDetails,
	}
}
