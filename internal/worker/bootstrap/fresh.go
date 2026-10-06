// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"os"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
	userservice "github.com/juju/juju/domain/access/service"
	macaroonerrors "github.com/juju/juju/domain/macaroon/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	environsbootstrap "github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/password"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	internalstorage "github.com/juju/juju/internal/storage"
)

var bootstrapSSHUser = "ubuntu"

// DeleteBootstrapSSHKeys removes bootstrap-only keys from an IAAS bootstrap
// machine's standard Ubuntu authorized_keys file.
func DeleteBootstrapSSHKeys(keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	// IAAS bootstrap machines use the standard Ubuntu account and file. K8s
	// bootstrap does not call this helper because it has no Ubuntu host.
	fingerprints := make([]string, 0, len(keys))
	for _, key := range keys {
		fingerprint, _, err := ssh.KeyFingerprint(key)
		if err != nil {
			return errors.Capture(err)
		}
		fingerprints = append(fingerprints, fingerprint)
	}
	return ssh.DeleteKeysFromFile(bootstrapSSHUser, "authorized_keys", fingerprints)
}

// IAASAgentFinalizer selects the machine identity and nonce for fresh
// bootstrap.
// A restoration operation can call FinaliseIAASAgent with its chosen identity.
func IAASAgentFinalizer(
	ctx context.Context,
	agentPasswordService AgentPasswordService,
	machineService MachineService,
	bootstrapParams instancecfg.StateInitializationParams,
	agentPassword string,
) error {
	return FinaliseIAASAgent(ctx, agentPasswordService, machineService,
		agent.BootstrapControllerId, agent.BootstrapNonce, bootstrapParams, agentPassword)
}

// K8sAgentFinalizer selects the controller identity and nonce file for fresh
// bootstrap. A restoration operation supplies its own identity to
// FinaliseK8sAgent.
func K8sAgentFinalizer(
	ctx context.Context,
	agentPasswordService AgentPasswordService,
	_ MachineService,
	_ instancecfg.StateInitializationParams,
	agentPassword string,
) error {
	return FinaliseK8sAgent(ctx, agentPasswordService, agent.BootstrapControllerId,
		agentPassword, k8sconstants.ControllerNonceFilePath)
}

// FreshBootstrapConfig contains the dependencies for a fresh controller.
type FreshBootstrapConfig struct {
	// RemoveBootstrapSSHKeys removes the bootstrap-only SSH keys from the
	// machine.
	RemoveBootstrapSSHKeys func([]string) error

	ControllerAgentBinaryStore AgentBinaryStore
	ControllerConfigService    ControllerConfigService
	ControllerNodeService      ControllerNodeService
	CloudService               CloudService
	UserService                UserService
	StorageService             StorageService
	AgentPasswordService       AgentPasswordService
	ApplicationService         ApplicationService
	ControllerModel            coremodel.Model
	ModelConfigService         ModelConfigService
	ModelInfoService           ModelInfoService
	MachineService             MachineService
	KeyManagerService          KeyManagerService
	NetworkService             NetworkService
	BakeryConfigService        BakeryConfigService
	BootstrapAddressFinder     BootstrapAddressFinderFunc
	DataDir                    string
	APIPort                    int
	AgentBinaryUploader        AgentBinaryBootstrapFunc
	ControllerCharmDeployer    ControllerCharmDeployerFunc
	PopulateControllerCharm    PopulateControllerCharmFunc
	AgentFinalizer             AgentFinalizerFunc
	AgentPassword              string
	ApplicationPassword        string
	CharmhubHTTPClient         HTTPClient
	UnitPassword               string
	ServiceManagerGetter       ServiceManagerGetterFunc
	Logger                     logger.Logger
	Clock                      clock.Clock
}

