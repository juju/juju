// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/lestrrat-go/jwx/v3/jwk"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
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
	controllerconfigbootstrap "github.com/juju/juju/domain/controllerconfig/bootstrap"
	credbootstrap "github.com/juju/juju/domain/credential/bootstrap"
	modeldomain "github.com/juju/juju/domain/model"
	modelbootstrap "github.com/juju/juju/domain/model/bootstrap"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modelconfigbootstrap "github.com/juju/juju/domain/modelconfig/bootstrap"
	modeldefaultsbootstrap "github.com/juju/juju/domain/modeldefaults/bootstrap"
	secretbackendbootstrap "github.com/juju/juju/domain/secretbackend/bootstrap"
	sshbootstrap "github.com/juju/juju/domain/ssh/bootstrap"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/uuid"
)

// DqliteInitialiserFunc is a function that initialises the Dqlite database
// for the controller.
type DqliteInitialiserFunc func(
	ctx context.Context,
	mgr database.BootstrapNodeManager,
	bootstrapAddresses corenetwork.ProviderAddresses,
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
		return errors.Errorf("failed to fetch jwks: %w", err)
	}
	return nil
}

// AgentBootstrap is used to initialise the state for a new controller.
type AgentBootstrap struct {
	adminUser                 names.UserTag
	agentConfig               agent.ConfigSetter
	bootstrapDqlite           DqliteInitialiserFunc
	bootstrapMachineAddresses corenetwork.ProviderAddresses

	stateInitialisationParams instancecfg.StateInitializationParams

	clock clock.Clock
	// StorageProviderRegistry is used to determine and store the
	// details of the default storage pools.
	logger logger.Logger
}

// AgentBootstrapArgs are the arguments to NewAgentBootstrap that are required
// to NewAgentBootstrap.
type AgentBootstrapArgs struct {
	AdminUser                 names.UserTag
	AgentConfig               agent.ConfigSetter
	BootstrapEnviron          environs.BootstrapEnviron
	BootstrapMachineAddresses corenetwork.ProviderAddresses
	StateInitialisationParams instancecfg.StateInitializationParams
	BootstrapDqlite           DqliteInitialiserFunc
	Logger                    logger.Logger
}

func (a *AgentBootstrapArgs) validate() error {
	if a.BootstrapEnviron == nil {
		return jujuerrors.NotValidf("bootstrap environ")
	}
	if a.AdminUser == (names.UserTag{}) {
		return jujuerrors.NotValidf("admin user")
	}
	if a.AgentConfig == nil {
		return jujuerrors.NotValidf("agent config")
	}

	if a.BootstrapDqlite == nil {
		return jujuerrors.NotValidf("bootstrap dqlite")
	}
	if a.Logger == nil {
		return jujuerrors.NotValidf("logger")
	}
	return nil
}

// NewAgentBootstrap constructs the state initialiser for a fresh controller.
// The caller supplies the bootstrap agent configuration and is responsible for
// persisting its changes after Initialise succeeds.
func NewAgentBootstrap(args AgentBootstrapArgs) (*AgentBootstrap, error) {
	if err := args.validate(); err != nil {
		return nil, errors.Capture(err)
	}
	return &AgentBootstrap{
		adminUser:                 args.AdminUser,
		agentConfig:               args.AgentConfig,
		bootstrapDqlite:           args.BootstrapDqlite,
		bootstrapMachineAddresses: args.BootstrapMachineAddresses,
		clock:                     clock.WallClock,
		logger:                    args.Logger,
		stateInitialisationParams: args.StateInitialisationParams,
	}, nil
}

// Initialise seeds a fresh controller and updates its in-memory agent
// configuration after database initialisation succeeds. Database failures may
// leave partial state; the caller must not start the agent after a failure.
func (b *AgentBootstrap) Initialise(ctx context.Context) error {
	agentConfig := b.agentConfig
	if agentConfig.Tag().Id() != agent.BootstrapControllerId || !coreagent.IsAllowedControllerTag(agentConfig.Tag().Kind()) {
		return errors.Errorf("Initialise not called with bootstrap controller's configuration")
	}
	controllerAgentInfo, ok := agentConfig.ControllerAgentInfo()
	if !ok {
		return errors.Errorf("controller agent info not available")
	}

	controllerModelUUID, seedOperations, err := b.prepareFreshState(controllerAgentInfo)
	if err != nil {
		return errors.Capture(err)
	}

	if err := b.initialiseDqlite(
		ctx, controllerModelUUID, seedOperations...,
	); err != nil {
		return errors.Capture(err)
	}

	b.agentConfig.SetControllerAgentInfo(controllerAgentInfo)

	// Generate the agent password only after its database state is ready.
	newPassword, err := password.RandomPassword()
	if err != nil {
		return err
	}

	b.agentConfig.SetPassword(newPassword)

	return nil
}

