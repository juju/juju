// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/flags"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
)

const (
	// States which report the state of the worker.
	stateStarted   = "started"
	stateCompleted = "completed"
)

// LegacyState is the interface that is used to get the legacy state (mongo).
type LegacyState interface {
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
}

// WorkerConfig encapsulates the configuration options for the
// bootstrap worker.
type WorkerConfig struct {
	Agent                   agent.Agent
	ObjectStoreGetter       ObjectStoreGetter
	ControllerConfigService ControllerConfigService
	CredentialService       CredentialService
	CloudService            CloudService
	FlagService             FlagService
	SpaceService            SpaceService
	BootstrapUnlocker       gate.Unlocker
	AgentBinaryUploader     AgentBinaryBootstrapFunc
	ControllerCharmDeployer ControllerCharmDeployerFunc
	PopulateControllerCharm PopulateControllerCharmFunc
	CharmhubHTTPClient      HTTPClient
	UnitPassword            string
	NewEnviron              NewEnvironFunc
	BootstrapAddresses      BootstrapAddressesFunc
	BootstrapAddressFinder  BootstrapAddressFinderFunc
	LoggerFactory           LoggerFactory

	// Deprecated: This is only here, until we can remove the state layer.
	SystemState SystemState
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.Agent == nil {
		return errors.NotValidf("nil Agent")
	}
	if c.ObjectStoreGetter == nil {
		return errors.NotValidf("nil ObjectStoreGetter")
	}
	if c.ControllerConfigService == nil {
		return errors.NotValidf("nil ControllerConfigService")
	}
	if c.CredentialService == nil {
		return errors.NotValidf("nil CredentialService")
	}
	if c.CloudService == nil {
		return errors.NotValidf("nil CloudService")
	}
	if c.BootstrapUnlocker == nil {
		return errors.NotValidf("nil BootstrapUnlocker")
	}
	if c.AgentBinaryUploader == nil {
		return errors.NotValidf("nil AgentBinaryUploader")
	}
	if c.FlagService == nil {
		return errors.NotValidf("nil FlagService")
	}
	if c.SpaceService == nil {
		return errors.NotValidf("nil SpaceService")
	}
	if c.ControllerCharmDeployer == nil {
		return errors.NotValidf("nil ControllerCharmDeployer")
	}
	if c.PopulateControllerCharm == nil {
		return errors.NotValidf("nil PopulateControllerCharm")
	}
	if c.CharmhubHTTPClient == nil {
		return errors.NotValidf("nil CharmhubHTTPClient")
	}
	if c.LoggerFactory == nil {
		return errors.NotValidf("nil LoggerFactory")
	}
	if c.SystemState == nil {
		return errors.NotValidf("nil SystemState")
	}
	if c.BootstrapAddressFinder == nil {
		return errors.NotValidf("nil BootstrapAddressFinder")
	}
	if c.NewEnviron == nil {
		return errors.NotValidf("nil NewEnviron")
	}
	if c.BootstrapAddresses == nil {
		return errors.NotValidf("nil BootstrapAddresses")
	}
	return nil
}

type bootstrapWorker struct {
	internalStates chan string
	cfg            WorkerConfig
	tomb           tomb.Tomb
}

// NewWorker creates a new bootstrap worker.
func NewWorker(cfg WorkerConfig) (*bootstrapWorker, error) {
	return newWorker(cfg, nil)
}

func newWorker(cfg WorkerConfig, internalStates chan string) (*bootstrapWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &bootstrapWorker{
		internalStates: internalStates,
		cfg:            cfg,
	}
	w.tomb.Go(w.loop)
	return w, nil
}