// Validate ensures that the config values are valid.
func (c *FreshBootstrapConfig) Validate() error {
	if c.ControllerAgentBinaryStore == nil {
		return jujuerrors.NotValidf("nil ControllerAgentBinaryStore")
	}
	if c.ControllerConfigService == nil {
		return jujuerrors.NotValidf("nil ControllerConfigService")
	}
	if c.ControllerNodeService == nil {
		return jujuerrors.NotValidf("nil ControllerNodeService")
	}
	if c.CloudService == nil {
		return jujuerrors.NotValidf("nil CloudService")
	}
	if c.UserService == nil {
		return jujuerrors.NotValidf("nil UserService")
	}
	if c.StorageService == nil {
		return jujuerrors.NotValidf("nil StorageService")
	}
	if c.AgentPasswordService == nil {
		return jujuerrors.NotValidf("nil AgentPasswordService")
	}
	if c.ApplicationService == nil {
		return jujuerrors.NotValidf("nil ApplicationService")
	}
	if c.ModelConfigService == nil {
		return jujuerrors.NotValidf("nil ModelConfigService")
	}
	if c.MachineService == nil {
		return jujuerrors.NotValidf("nil MachineService")
	}
	if c.KeyManagerService == nil {
		return jujuerrors.NotValidf("nil KeyManagerService")
	}
	if c.DataDir == "" {
		return jujuerrors.NotValidf("missing DataDir")
	}
	if c.APIPort == 0 {
		return jujuerrors.NotValidf("missing APIPort")
	}
	if c.AgentBinaryUploader == nil {
		return jujuerrors.NotValidf("nil AgentBinaryUploader")
	}
	if c.NetworkService == nil {
		return jujuerrors.NotValidf("nil NetworkService")
	}
	if c.BakeryConfigService == nil {
		return jujuerrors.NotValidf("nil BakeryConfigService")
	}
	if c.ControllerCharmDeployer == nil {
		return jujuerrors.NotValidf("nil ControllerCharmDeployer")
	}
	if c.PopulateControllerCharm == nil {
		return jujuerrors.NotValidf("nil PopulateControllerCharm")
	}
	if c.CharmhubHTTPClient == nil {
		return jujuerrors.NotValidf("nil CharmhubHTTPClient")
	}
	if c.AgentFinalizer == nil {
		return jujuerrors.NotValidf("nil AgentFinalizer")
	}
	if c.AgentPassword == "" {
		return jujuerrors.NotValidf("missing AgentPassword")
	}
	if c.BootstrapAddressFinder == nil {
		return jujuerrors.NotValidf("nil BootstrapAddressFinder")
	}
	if err := c.ControllerModel.UUID.Validate(); err != nil {
		return errors.Errorf("controller model id: %w", err)
	}
	if c.Logger == nil {
		return jujuerrors.NotValidf("nil Logger")
	}
	if c.Clock == nil {
		return jujuerrors.NotValidf("nil Clock")
	}
	return nil
}

type freshBootstrap struct {
	cfg    FreshBootstrapConfig
	logger logger.Logger
}

// NewFreshBootstrap constructs the operation that seeds a fresh controller.
// Construction validates dependencies; the operation performs the work.
func NewFreshBootstrap(cfg FreshBootstrapConfig) (Operation, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	b := &freshBootstrap{cfg: cfg, logger: cfg.Logger.Child("worker")}
	return b.run, nil
}

func (b *freshBootstrap) run(ctx context.Context) (func(), error) {
	if err := b.seedMacaroonConfig(ctx); err != nil {
		return nil, errors.Errorf("initialising macaroon bakery config: %w", err)
	}

	// Insert all the initial users into the state.
	if err := b.seedInitialUsers(ctx); err != nil {
		return nil, errors.Errorf("inserting initial users: %w", err)
	}

	dataDir := b.cfg.DataDir

	// Seed the agent binary to the object store.
	cleanup, err := b.seedAgentBinary(ctx, dataDir)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Seed the controller charm to the object store.
	bootstrapParams, err := b.bootstrapParams(ctx, dataDir)
	if err != nil {
		return nil, errors.Errorf("getting bootstrap params: %w", err)
	}

	// Create the user specified storage pools.
	if err := b.seedStoragePools(ctx, bootstrapParams.StoragePools); err != nil {
		return nil, errors.Errorf("seeding storage pools: %w", err)
	}

	controllerConfig, err := b.cfg.ControllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Retrieve controller addresses needed to set the API host ports.
	bootstrapAddresses, err := b.cfg.BootstrapAddressFinder(ctx, bootstrapParams.BootstrapMachineInstanceId)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Load spaces from the underlying substrate.
	if err := b.cfg.NetworkService.ReloadSpaces(ctx); err != nil {
		if !errors.Is(err, jujuerrors.NotSupported) {
			return nil, errors.Capture(err)
		}
		b.logger.Debugf(ctx, "reload spaces not supported due to a non-networking environment")
	}

	// Deploy the controller charm after calling reload spaces or
	// no subnets will be available for the ip address table with
	// kubernetes.
	if err := b.seedControllerCharm(ctx, dataDir, bootstrapParams, bootstrapAddresses); err != nil {
		return nil, errors.Capture(err)
	}
	if err := b.setControllerApplicationPassword(ctx); err != nil {
		return nil, errors.Capture(err)
	}

	if err := b.seedInitialAuthorisedKeys(ctx, bootstrapParams.ControllerModelAuthorizedKeys); err != nil {
		return nil, errors.Capture(err)
	}

	// Finalise the agent by either setting the machine as provisioned
	// or by setting the controller node password.
	if err := b.cfg.AgentFinalizer(ctx, b.cfg.AgentPasswordService, b.cfg.MachineService, bootstrapParams, b.cfg.AgentPassword); err != nil {
		return nil, errors.Errorf("finalising agent: %w", err)
	}

	// Fresh bootstrap creates controller 0. Restoration supplies its chosen
	// controller ID when publishing addresses with this helper.
	if err := InitialiseAPIHostPorts(ctx, b.cfg.ControllerNodeService,
		b.cfg.NetworkService, agent.BootstrapControllerId, controllerConfig,
		bootstrapAddresses, b.cfg.APIPort); err != nil {
		b.logger.Errorf(ctx, "unable to set API host ports %v:%w", bootstrapAddresses, err)
		return nil, errors.Capture(err)
	}

	if err := b.cfg.RemoveBootstrapSSHKeys(bootstrapParams.BootstrapSSHAuthorizedKeys); err != nil {
		return nil, errors.Errorf("removing bootstrap SSH keys: %w", err)
	}

	return cleanup, nil
}