// prepareFreshState builds the seed operations for a fresh controller without
// opening databases or changing the agent configuration. Restoration supplies
// its own state population workflow rather than modifying these seed operations.
func (b *AgentBootstrap) prepareFreshState(
	controllerAgentInfo controller.ControllerAgentInfo,
) (coremodel.UUID, []database.BootstrapOpt, error) {
	agentConfig := b.agentConfig
	stateParams := b.stateInitialisationParams

	// Add the controller model cloud and credential to the database.
	cloudCred, cloudCredTag, err := b.getCloudCredential()
	if err != nil {
		return "", nil, errors.Errorf("getting cloud credentials from args: %w", err)
	}

	controllerUUID, err := uuid.UUIDFromString(stateParams.ControllerConfig.ControllerUUID())
	if err != nil {
		return "", nil, errors.Errorf("parsing controller uuid %q: %w", stateParams.ControllerConfig.ControllerUUID(), err)
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
		b.clock.Now().UTC(),
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

	isK8s := cloud.CloudIsCAAS(stateParams.ControllerCloud)
	modelType := coremodel.IAAS
	if isK8s {
		modelType = coremodel.CAAS
	}

	agentVersion := stateParams.AgentVersion
	if agentVersion == semversion.Zero {
		agentVersion = jujuversion.Current
	}
	if agentVersion.Major != jujuversion.Current.Major || agentVersion.Minor != jujuversion.Current.Minor {
		return "", nil, errors.Errorf("%w %q during bootstrap", modelerrors.AgentVersionNotSupported, agentVersion)
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
		controllerconfigbootstrap.InsertInitialControllerConfig(stateParams.ControllerConfig, controllerModelUUID),
		controllerbootstrap.InsertInitialController(controllerAgentInfo.Cert, controllerAgentInfo.PrivateKey, controllerAgentInfo.CAPrivateKey, controllerAgentInfo.SystemIdentity),
		sshbootstrap.InsertInitialSSHServerHostKey(stateParams.SSHServerHostKey),
		// The admin user needs to be added before everything else that
		// requires being owned by a Juju user.
		addAdminUser,
		cloudbootstrap.InsertCloud(user.NameFromTag(b.adminUser), stateParams.ControllerCloud),
		credbootstrap.InsertCredential(credential.KeyFromTag(cloudCredTag), cloudCred),
		modeldefaultsbootstrap.SetCloudDefaults(stateParams.ControllerCloud.Name, stateParams.ControllerInheritedConfig),
		secretbackendbootstrap.CreateDefaultBackends(modelType),
		controllerModelCreateFunc,
		localModelRecordOp,
		modelbootstrap.SetModelConstraints(stateParams.ModelConstraints),
		modelconfigbootstrap.SetModelConfig(
			controllerModelUUID, stateParams.ControllerModelConfig.AllAttrs(), controllerModelDefaults),
	}
	if !isK8s {
		databaseBootstrapOptions = append(databaseBootstrapOptions,
			cloudimagemetadatabootstrap.AddCustomImageMetadata(
				b.clock, stateParams.ControllerModelConfig.ImageStream(), stateParams.CustomImageMetadata),
		)
	}

	return controllerModelUUID, databaseBootstrapOptions, nil
}

func (b *AgentBootstrap) initialiseDqlite(
	ctx context.Context, controllerModelUUID coremodel.UUID,
	options ...database.BootstrapOpt,
) error {
	agentInfo, _ := b.agentConfig.ControllerAgentInfo()
	nodeManagerCfg := database.NodeManagerConfig{
		DataDir:              b.agentConfig.DataDir(),
		CACert:               b.agentConfig.CACert(),
		ControllerCert:       agentInfo.Cert,
		ControllerPrivateKey: agentInfo.PrivateKey,
	}
	return b.bootstrapDqlite(
		ctx,
		database.NewNodeManager(
			nodeManagerCfg, b.logger, coredatabase.NoopSlowQueryLogger{},
		),
		b.bootstrapMachineAddresses,
		controllerModelUUID,
		b.logger,
		options...,
	)
}

func (b *AgentBootstrap) getCloudCredential() (cloud.Credential, names.CloudCredentialTag, error) {
	var cloudCredentialTag names.CloudCredentialTag

	stateParams := b.stateInitialisationParams
	if stateParams.ControllerCloudCredential != nil && stateParams.ControllerCloudCredentialName != "" {
		id := fmt.Sprintf(
			"%s/%s/%s",
			stateParams.ControllerCloud.Name,
			b.adminUser.Id(),
			stateParams.ControllerCloudCredentialName,
		)
		if !names.IsValidCloudCredential(id) {
			return cloud.Credential{}, cloudCredentialTag, jujuerrors.NotValidf("cloud credential UUID %q", id)
		}
		cloudCredentialTag = names.NewCloudCredentialTag(id)
		return *stateParams.ControllerCloudCredential, cloudCredentialTag, nil
	}
	return cloud.Credential{}, cloudCredentialTag, nil
}
