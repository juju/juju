// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"
	"os"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/flags"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
	userservice "github.com/juju/juju/domain/access/service"
	macaroonerrors "github.com/juju/juju/domain/macaroon/errors"
	networkerrors "github.com/juju/juju/domain/network/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	storageservice "github.com/juju/juju/domain/storage/service"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/worker/gate"
)

const (
	// States which report the state of the worker.
	stateStarted   = "started"
	stateCompleted = "completed"
)

// WorkerConfig encapsulates the configuration options for the
// bootstrap worker.
type WorkerConfig struct {
	Agent                      agent.Agent
	ObjectStoreGetter          ObjectStoreGetter
	ControllerAgentBinaryStore AgentBinaryStore
	ControllerConfigService    ControllerConfigService
	ControllerNodeService      ControllerNodeService
	CloudService               CloudService
	UserService                UserService
	StorageService             StorageService
	ProviderRegistry           storage.ProviderRegistry
	AgentPasswordService       AgentPasswordService
	ApplicationService         ApplicationService
	ControllerModel            coremodel.Model
	ModelConfigService         ModelConfigService
	MachineService             MachineService
	KeyManagerService          KeyManagerService
	FlagService                FlagService
	NetworkService             NetworkService
	BakeryConfigService        BakeryConfigService
	BootstrapAddressFinder     BootstrapAddressFinderFunc
	BootstrapUnlocker          gate.Unlocker
	AgentBinaryUploader        AgentBinaryBootstrapFunc
	ControllerCharmDeployer    ControllerCharmDeployerFunc
	PopulateControllerCharm    PopulateControllerCharmFunc
	SetMachineProvisioned      SetMachineProvisionedFunc
	FinaliseControllerNode     FinaliseControllerNodeFunc
	CharmhubHTTPClient         HTTPClient
	UnitPassword               string
	ServiceManagerGetter       ServiceManagerGetterFunc
	Logger                     logger.Logger
	Clock                      clock.Clock

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
	if c.ControllerAgentBinaryStore == nil {
		return errors.NotValidf("nil ControllerAgentBinaryStore")
	}
	if c.ControllerConfigService == nil {
		return errors.NotValidf("nil ControllerConfigService")
	}
	if c.ControllerNodeService == nil {
		return errors.NotValidf("nil ControllerNodeService")
	}
	if c.CloudService == nil {
		return errors.NotValidf("nil CloudService")
	}
	if c.UserService == nil {
		return errors.NotValidf("nil UserService")
	}
	if c.StorageService == nil {
		return errors.NotValidf("nil StorageService")
	}
	if c.AgentPasswordService == nil {
		return errors.NotValidf("nil AgentPasswordService")
	}
	if c.ApplicationService == nil {
		return errors.NotValidf("nil ApplicationService")
	}
	if c.ModelConfigService == nil {
		return errors.NotValidf("nil ModelConfigService")
	}
	if c.MachineService == nil {
		return errors.NotValidf("nil MachineService")
	}
	if c.KeyManagerService == nil {
		return errors.NotValidf("nil KeyManagerService")
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
	if c.NetworkService == nil {
		return errors.NotValidf("nil NetworkService")
	}
	if c.BakeryConfigService == nil {
		return errors.NotValidf("nil BakeryConfigService")
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
	if c.SetMachineProvisioned == nil {
		return errors.NotValidf("nil SetMachineProvisioned")
	}
	if c.FinaliseControllerNode == nil {
		return errors.NotValidf("nil FinaliseControllerNode")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if c.SystemState == nil {
		return errors.NotValidf("nil SystemState")
	}
	if c.BootstrapAddressFinder == nil {
		return errors.NotValidf("nil BootstrapAddressFinder")
	}
	if err := c.ControllerModel.UUID.Validate(); err != nil {
		return fmt.Errorf("controller model id: %w", err)
	}
	return nil
}

type bootstrapWorker struct {
	internalStates chan string
	cfg            WorkerConfig
	logger         logger.Logger
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
		logger:         cfg.Logger.Child("worker"),
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

	if err := w.seedMacaroonConfig(ctx); err != nil {
		return errors.Annotatef(err, "initialising macaroon bakery config")
	}

	// Insert all the initial users into the state.
	if err := w.seedInitialUsers(ctx); err != nil {
		return errors.Annotatef(err, "inserting initial users")
	}

	agentConfig := w.cfg.Agent.CurrentConfig()
	dataDir := agentConfig.DataDir()

	// Seed the agent binary to the object store.
	cleanup, err := w.seedAgentBinary(ctx, dataDir)
	if err != nil {
		return errors.Trace(err)
	}

	// Seed the controller charm to the object store.
	bootstrapParams, err := w.bootstrapParams(ctx, dataDir)
	if err != nil {
		return errors.Annotatef(err, "getting bootstrap params")
	}

	// Create the user specified storage pools.
	if err := w.seedStoragePools(ctx, bootstrapParams.StoragePools); err != nil {
		return errors.Annotate(err, "seeding storage pools")
	}

	controllerConfig, err := w.cfg.ControllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	// Retrieve controller addresses needed to set the API host ports.
	bootstrapAddresses, err := w.cfg.BootstrapAddressFinder(ctx, bootstrapParams.BootstrapMachineInstanceId)
	if err != nil {
		return errors.Trace(err)
	}

	servingInfo, ok := agentConfig.StateServingInfo()
	if !ok {
		return errors.Errorf("state serving information not available")
	}

	// Load spaces from the underlying substrate.
	if err := w.cfg.NetworkService.ReloadSpaces(ctx); err != nil {
		if !errors.Is(err, errors.NotSupported) {
			return errors.Trace(err)
		}
		w.logger.Debugf(ctx, "reload spaces not supported due to a non-networking environement")
	}

	// Deploy the controller charm after calling reload spaces or
	// no subnets will be available for the ip address table with
	// kubernetes.
	if err := w.seedControllerCharm(ctx, dataDir, bootstrapParams); err != nil {
		return errors.Trace(err)
	}

	if err := w.seedInitialAuthorizedKeys(ctx, bootstrapParams.ControllerModelAuthorizedKeys); err != nil {
		return errors.Trace(err)
	}

	// Set the bootstrap machine as provisioned.
	if err := w.cfg.SetMachineProvisioned(ctx, w.cfg.AgentPasswordService, w.cfg.MachineService, bootstrapParams, agentConfig); err != nil {
		return errors.Annotatef(err, "setting machine as provisioned")
	}

	// Finalise the controller node.
	if err := w.cfg.FinaliseControllerNode(ctx, w.cfg.ControllerNodeService, agentConfig); err != nil {
		return errors.Annotatef(err, "finalising controller node")
	}

	// Convert the provider addresses that we got from the bootstrap instance
	// to space ID decorated addresses.
	if err := w.initAPIHostPorts(ctx, controllerConfig, bootstrapAddresses, servingInfo.APIPort); err != nil {
		w.logger.Errorf(ctx, "unable to set API host ports %v:%w", bootstrapAddresses, err)
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

func (w *bootstrapWorker) seedMacaroonConfig(ctx context.Context) error {
	err := w.cfg.BakeryConfigService.InitialiseBakeryConfig(ctx)
	if errors.Is(err, macaroonerrors.BakeryConfigAlreadyInitialised) {
		return nil
	}
	return errors.Trace(err)
}

func (w *bootstrapWorker) seedInitialUsers(ctx context.Context) error {
	// Any failure should be retryable, so we can re-attempt to bootstrap.

	controllerCfg, err := w.cfg.ControllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	controllerUUID := controllerCfg.ControllerUUID()

	adminUser, err := w.cfg.UserService.GetUserByName(ctx, user.AdminUserName)
	if err != nil {
		return errors.Annotatef(err, "getting admin user %q", user.AdminUserName)
	}

	pass, err := password.RandomPassword()
	if err != nil {
		return errors.Annotatef(err, "generating metrics password")
	}
	metricsPassword := auth.NewPassword(pass)

	metricsName, err := user.NewName("juju-metrics")
	if err != nil {
		return errors.Trace(err)
	}
	_, _, err = w.cfg.UserService.AddUser(ctx, userservice.AddUserArg{
		Name:        metricsName,
		DisplayName: "Juju Metrics",
		Password:    &metricsPassword,
		CreatorUUID: adminUser.UUID,
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        controllerUUID,
			},
		},
	})
	if errors.Is(err, accesserrors.UserAlreadyExists) {
		return nil
	}

	err = w.cfg.UserService.AddExternalUser(
		ctx,
		permission.EveryoneUserName,
		"",
		adminUser.UUID,
	)
	if errors.Is(err, accesserrors.UserAlreadyExists) {
		return nil
	}

	return errors.Annotatef(err, "inserting initial users")
}

// seedInitialAuthorisedKeys is responsible for adding any extra authorised keys
// requested during bootstrap to the admin user on the controller model. It is
// valid and safe to pass in a nil slice of keys to this function.
func (w *bootstrapWorker) seedInitialAuthorizedKeys(
	ctx context.Context,
	keys []string,
) error {
	adminUser, err := w.cfg.UserService.GetUserByName(ctx, coremodel.ControllerModelOwnerUsername)
	if err != nil {
		return fmt.Errorf(
			"cannot get %q user to seed %d authorized keys into the controller model: %w",
			coremodel.ControllerModelOwnerUsername,
			len(keys),
			err,
		)
	}

	err = w.cfg.KeyManagerService.AddPublicKeysForUser(ctx, adminUser.UUID, keys...)
	if err != nil {
		return fmt.Errorf("cannot seed %d authorized keys into the controller model: %w",
			len(keys),
			err,
		)
	}

	return nil
}

func (w *bootstrapWorker) seedStoragePools(ctx context.Context, poolParams map[string]storage.Attrs) error {
	storagePools, err := initialStoragePools(w.cfg.ProviderRegistry, poolParams)
	if err != nil {
		return errors.Trace(err)
	}
	for _, p := range storagePools {
		if err := w.cfg.StorageService.CreateStoragePool(ctx, p.Name(), p.Provider(), storageservice.PoolAttrs(p.Attrs())); err != nil {
			// Allow for bootstrap worker to have been restarted.
			if errors.Is(err, storageerrors.PoolAlreadyExists) {
				continue
			}
			return errors.Annotatef(err, "saving storage pool %q", p.Name())
		}
	}
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
	allSpaces, err := w.cfg.NetworkService.GetAllSpaces(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	addrs, err := pAddrs.ToSpaceAddresses(allSpaces)
	if err != nil {
		return errors.Trace(err)
	}
	hostPorts := network.SpaceAddressesWithPort(addrs, apiPort)

	mgmtSpaceCfg := controllerConfig.JujuManagementSpace()
	mgmtSpace, err := w.cfg.NetworkService.SpaceByName(ctx, mgmtSpaceCfg)
	if err != nil && !errors.Is(err, networkerrors.SpaceNotFound) {
		return errors.Trace(err)
	}

	// During bootstrap, the controller node will always be "0".
	if err := w.cfg.ControllerNodeService.SetAPIAddresses(ctx, "0", hostPorts, mgmtSpace); err != nil {
		return errors.Trace(err)
	}

	// TODO(nvinuesa): Remove this double write to mongodb once we wire the
	// apiaddresssetter worker.
	hostPortsForAgents := w.filterHostPortsForManagementSpace(ctx, mgmtSpaceCfg, []network.SpaceHostPorts{hostPorts}, allSpaces)
	return w.cfg.SystemState.SetAPIHostPorts(controllerConfig, []network.SpaceHostPorts{hostPorts}, hostPortsForAgents)
}

// We filter the collection of API addresses based on the configured
// management space for the controller.
// If there is no space configured, or if one of the slices is filtered down
// to zero elements, just use the unfiltered slice for safety - we do not
// want to cut off communication to the controller based on erroneous config.
func (w *bootstrapWorker) filterHostPortsForManagementSpace(
	ctx context.Context,
	mgmtSpace network.SpaceName,
	apiHostPorts []network.SpaceHostPorts,
	allSpaces network.SpaceInfos,
) []network.SpaceHostPorts {
	var hostPortsForAgents []network.SpaceHostPorts

	if mgmtSpace == "" {
		hostPortsForAgents = apiHostPorts
	} else {
		mgmtSpaceInfo := allSpaces.GetByName(mgmtSpace)
		if mgmtSpaceInfo == nil {
			return apiHostPorts
		}
		hostPortsForAgents = make([]network.SpaceHostPorts, len(apiHostPorts))
		for i, apiHostPort := range apiHostPorts {
			filtered, addrsIsInSpace := apiHostPort.InSpaces(*mgmtSpaceInfo)
			if addrsIsInSpace {
				hostPortsForAgents[i] = filtered
			} else {
				w.logger.Warningf(ctx, "API addresses %v not in the management space %s", apiHostPort, mgmtSpace)
				hostPortsForAgents[i] = apiHostPort
			}
		}
	}

	return hostPortsForAgents
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *bootstrapWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.tomb.Context(context.Background()))
}

func (w *bootstrapWorker) seedAgentBinary(ctx context.Context, dataDir string) (func(), error) {
	objectStore, err := w.cfg.ObjectStoreGetter.GetObjectStore(
		ctx,
		w.cfg.ControllerModel.UUID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get object store: %w", err)
	}

	// Agent binary seeder will populate the tools for the agent.
	// TODO (tlm) agentStore is a temprorary hook back into Mongo that will be
	// removed soon.
	agentStorage := agentStorageShim{State: w.cfg.SystemState}
	cleanup, err := w.cfg.AgentBinaryUploader(
		ctx,
		dataDir,
		agentStorage,
		w.cfg.ControllerAgentBinaryStore,
		objectStore,
		w.cfg.Logger.Child("agentbinary"),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return cleanup, nil
}

func (w *bootstrapWorker) seedControllerCharm(ctx context.Context, dataDir string, bootstrapArgs instancecfg.StateInitializationParams) error {
	controllerConfig, err := w.cfg.ControllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	objectStore, err := w.cfg.ObjectStoreGetter.GetObjectStore(
		ctx,
		w.cfg.ControllerModel.UUID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to get object store: %w", err)
	}

	// Controller charm seeder will populate the charm for the controller.
	deployer, err := w.cfg.ControllerCharmDeployer(ctx, ControllerCharmDeployerConfig{
		StateBackend:                w.cfg.SystemState,
		AgentPasswordService:        w.cfg.AgentPasswordService,
		ApplicationService:          w.cfg.ApplicationService,
		Model:                       w.cfg.ControllerModel,
		ModelConfigService:          w.cfg.ModelConfigService,
		ObjectStore:                 objectStore,
		ControllerConfig:            controllerConfig,
		DataDir:                     dataDir,
		BootstrapMachineConstraints: bootstrapArgs.BootstrapMachineConstraints,
		ControllerCharmName:         bootstrapArgs.ControllerCharmPath,
		ControllerCharmChannel:      bootstrapArgs.ControllerCharmChannel,
		CharmhubHTTPClient:          w.cfg.CharmhubHTTPClient,
		UnitPassword:                w.cfg.UnitPassword,
		ServiceManagerGetter:        w.cfg.ServiceManagerGetter,
		Logger:                      w.cfg.Logger,
		Clock:                       w.cfg.Clock,
	})
	if err != nil {
		return errors.Trace(err)
	}

	return w.cfg.PopulateControllerCharm(ctx, deployer)
}

func (w *bootstrapWorker) bootstrapParams(ctx context.Context, dataDir string) (instancecfg.StateInitializationParams, error) {
	bootstrapParamsData, err := os.ReadFile(bootstrap.BootstrapParamsPath(dataDir))
	if err != nil {
		return instancecfg.StateInitializationParams{}, errors.Annotate(err, "reading bootstrap params file")
	}
	var args instancecfg.StateInitializationParams
	if err := args.Unmarshal(bootstrapParamsData); err != nil {
		return instancecfg.StateInitializationParams{}, errors.Trace(err)
	}
	return args, nil
}

// initialStoragePools extract any storage pools included with the bootstrap params.
func initialStoragePools(registry storage.ProviderRegistry, poolParams map[string]storage.Attrs) ([]*storage.Config, error) {
	var result []*storage.Config
	defaultStoragePools, err := domainstorage.DefaultStoragePools(registry)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Add the default storage pools.
	result = append(result, defaultStoragePools...)

	for name, attrs := range poolParams {
		pType, _ := attrs[domainstorage.StorageProviderType].(string)
		if pType == "" {
			return nil, errors.Errorf("missing provider type for storage pool %q", name)
		}
		delete(attrs, domainstorage.StoragePoolName)
		delete(attrs, domainstorage.StorageProviderType)
		pool, err := storage.NewConfig(name, storage.ProviderType(pType), attrs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result = append(result, pool)
	}
	return result, nil
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