func (b *freshBootstrap) setControllerApplicationPassword(ctx context.Context) error {
	if b.cfg.ApplicationPassword == "" {
		return nil
	}
	applicationUUID, err := b.cfg.ApplicationService.GetApplicationUUIDByName(
		ctx, environsbootstrap.ControllerApplicationName,
	)
	if err != nil {
		return errors.Errorf("getting controller application UUID: %w", err)
	}
	if err := b.cfg.AgentPasswordService.SetApplicationPassword(
		ctx, applicationUUID, b.cfg.ApplicationPassword,
	); err != nil {
		return errors.Errorf("setting controller application password: %w", err)
	}
	return nil
}

func (b *freshBootstrap) seedMacaroonConfig(ctx context.Context) error {
	err := b.cfg.BakeryConfigService.InitialiseBakeryConfig(ctx)
	if errors.Is(err, macaroonerrors.BakeryConfigAlreadyInitialised) {
		return nil
	}
	return errors.Capture(err)
}

func (b *freshBootstrap) seedInitialUsers(ctx context.Context) error {
	// Any failure should be retryable, so we can re-attempt to bootstrap.
	controllerCfg, err := b.cfg.ControllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	controllerUUID := controllerCfg.ControllerUUID()

	adminUser, err := b.cfg.UserService.GetUserByName(ctx, user.AdminUserName)
	if err != nil {
		return errors.Errorf("getting admin user %q: %w", user.AdminUserName, err)
	}

	pass, err := password.RandomPassword()
	if err != nil {
		return errors.Errorf("generating metrics password: %w", err)
	}
	metricsPassword := auth.NewPassword(pass)

	metricsName, err := user.NewName("juju-metrics")
	if err != nil {
		return errors.Capture(err)
	}
	_, _, err = b.cfg.UserService.AddUser(ctx, userservice.AddUserArg{
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

	err = b.cfg.UserService.AddExternalUser(
		ctx,
		permission.EveryoneUserName,
		"",
		adminUser.UUID,
	)
	if errors.Is(err, accesserrors.UserAlreadyExists) {
		return nil
	}

	if err != nil {
		return errors.Errorf("inserting initial users: %w", err)
	}
	return nil
}

// seedInitialAuthorisedKeys is responsible for adding any extra authorised keys
// requested during bootstrap to the admin user on the controller model. It is
// valid and safe to pass in a nil slice of keys to this function.
func (b *freshBootstrap) seedInitialAuthorisedKeys(
	ctx context.Context,
	keys []string,
) error {
	adminUser, err := b.cfg.UserService.GetUserByName(ctx, coremodel.ControllerModelOwnerUsername)
	if err != nil {
		return errors.Errorf(
			"cannot get %q user to seed %d authorised keys into the controller model: %w",
			coremodel.ControllerModelOwnerUsername,
			len(keys),
			err,
		)
	}

	err = b.cfg.KeyManagerService.AddPublicKeysForUser(ctx, adminUser.UUID, keys...)
	if err != nil {
		return errors.Errorf("cannot seed %d authorised keys into the controller model: %w",
			len(keys),
			err,
		)
	}

	return nil
}

// seedStoragePools is responsible for seeing the initial set of storage pools
// required for the newly created controller.
func (b *freshBootstrap) seedStoragePools(
	ctx context.Context,
	poolParams map[string]internalstorage.Attrs,
) error {
	err := b.cfg.ModelInfoService.SeedDefaultStoragePools(ctx)
	if err != nil {
		return errors.Errorf("seeding default storage pools into model: %w", err)
	}

	storagePoolsToCreate := initialStoragePools(poolParams)
	for _, p := range storagePoolsToCreate {
		_, err := b.cfg.StorageService.CreateStoragePool(
			ctx,
			p.Name,
			p.ProviderType,
			p.Attributes,
		)

		if errors.Is(err, storageerrors.StoragePoolAlreadyExists) {
			// If the user defined storage pool already exists, skip it.
			continue
		} else if err != nil {
			return errors.Errorf("creating bootstrap storage pool %q: %w", p.Name, err)
		}
	}
	return nil
}

func (b *freshBootstrap) seedAgentBinary(ctx context.Context, dataDir string) (func(), error) {
	cleanup, err := b.cfg.AgentBinaryUploader(
		ctx,
		dataDir,
		b.cfg.ControllerAgentBinaryStore,
		b.cfg.Logger.Child("agentbinary"),
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return cleanup, nil
}

func (b *freshBootstrap) seedControllerCharm(
	ctx context.Context,
	dataDir string,
	bootstrapArgs instancecfg.StateInitializationParams,
	bootstrapAddresses network.ProviderAddresses,
) error {
	controllerConfig, err := b.cfg.ControllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	// Controller charm seeder will populate the charm for the controller.
	deployer, err := b.cfg.ControllerCharmDeployer(ctx, ControllerCharmDeployerConfig{
		AgentPasswordService:        b.cfg.AgentPasswordService,
		ApplicationService:          b.cfg.ApplicationService,
		Model:                       b.cfg.ControllerModel,
		ModelConfigService:          b.cfg.ModelConfigService,
		ControllerConfig:            controllerConfig,
		DataDir:                     dataDir,
		BootstrapMachineConstraints: bootstrapArgs.BootstrapMachineConstraints,
		BootstrapAddresses:          bootstrapAddresses,
		ControllerCharmName:         bootstrapArgs.ControllerCharmPath,
		ControllerCharmChannel:      bootstrapArgs.ControllerCharmChannel,
		CharmhubHTTPClient:          b.cfg.CharmhubHTTPClient,
		UnitPassword:                b.cfg.UnitPassword,
		ServiceManagerGetter:        b.cfg.ServiceManagerGetter,
		Logger:                      b.cfg.Logger,
		Clock:                       b.cfg.Clock,
	})
	if err != nil {
		return errors.Capture(err)
	}

	return errors.Capture(b.cfg.PopulateControllerCharm(ctx, deployer))
}

func (b *freshBootstrap) bootstrapParams(ctx context.Context, dataDir string) (instancecfg.StateInitializationParams, error) {
	bootstrapParamsData, err := os.ReadFile(bootstrap.BootstrapParamsPath(dataDir))
	if err != nil {
		return instancecfg.StateInitializationParams{}, errors.Errorf("reading bootstrap params file: %w", err)
	}
	var args instancecfg.StateInitializationParams
	if err := args.Unmarshal(bootstrapParamsData); err != nil {
		return instancecfg.StateInitializationParams{}, errors.Capture(err)
	}
	return args, nil
}

// initialStoragePools extracts any storage pools included with the bootstrap
// params and returns them as a slice of [StoragePoolToCreate] values.
func initialStoragePools(params map[string]internalstorage.Attrs) []StoragePoolToCreate {
	retVal := make([]StoragePoolToCreate, 0, len(params))
	for name, attrs := range params {
		pType, _ := attrs[corestorage.BootstrapStoragePoolTypeKey].(string)
		// During bootstrap a client passes any initial storage pools that
		// should be created as an attribute map. This isn't an ideal
		// representation but the one we have. We MUST make sure we remove these
		// keys from the map before creating the storage pool(s) in the
		// controller.
		delete(attrs, corestorage.BootstrapStoragePoolNameKey)
		delete(attrs, corestorage.BootstrapStoragePoolTypeKey)
		retVal = append(retVal, StoragePoolToCreate{
			Attributes:   attrs,
			Name:         name,
			ProviderType: domainstorage.ProviderType(pType),
		})
	}
	return retVal
}
