// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"os"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/charmdownloader"
	"github.com/juju/juju/internal/charm/repository"
	"github.com/juju/juju/internal/charmhub"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
)

// ServiceManager provides the API to manipulate services.
type ServiceManager interface {
	// GetService returns the service for the specified application.
	GetService(ctx context.Context, appName string, includeClusterIP bool) (*caas.Service, error)
}

// ServiceManagerGetterFunc is the function that is used to get a service manager.
type ServiceManagerGetterFunc func(context.Context) (ServiceManager, error)

// AgentBinaryBootstrapFunc is the function that is used to populate the tools.
type AgentBinaryBootstrapFunc func(
	context.Context,
	string,
	AgentBinaryStore,
	objectstore.ObjectStore,
	logger.Logger,
) (func(), error)

// ControllerCharmDeployerConfig holds the configuration for the
// ControllerCharmDeployer.
type ControllerCharmDeployerConfig struct {
	AgentPasswordService        AgentPasswordService
	ApplicationService          ApplicationService
	Model                       coremodel.Model
	ModelConfigService          ModelConfigService
	ObjectStore                 objectstore.ObjectStore
	ControllerConfig            controller.Config
	DataDir                     string
	BootstrapAddresses          network.ProviderAddresses
	BootstrapMachineConstraints constraints.Value
	ControllerCharmName         string
	ControllerCharmChannel      charm.Channel
	CharmhubHTTPClient          HTTPClient
	UnitPassword                string
	ServiceManagerGetter        ServiceManagerGetterFunc
	Logger                      logger.Logger
	Clock                       clock.Clock
}

// CAASControllerUnitPassword is the function that is used to get the unit
// password for CAAS. This is currently retrieved from the environment
// variable.
func CAASControllerUnitPassword(context.Context) (string, error) {
	return os.Getenv(k8sconstants.EnvJujuK8sUnitPassword), nil
}

// IAASControllerUnitPassword is the function that is used to get the unit
// password for IAAS.
func IAASControllerUnitPassword(context.Context) (string, error) {
	// IAAS doesn't need a unit password.
	return "", nil
}

// CAASAgentBinaryUploader is the function that is used to populate the tools
// for CAAS.
func CAASAgentBinaryUploader(context.Context, string, AgentBinaryStore, objectstore.ObjectStore, logger.Logger) (func(), error) {
	// CAAS doesn't need to populate the tools.
	return func() {}, nil
}

// IAASAgentBinaryUploader is the function that is used to populate the tools
// for IAAS.
func IAASAgentBinaryUploader(
	ctx context.Context,
	dataDir string,
	agentBinaryStore AgentBinaryStore,
	objectStore objectstore.ObjectStore,
	logger logger.Logger,
) (func(), error) {
	return bootstrap.PopulateAgentBinary(ctx, dataDir, agentBinaryStore, logger)
}

// CAASControllerCharmUploader is the function that is used to upload the
// controller charm for CAAS.
func CAASControllerCharmUploader(ctx context.Context, cfg ControllerCharmDeployerConfig) (bootstrap.ControllerCharmDeployer, error) {
	serviceManager, err := cfg.ServiceManagerGetter(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return bootstrap.NewCAASDeployer(bootstrap.CAASDeployerConfig{
		BaseDeployerConfig: makeBaseDeployerConfig(cfg),
		ApplicationService: cfg.ApplicationService,
		UnitPassword:       cfg.UnitPassword,
		ServiceManager:     serviceManager,
	})
}

// IAASControllerCharmUploader is the function that is used to upload the
// controller charm for CAAS.
func IAASControllerCharmUploader(ctx context.Context, cfg ControllerCharmDeployerConfig) (bootstrap.ControllerCharmDeployer, error) {
	return bootstrap.NewIAASDeployer(bootstrap.IAASDeployerConfig{
		BaseDeployerConfig: makeBaseDeployerConfig(cfg),
		ApplicationService: cfg.ApplicationService,
		HostBaseFn:         coreos.HostBase,
	})
}

func makeBaseDeployerConfig(cfg ControllerCharmDeployerConfig) bootstrap.BaseDeployerConfig {
	return bootstrap.BaseDeployerConfig{
		DataDir:              cfg.DataDir,
		ObjectStore:          cfg.ObjectStore,
		ApplicationService:   cfg.ApplicationService,
		AgentPasswordService: cfg.AgentPasswordService,
		ModelConfigService:   cfg.ModelConfigService,
		Constraints:          cfg.BootstrapMachineConstraints,
		BootstrapAddresses:   cfg.BootstrapAddresses,
		ControllerConfig:     cfg.ControllerConfig,
		Channel:              cfg.ControllerCharmChannel,
		CharmhubHTTPClient:   cfg.CharmhubHTTPClient,
		ControllerCharmName:  cfg.ControllerCharmName,
		NewCharmHubRepo: func(cfg repository.CharmHubRepositoryConfig) (corecharm.Repository, error) {
			return repository.NewCharmHubRepository(cfg)
		},
		NewCharmDownloader: func(client bootstrap.HTTPClient, logger logger.Logger) bootstrap.Downloader {
			charmhubClient := charmhub.NewDownloadClient(client, charmhub.DefaultFileSystem(), logger)
			return charmdownloader.NewCharmDownloader(charmhubClient, logger)
		},
		Logger: cfg.Logger,
		Clock:  cfg.Clock,
	}
}
