// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"context"

	"github.com/juju/errors"

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
	// ControllerConfig returns the current controller configuration.
	ControllerConfig(context.Context) (controller.Config, error)
	// WatchControllerConfig returns a watcher that observes changes to
	// controller configuration.
	WatchControllerConfig(context.Context) (watcher.StringsWatcher, error)
}

// ModelConfigService provides access to model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current model configuration.
	ModelConfig(context.Context) (*config.Config, error)
	// Watch returns a watcher that observes changes to model configuration.
	Watch(context.Context) (watcher.StringsWatcher, error)
}

// ControllerNodeService provides access to controller node information.
type ControllerNodeService interface {
	// GetAllAPIAddressesForAgents returns all API addresses available for
	// agents.
	GetAllAPIAddressesForAgents(ctx context.Context) ([]string, error)
	// WatchControllerAPIAddresses returns a watcher that observes changes to
	// controller API addresses.
	WatchControllerAPIAddresses(context.Context) (watcher.NotifyWatcher, error)
}

// ControllerService provides access to controller information.
type ControllerService interface {
	// GetControllerAgentInfo returns the controller agent certificate and key
	// information.
	GetControllerAgentInfo(ctx context.Context) (controller.ControllerAgentInfo, error)
}

// AgentPasswordService provides access to agent password operations.
type AgentPasswordService interface {
	// SetModelPassword sets the password for the model agent.
	SetModelPassword(ctx context.Context, password string) error
}

// modelOperatorAPIAdapter implements ModelOperatorAPI using domain services.
type modelOperatorAPIAdapter struct {
	// ctrlConfigSvc is used to retrieve the controller configuration, which
	// contains the CAAS image repository and agent version details needed to
	// build the OCI image path for the model operator. It is also watched for
	// changes that may affect provisioning.
	ctrlConfigSvc ControllerConfigService

	// modelConfigSvc is used to retrieve the model configuration, which provides
	// the agent version required to determine the correct OCI image tag. It is
	// also watched for changes.
	modelConfigSvc ModelConfigService

	// ctrlNodeSvc is used to retrieve the API addresses that agents use to
	// connect to the controller, and to watch for changes to those addresses so
	// the model operator can be updated.
	ctrlNodeSvc ControllerNodeService

	// ctrlSvc is used to retrieve the controller agent certificate, private key,
	// and CA private key, which the model operator needs to establish secure TLS
	// connections to the controller.
	ctrlSvc ControllerService

	// agentPwdSvc is used to set the model agent password, which is required for
	// the model operator to authenticate with the controller.
	agentPwdSvc AgentPasswordService
}

// SetPassword implements ModelOperatorAPI.
func (a *modelOperatorAPIAdapter) SetPassword(ctx context.Context, password string) error {
	return a.agentPwdSvc.SetModelPassword(ctx, password)
}

// ModelOperatorProvisioningInfo implements ModelOperatorAPI.
func (a *modelOperatorAPIAdapter) ModelOperatorProvisioningInfo(ctx context.Context) (ModelOperatorProvisioningInfo, error) {
	controllerConfig, err := a.ctrlConfigSvc.ControllerConfig(ctx)
	if err != nil {
		return ModelOperatorProvisioningInfo{}, errors.Annotate(err, "getting controller config")
	}

	modelConfig, err := a.modelConfigSvc.ModelConfig(ctx)
	if err != nil {
		return ModelOperatorProvisioningInfo{}, errors.Annotate(err, "getting model config")
	}

	vers, ok := modelConfig.AgentVersion()
	if !ok {
		return ModelOperatorProvisioningInfo{}, errors.New("agent version not set in model config")
	}

	apiAddresses, err := a.ctrlNodeSvc.GetAllAPIAddressesForAgents(ctx)
	if err != nil {
		return ModelOperatorProvisioningInfo{}, errors.Annotate(err, "getting API addresses")
	}

	controllerAgentInfo, err := a.ctrlSvc.GetControllerAgentInfo(ctx)
	if err != nil {
		return ModelOperatorProvisioningInfo{}, errors.Annotate(err, "getting controller agent info")
	}

	registryPath, err := podcfg.GetJujuOCIImagePathFromControllerCfg(controllerConfig, vers)
	if err != nil {
		return ModelOperatorProvisioningInfo{}, errors.Annotate(err, "getting OCI image path")
	}

	imageRepoDetails, err := docker.NewImageRepoDetails(controllerConfig.CAASImageRepo())
	if err != nil {
		return ModelOperatorProvisioningInfo{}, errors.Annotate(err, "parsing image repo details")
	}

	imageDetails := convertToDockerImageDetails(docker.ConvertToResourceImageDetails(imageRepoDetails), registryPath)

	return ModelOperatorProvisioningInfo{
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
