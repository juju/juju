// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"io"
	"os"

	"github.com/juju/charm/v13"
	"github.com/juju/errors"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/charm/services"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
)

// SystemState is the interface that is used to get the legacy state (mongo).
//
// Note: It is expected over time for each one of these methods to be replaced
// with a domain service.
//
// Deprecated: Use domain services when available.
type SystemState interface {
	// ControllerModelUUID returns the UUID of the model that was
	// bootstrapped.  This is the only model that can have controller
	// machines.  The owner of this model is also considered "special", in
	// that they are the only user that is able to create other users
	// (until we have more fine grained permissions), and they cannot be
	// disabled.
	ControllerModelUUID() string
	// ToolsStorage returns a new binarystorage.StorageCloser that stores tools
	// metadata in the "juju" database "toolsmetadata" collection.
	ToolsStorage(store objectstore.ObjectStore) (binarystorage.StorageCloser, error)
	// AddApplication adds an application to the model.
	AddApplication(state.AddApplicationArgs, objectstore.ObjectStore) (bootstrap.Application, error)
	// Charm returns the charm with the given name.
	Charm(string) (bootstrap.Charm, error)
	// Model returns the model.
	Model() (bootstrap.Model, error)
	// ModelUUID returns the UUID of the model.
	ModelUUID() string
	// Unit returns the unit with the given id.
	Unit(string) (bootstrap.Unit, error)
	// Machine returns the machine with the given id.
	Machine(string) (bootstrap.Machine, error)
	// PrepareLocalCharmUpload returns the charm URL that should be used to
	// upload the charm.
	PrepareLocalCharmUpload(url string) (chosenURL *charm.URL, err error)
	// UpdateUploadedCharm updates the charm with the given info.
	UpdateUploadedCharm(info state.CharmInfo) (services.UploadedCharm, error)
	// PrepareCharmUpload returns the charm URL that should be used to upload
	// the charm.
	PrepareCharmUpload(curl string) (services.UploadedCharm, error)
	// ApplyOperation applies the given operation.
	ApplyOperation(*state.UpdateUnitOperation) error
	// CloudService returns the cloud service for the given cloud.
	CloudService(string) (bootstrap.CloudService, error)
	// SetAPIHostPorts sets the addresses, if changed, of two collections:
	//   - The list of *all* addresses at which the API is accessible.
	//   - The list of addresses at which the API can be accessed by agents according
	//     to the controller management space configuration.
	//
	// Each server is represented by one element in the top level slice.
	SetAPIHostPorts(controllerConfig controller.Config, newHostPorts []network.SpaceHostPorts, newHostPortsForAgents []network.SpaceHostPorts) error
	// SaveCloudService creates a cloud service.
	SaveCloudService(args state.SaveCloudServiceArgs) (*state.CloudService, error)
}

// BinaryAgentStorageService is the interface that is used to get the storage
// for the agent binary.
type BinaryAgentStorageService interface {
	AgentBinaryStorage(objectstore.ObjectStore) (BinaryAgentStorage, error)
}

// BinaryAgentStorage is the interface that is used to store the agent binary.
type BinaryAgentStorage interface {
	// Add adds the agent binary to the storage.
	Add(context.Context, io.Reader, binarystorage.Metadata) error
	// Close closes the storage.
	Close() error
}

// AgentBinaryBootstrapFunc is the function that is used to populate the tools.
type AgentBinaryBootstrapFunc func(context.Context, string, BinaryAgentStorageService, objectstore.ObjectStore, logger.Logger) (func(), error)

