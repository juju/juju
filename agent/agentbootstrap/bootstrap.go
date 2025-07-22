// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/lestrrat-go/jwx/v2/jwk"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloud"
	coreagent "github.com/juju/juju/core/agent"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/credential"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	userbootstrap "github.com/juju/juju/domain/access/bootstrap"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	cloudimagemetadatabootstrap "github.com/juju/juju/domain/cloudimagemetadata/bootstrap"
	controllerbootstrap "github.com/juju/juju/domain/controller/bootstrap"
	controllerconifgbootstrap "github.com/juju/juju/domain/controllerconfig/bootstrap"
	credbootstrap "github.com/juju/juju/domain/credential/bootstrap"
	modeldomain "github.com/juju/juju/domain/model"
	modelbootstrap "github.com/juju/juju/domain/model/bootstrap"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modelconfigbootstrap "github.com/juju/juju/domain/modelconfig/bootstrap"
	modeldefaultsbootstrap "github.com/juju/juju/domain/modeldefaults/bootstrap"
	secretbackendbootstrap "github.com/juju/juju/domain/secretbackend/bootstrap"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/state"
)

// DqliteInitializerFunc is a function that initializes the dqlite database
// for the controller.
type DqliteInitializerFunc func(
	ctx context.Context,
	mgr database.BootstrapNodeManager,
	modelUUID coremodel.UUID,
	logger logger.Logger,
	options ...database.BootstrapOpt,
) error

// CheckJWKSReachable checks if the given JWKS URL is reachable.
func CheckJWKSReachable(url string) error {
	ctx, cancelF := context.WithTimeout(context.TODO(), 30*time.Second)
	defer cancelF()
	_, err := jwk.Fetch(ctx, url)
	if err != nil {
		return errors.Annotatef(err, "failed to fetch jwks")
	}
	return nil
}

// AgentBootstrap is used to initialize the state for a new controller.
type AgentBootstrap struct {
	adminUser       names.UserTag
	agentConfig     agent.ConfigSetter
	stateNewPolicy  state.NewPolicyFunc
	bootstrapDqlite DqliteInitializerFunc

	stateInitializationParams instancecfg.StateInitializationParams

	// StorageProviderRegistry is used to determine and store the
	// details of the default storage pools.
	storageProviderRegistry storage.ProviderRegistry
	logger                  logger.Logger
}

// AgentBootstrapArgs are the arguments to NewAgentBootstrap that are required
// to NewAgentBootstrap.
type AgentBootstrapArgs struct {
	AdminUser                 names.UserTag
	AgentConfig               agent.ConfigSetter
	BootstrapEnviron          environs.BootstrapEnviron
	BootstrapMachineAddresses corenetwork.ProviderAddresses
	StateInitializationParams instancecfg.StateInitializationParams
	StorageProviderRegistry   storage.ProviderRegistry
	BootstrapDqlite           DqliteInitializerFunc
	Logger                    logger.Logger
}

func (a *AgentBootstrapArgs) validate() error {
	if a.BootstrapEnviron == nil {
		return errors.NotValidf("bootstrap environ")
	}
	if a.AdminUser == (names.UserTag{}) {
		return errors.NotValidf("admin user")
	}
	if a.AgentConfig == nil {
		return errors.NotValidf("agent config")
	}
	if a.StorageProviderRegistry == nil {
		return errors.NotValidf("storage provider registry")
	}
	if a.BootstrapDqlite == nil {
		return errors.NotValidf("bootstrap dqlite")
	}
	if a.Logger == nil {
		return errors.NotValidf("logger")
	}
	return nil
}

// NewAgentBootstrap creates a new AgentBootstrap, that can be used to
// initialize the state for a new controller.
// NewAgentBootstrap should be called with the bootstrap machine's agent
// configuration. It uses that information to create the controller, dial the
// controller, and initialize it. It also generates a new password for the
// bootstrap machine and calls Write to save the configuration.
//
// The cfg values will be stored in the state's ModelConfig; the
// machineCfg values will be used to configure the bootstrap Machine,
// and its constraints will be also be used for the model-level
// constraints. The connection to the controller will respect the
// given timeout parameter.
func NewAgentBootstrap(args AgentBootstrapArgs) (*AgentBootstrap, error) {
	if err := args.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return &AgentBootstrap{
		adminUser:                 args.AdminUser,
		agentConfig:               args.AgentConfig,
		bootstrapDqlite:           args.BootstrapDqlite,
		logger:                    args.Logger,
		stateInitializationParams: args.StateInitializationParams,
		storageProviderRegistry:   args.StorageProviderRegistry,
	}, nil
}