// Kill stops the worker.
func (w *bootstrapWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the worker to stop and then returns the reason it was
// killed.
func (w *bootstrapWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *bootstrapWorker) loop() error {
	// Report the initial started state.
	w.reportInternalState(stateStarted)

	ctx, cancel := w.scopedContext()
	defer cancel()

	agentConfig := w.cfg.Agent.CurrentConfig()
	dataDir := agentConfig.DataDir()

	// Seed the agent binary to the object store.
	cleanup, err := w.seedAgentBinary(ctx, dataDir)
	if err != nil {
		return errors.Trace(err)
	}

	// Seed the controller charm to the object store.
	args, err := w.bootstrapParams(ctx, dataDir)
	if err != nil {
		return errors.Annotatef(err, "getting bootstrap params")
	}
	controllerConfig, err := w.cfg.ControllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	// Create cloud service.
	if err := w.initControllerCloudService(ctx, controllerConfig.ControllerUUID(), w.cfg.SystemState); err != nil {
		return errors.Annotate(err, "cannot initialize cloud service")
	}
	if err := w.seedControllerCharm(ctx, dataDir, args, controllerConfig); err != nil {
		return errors.Trace(err)
	}

	// Retrieve controller addresses needed to set the API host ports.
	_, err = w.cfg.BootstrapAddressFinder(ctx, BootstrapAddressesConfig{
		BootstrapInstanceID:    bootstrapArgs.BootstrapMachineInstanceId,
		SystemState:            w.cfg.SystemState,
		CloudService:           w.cfg.CloudService,
		CredentialService:      w.cfg.CredentialService,
		NewEnvironFunc:         w.cfg.NewEnviron,
		BootstrapAddressesFunc: w.cfg.BootstrapAddresses,
	})
	if err != nil {
		return errors.Trace(err)
	}
	servingInfo, ok := agentConfig.StateServingInfo()
	if !ok {
		return errors.Errorf("state serving information not available")
	}
	// Convert the provider addresses that we got from the bootstrap instance
	// to space ID decorated addresses.
	if err := w.initAPIHostPorts(ctx, controllerConfig, bootstrapAddresses, servingInfo.APIPort); err != nil {
		return errors.Trace(err)
	}

	// Set the bootstrap flag, to indicate that the bootstrap has completed.
	if err := w.cfg.FlagService.SetFlag(ctx, flags.BootstrapFlag, true, flags.BootstrapFlagDescription); err != nil {
		return errors.Trace(err)
	}

	// Cleanup only after the bootstrap flag has been set.
	cleanup()

	w.reportInternalState(stateCompleted)

	w.cfg.BootstrapUnlocker.Unlock()
	return nil
}

func (w *bootstrapWorker) reportInternalState(state string) {
	select {
	case <-w.tomb.Dying():
	case w.internalStates <- state:
	default:
	}
}

// initAPIHostPorts sets the initial API host/port addresses in state.
func (w *bootstrapWorker) initAPIHostPorts(ctx context.Context, controllerConfig controller.Config, pAddrs network.ProviderAddresses, apiPort int) error {
	addrs, err := w.providerAddressesToSpaceAddresses(ctx, pAddrs)
	if err != nil {
		return errors.Trace(err)
	}

	hostPorts := []network.SpaceHostPorts{network.SpaceAddressesWithPort(addrs, apiPort)}
	hostPortsForAgents, err := w.cfg.SpaceService.FilterHostPortsForManagementSpace(ctx, controllerConfig, hostPorts)
	if err != nil {
		return errors.Trace(err)
	}

	return w.cfg.SystemState.SetAPIHostPorts(controllerConfig, hostPorts, hostPortsForAgents)
}

// initControllerCloudService creates cloud service for controller service.
func (w *bootstrapWorker) initControllerCloudService(
	ctx context.Context,
	controllerUUID string,
	st SystemState,
) error {
	env, err := w.getEnviron(ctx)
	if err != nil {
		return errors.Annotate(err, "getting environ")
	}

	broker, ok := env.(caas.ServiceManager)
	if !ok {
		// This means we are on IAAS environs, so we just return.
		return nil
	}
	svc, err := broker.GetService(ctx, constants.JujuControllerStackName, true)
	if err != nil {
		return errors.Trace(err)
	}

	if len(svc.Addresses) == 0 {
		// this should never happen because we have already checked in k8s controller bootstrap stacker.
		return errors.NotProvisionedf("k8s controller service %q address", svc.Id)
	}
	addrs, err := w.providerAddressesToSpaceAddresses(ctx, svc.Addresses)
	if err != nil {
		return errors.Trace(err)
	}

	svcId := controllerUUID
	cloudSvc, err := st.SaveCloudService(state.SaveCloudServiceArgs{
		Id:         svcId,
		ProviderId: svc.Id,
		Addresses:  addrs,
	})
	w.cfg.LoggerFactory.Child("").Debugf("created cloud service %v for controller", cloudSvc)
	return errors.Trace(err)
}

func (w *bootstrapWorker) providerAddressesToSpaceAddresses(ctx context.Context, providerAddresses network.ProviderAddresses) (network.SpaceAddresses, error) {
	addrs := make(network.SpaceAddresses, len(providerAddresses))

	for i, pa := range providerAddresses {
		addrs[i] = network.SpaceAddress{MachineAddress: pa.MachineAddress}
		if pa.SpaceName != "" {
			spInfo, err := w.cfg.SpaceService.SpaceByName(ctx, string(pa.SpaceName))
			if err != nil {
				return nil, errors.Trace(err)
			}
			if spInfo == nil {
				return nil, errors.NotFoundf("space with name %q", pa.SpaceName)
			}
			addrs[i].SpaceID = spInfo.ID
		}
	}

	return addrs, nil
}

func (w *bootstrapWorker) getBoostrapAddresses(ctx context.Context, bootstrapInstanceID instance.Id) (network.ProviderAddresses, error) {

	// Retrieve controller addresses needed to set the API host ports.
	var addresses network.ProviderAddresses
	env, err := w.getEnviron(ctx)
	if err != nil && !errors.Is(err, errors.NotSupported) {
		return nil, errors.Trace(err)
	}
	if errors.Is(err, errors.NotSupported) {
		return network.NewMachineAddresses([]string{"localhost"}).AsProviderAddresses(), nil
	}
	addresses, err = w.cfg.BootstrapAddressesFunc(ctx, env, bootstrapInstanceID)
	if err != nil && !errors.Is(err, errors.NotSupported) {
		return nil, errors.Trace(err)
	}
	if errors.Is(err, errors.NotSupported) {
		addresses = network.NewMachineAddresses([]string{"localhost"}).AsProviderAddresses()
	}

	return addresses, nil
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *bootstrapWorker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.tomb.Context(ctx), cancel
}

func (w *bootstrapWorker) seedAgentBinary(ctx context.Context, dataDir string) (func(), error) {
	objectStore, err := w.cfg.ObjectStoreGetter.GetObjectStore(ctx, w.cfg.SystemState.ControllerModelUUID())
	if err != nil {
		return nil, fmt.Errorf("failed to get object store: %w", err)
	}

	// Agent binary seeder will populate the tools for the agent.
	agentStorage := agentStorageShim{State: w.cfg.SystemState}
	cleanup, err := w.cfg.AgentBinaryUploader(ctx, dataDir, agentStorage, objectStore, w.cfg.LoggerFactory.Child("agentbinary"))
	if err != nil {
		return nil, errors.Trace(err)
	}

	return cleanup, nil
}

func (w *bootstrapWorker) seedControllerCharm(ctx context.Context, dataDir string, bootstrapArgs instancecfg.StateInitializationParams) (instancecfg.StateInitializationParams, error) {
	controllerConfig, err := w.cfg.ControllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return instancecfg.StateInitializationParams{}, errors.Trace(err)
	}

	objectStore, err := w.cfg.ObjectStoreGetter.GetObjectStore(ctx, w.cfg.SystemState.ControllerModelUUID())
	if err != nil {
		return fmt.Errorf("failed to get object store: %v", err)
	}

	// Controller charm seeder will populate the charm for the controller.
	deployer, err := w.cfg.ControllerCharmDeployer(ControllerCharmDeployerConfig{
		StateBackend:                w.cfg.SystemState,
		ObjectStore:                 objectStore,
		ControllerConfig:            controllerConfig,
		DataDir:                     dataDir,
		BootstrapMachineConstraints: bootstrapArgs.BootstrapMachineConstraints,
		ControllerCharmName:         bootstrapArgs.ControllerCharmPath,
		ControllerCharmChannel:      bootstrapArgs.ControllerCharmChannel,
		CharmhubHTTPClient:          w.cfg.CharmhubHTTPClient,
		UnitPassword:                w.cfg.UnitPassword,
		LoggerFactory:               w.cfg.LoggerFactory,
	})
	if err != nil {
		return errors.Trace(err)
	}

	return w.cfg.PopulateControllerCharm(ctx, deployer)
}

func (w *bootstrapWorker) bootstrapParams(ctx context.Context, dataDir string) (instancecfg.StateInitializationParams, error) {
	bootstrapParamsData, err := os.ReadFile(filepath.Join(dataDir, cloudconfig.FileNameBootstrapParams))
	if err != nil {
		return instancecfg.StateInitializationParams{}, errors.Annotate(err, "reading bootstrap params file")
	}
	var args instancecfg.StateInitializationParams
	if err := args.Unmarshal(bootstrapParamsData); err != nil {
		return instancecfg.StateInitializationParams{}, errors.Trace(err)
	}
	return args, nil
}

type agentStorageShim struct {
	State SystemState
}

// AgentBinaryStorage returns the interface for the BinaryAgentStorage.
// This is currently a shim wrapper around the tools storage. That will be
// renamed once we re-implement the tools storage in dqlite.
func (s agentStorageShim) AgentBinaryStorage(objectStore objectstore.ObjectStore) (BinaryAgentStorage, error) {
	return s.State.ToolsStorage(objectStore)
}