// ControllerCharmDeployerConfig holds the configuration for the
// ControllerCharmDeployer.
type ControllerCharmDeployerConfig struct {
	StateBackend                SystemState
	ApplicationService          ApplicationService
	ObjectStore                 objectstore.ObjectStore
	ControllerConfig            controller.Config
	DataDir                     string
	BootstrapMachineConstraints constraints.Value
	ControllerCharmName         string
	ControllerCharmChannel      charm.Channel
	CharmhubHTTPClient          HTTPClient
	UnitPassword                string
	Logger                      logger.Logger
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
func CAASAgentBinaryUploader(context.Context, string, BinaryAgentStorageService, objectstore.ObjectStore, logger.Logger) (func(), error) {
	// CAAS doesn't need to populate the tools.
	return func() {}, nil
}

// IAASAgentBinaryUploader is the function that is used to populate the tools
// for IAAS.
func IAASAgentBinaryUploader(ctx context.Context, dataDir string, storageService BinaryAgentStorageService, objectStore objectstore.ObjectStore, logger logger.Logger) (func(), error) {
	storage, err := storageService.AgentBinaryStorage(objectStore)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer storage.Close()

	return bootstrap.PopulateAgentBinary(ctx, dataDir, storage, logger)
}

// CAASControllerCharmUploader is the function that is used to upload the
// controller charm for CAAS.
func CAASControllerCharmUploader(cfg ControllerCharmDeployerConfig) (bootstrap.ControllerCharmDeployer, error) {
	return bootstrap.NewCAASDeployer(bootstrap.CAASDeployerConfig{
		BaseDeployerConfig: makeBaseDeployerConfig(cfg),
		CloudServiceGetter: cfg.StateBackend,
		OperationApplier:   cfg.StateBackend,
		UnitPassword:       cfg.UnitPassword,
	})
}

// IAASControllerCharmUploader is the function that is used to upload the
// controller charm for CAAS.
func IAASControllerCharmUploader(cfg ControllerCharmDeployerConfig) (bootstrap.ControllerCharmDeployer, error) {
	return bootstrap.NewIAASDeployer(bootstrap.IAASDeployerConfig{
		BaseDeployerConfig: makeBaseDeployerConfig(cfg),
		MachineGetter:      cfg.StateBackend,
	})
}

func makeBaseDeployerConfig(cfg ControllerCharmDeployerConfig) bootstrap.BaseDeployerConfig {
	return bootstrap.BaseDeployerConfig{
		DataDir:             cfg.DataDir,
		ObjectStore:         cfg.ObjectStore,
		StateBackend:        cfg.StateBackend,
		ApplicationService:  cfg.ApplicationService,
		CharmUploader:       cfg.StateBackend,
		Constraints:         cfg.BootstrapMachineConstraints,
		ControllerConfig:    cfg.ControllerConfig,
		Channel:             cfg.ControllerCharmChannel,
		CharmhubHTTPClient:  cfg.CharmhubHTTPClient,
		ControllerCharmName: cfg.ControllerCharmName,
		NewCharmRepo: func(cfg services.CharmRepoFactoryConfig) (corecharm.Repository, error) {
			charmRepoFactory := services.NewCharmRepoFactory(cfg)
			return charmRepoFactory.GetCharmRepository(context.TODO(), corecharm.CharmHub)
		},
		NewCharmDownloader: func(cfg services.CharmDownloaderConfig) (bootstrap.Downloader, error) {
			return services.NewCharmDownloader(cfg)
		},
		Logger: cfg.Logger,
	}
}

type stateShim struct {
	*state.State
	prechecker environs.InstancePrechecker
}

func (s *stateShim) PrepareCharmUpload(curl string) (services.UploadedCharm, error) {
	return s.State.PrepareCharmUpload(curl)
}

func (s *stateShim) UpdateUploadedCharm(info state.CharmInfo) (services.UploadedCharm, error) {
	return s.State.UpdateUploadedCharm(info)
}

func (s *stateShim) AddApplication(args state.AddApplicationArgs, objectStore objectstore.ObjectStore) (bootstrap.Application, error) {
	a, err := s.State.AddApplication(s.prechecker, args, objectStore)
	if err != nil {
		return nil, err
	}
	return &applicationShim{Application: a}, nil
}

func (s *stateShim) Charm(name string) (bootstrap.Charm, error) {
	c, err := s.State.Charm(name)
	if err != nil {
		return nil, err
	}
	return &charmShim{Charm: c}, nil
}

func (s *stateShim) Model() (bootstrap.Model, error) {
	m, err := s.State.Model()
	if err != nil {
		return nil, err
	}
	return &modelShim{Model: m}, nil
}

func (s *stateShim) Unit(tag string) (bootstrap.Unit, error) {
	u, err := s.State.Unit(tag)
	if err != nil {
		return nil, err
	}
	return &unitShim{Unit: u}, nil
}

func (s *stateShim) Machine(name string) (bootstrap.Machine, error) {
	m, err := s.State.Machine(name)
	if err != nil {
		return nil, err
	}
	return &machineShim{Machine: m}, nil
}

func (s *stateShim) ApplyOperation(op *state.UpdateUnitOperation) error {
	return s.State.ApplyOperation(op)
}

func (s *stateShim) CloudService(name string) (bootstrap.CloudService, error) {
	return s.State.CloudService(name)
}

type applicationShim struct {
	*state.Application
}

type charmShim struct {
	*state.Charm
}

type modelShim struct {
	*state.Model
}

type unitShim struct {
	*state.Unit
}

type machineShim struct {
	*state.Machine
}