// Initialize returns the newly initialized state and bootstrap machine.
// If it fails, the state may well be irredeemably compromised.
// TODO (stickupkid): Split this function into testable smaller functions.
func (b *AgentBootstrap) Initialize(ctx context.Context) (resultErr error) {
	agentConfig := b.agentConfig
	if agentConfig.Tag().Id() != agent.BootstrapControllerId || !coreagent.IsAllowedControllerTag(agentConfig.Tag().Kind()) {
		return errors.Errorf("InitializeState not called with bootstrap controller's configuration")
	}
	controllerAgentInfo, ok := agentConfig.ControllerAgentInfo()
	if !ok {
		return errors.Errorf("controller agent info not available")
	}

	// N.B. no users are set up when we're initializing the state,
	// so don't use any tag or password when opening it.
	info, ok := agentConfig.MongoInfo()
	if !ok {
		return errors.Errorf("state info not available")
	}
	info.Tag = nil

	stateParams := b.stateInitializationParams

	// Add the controller model cloud and credential to the database.
	cloudCred, cloudCredTag, err := b.getCloudCredential()
	if err != nil {
		return errors.Annotate(err, "getting cloud credentials from args")
	}

	controllerUUID, err := uuid.UUIDFromString(stateParams.ControllerConfig.ControllerUUID())
	if err != nil {
		return fmt.Errorf("parsing controller uuid %q: %w", stateParams.ControllerConfig.ControllerUUID(), err)
	}

	controllerModelUUID := coremodel.UUID(
		stateParams.ControllerModelConfig.UUID(),
	)

	// Add initial Admin user to the database. This will return Admin user UUID
	// and a function to insert it into the database.
	adminUserUUID, addAdminUser := userbootstrap.AddUserWithPassword(
		user.NameFromTag(b.adminUser),
		auth.NewPassword(agentConfig.OldPassword()),
		permission.AccessSpec{
			Access: permission.SuperuserAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        controllerUUID.String(),
			},
		},
	)

	controllerModelArgs := modeldomain.GlobalModelCreationArgs{
		Name:        stateParams.ControllerModelConfig.Name(),
		AdminUsers:  []user.UUID{adminUserUUID},
		Qualifier:   coremodel.QualifierFromUserTag(b.adminUser),
		Cloud:       stateParams.ControllerCloud.Name,
		CloudRegion: stateParams.ControllerCloudRegion,
		Credential:  credential.KeyFromTag(cloudCredTag),
	}
	controllerModelCreateFunc := modelbootstrap.CreateGlobalModelRecord(controllerModelUUID, controllerModelArgs)

	controllerModelDefaults := modeldefaultsbootstrap.ModelDefaultsProvider(
		stateParams.ControllerInheritedConfig,
		stateParams.RegionInheritedConfig[stateParams.ControllerCloudRegion],
		stateParams.ControllerCloud.Type,
	)

	isCAAS := cloud.CloudIsCAAS(stateParams.ControllerCloud)
	modelType := state.ModelTypeIAAS
	if isCAAS {
		modelType = state.ModelTypeCAAS
	}

	agentVersion := stateParams.AgentVersion
	if agentVersion == semversion.Zero {
		agentVersion = jujuversion.Current
	}
	if agentVersion.Major != jujuversion.Current.Major || agentVersion.Minor != jujuversion.Current.Minor {
		return fmt.Errorf("%w %q during bootstrap", modelerrors.AgentVersionNotSupported, agentVersion)
	}

	// localModelRecordOP defines the bootstrap operation that should be run
	// to establish the local model record in the controller model's database.
	// We have two variants of this to handle the case when the user as set a
	// custom agent stream to use for the controller model.
	localModelRecordOp := modelbootstrap.CreateLocalModelRecord(
		controllerModelUUID, controllerUUID, agentVersion,
	)
	if stateParams.ControllerModelConfig.AgentStream() != "" {
		agentStream := coreagentbinary.AgentStream(stateParams.ControllerModelConfig.AgentStream())
		localModelRecordOp = modelbootstrap.CreateLocalModelRecordWithAgentStream(
			controllerModelUUID, controllerUUID, agentVersion, agentStream,
		)
	}

	databaseBootstrapOptions := []database.BootstrapOpt{
		// The controller config needs to be inserted before the admin users
		// because the admin users permissions require the controller UUID.
		controllerconifgbootstrap.InsertInitialControllerConfig(stateParams.ControllerConfig, controllerModelUUID),
		controllerbootstrap.InsertInitialController(controllerAgentInfo.Cert, controllerAgentInfo.PrivateKey, controllerAgentInfo.CAPrivateKey, controllerAgentInfo.SystemIdentity),
		// The admin user needs to be added before everything else that
		// requires being owned by a Juju user.
		addAdminUser,
		cloudbootstrap.InsertCloud(user.NameFromTag(b.adminUser), stateParams.ControllerCloud),
		credbootstrap.InsertCredential(credential.KeyFromTag(cloudCredTag), cloudCred),
		modeldefaultsbootstrap.SetCloudDefaults(stateParams.ControllerCloud.Name, stateParams.ControllerInheritedConfig),
		secretbackendbootstrap.CreateDefaultBackends(coremodel.ModelType(modelType)),
		controllerModelCreateFunc,
		localModelRecordOp,
		modelbootstrap.SetModelConstraints(stateParams.ModelConstraints),
		modelconfigbootstrap.SetModelConfig(
			controllerModelUUID, stateParams.ControllerModelConfig.AllAttrs(), controllerModelDefaults),
	}
	if !isCAAS {
		databaseBootstrapOptions = append(databaseBootstrapOptions,
			cloudimagemetadatabootstrap.AddCustomImageMetadata(
				clock.WallClock, stateParams.ControllerModelConfig.ImageStream(), stateParams.CustomImageMetadata),
		)
	}

	// If we're running caas, we need to bind to the loopback address
	// and eschew TLS termination.
	// This is to prevent dqlite to become all at sea when the controller pod
	// is rescheduled. This is only a temporary measure until we have HA
	// dqlite for k8s.
	isLoopbackPreferred := isCAAS

	if err := b.bootstrapDqlite(
		ctx,
		database.NewNodeManager(b.agentConfig, isLoopbackPreferred, b.logger, coredatabase.NoopSlowQueryLogger{}),
		controllerModelUUID,
		b.logger,
		databaseBootstrapOptions...,
	); err != nil {
		return errors.Trace(err)
	}

	b.logger.Debugf(ctx, "initializing address %v", info.Addrs)

	ctrl, err := state.Initialize(state.InitializeParams{
		SSHServerHostKey: stateParams.SSHServerHostKey,
		Clock:            clock.WallClock,
		ControllerModelArgs: state.ModelArgs{
			Name:            stateParams.ControllerModelConfig.Name(),
			UUID:            coremodel.UUID(stateParams.ControllerModelConfig.UUID()),
			Type:            modelType,
			Owner:           b.adminUser,
			CloudName:       stateParams.ControllerCloud.Name,
			CloudRegion:     stateParams.ControllerCloudRegion,
			CloudCredential: cloudCredTag,
		},
		StoragePools:              stateParams.StoragePools,
		CloudName:                 stateParams.ControllerCloud.Name,
		ControllerConfig:          stateParams.ControllerConfig,
		ControllerInheritedConfig: stateParams.ControllerInheritedConfig,
		RegionInheritedConfig:     stateParams.RegionInheritedConfig,
		NewPolicy:                 b.stateNewPolicy,
	})
	if err != nil {
		return errors.Errorf("failed to initialize state: %v", err)
	}
	b.logger.Debugf(ctx, "connected to initial state")
	defer func() {
		_ = ctrl.Close()
	}()
	b.agentConfig.SetControllerAgentInfo(controllerAgentInfo)

	// Create a new password. It is used down below to set  the agent's initial
	// API password in agent config.
	newPassword, err := password.RandomPassword()
	if err != nil {
		return err
	}

	b.agentConfig.SetPassword(newPassword)

	return nil
}

func (b *AgentBootstrap) getCloudCredential() (cloud.Credential, names.CloudCredentialTag, error) {
	var cloudCredentialTag names.CloudCredentialTag

	stateParams := b.stateInitializationParams
	if stateParams.ControllerCloudCredential != nil && stateParams.ControllerCloudCredentialName != "" {
		id := fmt.Sprintf(
			"%s/%s/%s",
			stateParams.ControllerCloud.Name,
			b.adminUser.Id(),
			stateParams.ControllerCloudCredentialName,
		)
		if !names.IsValidCloudCredential(id) {
			return cloud.Credential{}, cloudCredentialTag, errors.NotValidf("cloud credential UUID %q", id)
		}
		cloudCredentialTag = names.NewCloudCredentialTag(id)
		return *stateParams.ControllerCloudCredential, cloudCredentialTag, nil
	}
	return cloud.Credential{}, cloudCredentialTag, nil
}
