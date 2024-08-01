// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbootstrap

import (
	stdcontext "context"
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v5"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cloud"
	coreagent "github.com/juju/juju/core/agent"
	"github.com/juju/juju/core/credential"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	corenetwork "github.com/juju/juju/core/network"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/permission"
	userbootstrap "github.com/juju/juju/domain/access/bootstrap"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	ccbootstrap "github.com/juju/juju/domain/controllerconfig/bootstrap"
	credbootstrap "github.com/juju/juju/domain/credential/bootstrap"
	machinebootstrap "github.com/juju/juju/domain/machine/bootstrap"
	modeldomain "github.com/juju/juju/domain/model"
	modelbootstrap "github.com/juju/juju/domain/model/bootstrap"
	modelconfigbootstrap "github.com/juju/juju/domain/modelconfig/bootstrap"
	modeldefaultsbootstrap "github.com/juju/juju/domain/modeldefaults/bootstrap"
	secretbackendbootstrap "github.com/juju/juju/domain/secretbackend/bootstrap"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/internal/network"
	"github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/state"
)

// DqliteInitializerFunc is a function that initializes the dqlite database
// for the controller.
type DqliteInitializerFunc func(
	ctx stdcontext.Context,
	mgr database.BootstrapNodeManager,
	modelUUID model.UUID,
	logger logger.Logger,
	options ...database.BootstrapOpt,
) error

// ProviderFunc is a function that returns an EnvironProvider.
type ProviderFunc func(string) (environs.EnvironProvider, error)

type bootstrapController interface {
	state.Authenticator
	Id() string
	SetMongoPassword(password string) error
}

// AgentBootstrap is used to initialize the state for a new controller.
type AgentBootstrap struct {
	bootstrapEnviron         environs.BootstrapEnviron
	adminUser                names.UserTag
	agentConfig              agent.ConfigSetter
	mongoDialOpts            mongo.DialOpts
	stateNewPolicy           state.NewPolicyFunc
	precheckerGetter         func(*state.State) (environs.InstancePrechecker, error)
	configSchemaSourceGetter config.ConfigSchemaSourceGetter
	bootstrapDqlite          DqliteInitializerFunc

	stateInitializationParams instancecfg.StateInitializationParams
	// BootstrapMachineAddresses holds the bootstrap machine's addresses.
	bootstrapMachineAddresses corenetwork.ProviderAddresses

	// BootstrapMachineJobs holds the jobs that the bootstrap machine
	// agent will run.
	bootstrapMachineJobs []model.MachineJob

	// SharedSecret is the Mongo replica set shared secret (keyfile).
	sharedSecret string

	// Provider is called to obtain an EnvironProvider.
	provider func(string) (environs.EnvironProvider, error)

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
	BootstrapMachineJobs      []model.MachineJob
	MongoDialOpts             mongo.DialOpts
	SharedSecret              string
	StateInitializationParams instancecfg.StateInitializationParams
	StorageProviderRegistry   storage.ProviderRegistry
	BootstrapDqlite           DqliteInitializerFunc
	Provider                  ProviderFunc
	Logger                    logger.Logger

	// Deprecated: use InstancePrechecker
	StateNewPolicy           state.NewPolicyFunc
	InstancePrecheckerGetter func(*state.State) (environs.InstancePrechecker, error)
	ConfigSchemaSourceGetter config.ConfigSchemaSourceGetter
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
	if a.SharedSecret == "" {
		return errors.NotValidf("shared secret")
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
		bootstrapEnviron:          args.BootstrapEnviron,
		bootstrapMachineAddresses: args.BootstrapMachineAddresses,
		bootstrapMachineJobs:      args.BootstrapMachineJobs,
		logger:                    args.Logger,
		mongoDialOpts:             args.MongoDialOpts,
		provider:                  args.Provider,
		sharedSecret:              args.SharedSecret,
		stateInitializationParams: args.StateInitializationParams,
		storageProviderRegistry:   args.StorageProviderRegistry,

		stateNewPolicy:           args.StateNewPolicy,
		precheckerGetter:         args.InstancePrecheckerGetter,
		configSchemaSourceGetter: args.ConfigSchemaSourceGetter,
	}, nil
}

// Initialize returns the newly initialized state and bootstrap machine.
// If it fails, the state may well be irredeemably compromised.
// TODO (stickupkid): Split this function into testable smaller functions.
func (b *AgentBootstrap) Initialize(ctx stdcontext.Context) (_ *state.Controller, resultErr error) {
	agentConfig := b.agentConfig
	if agentConfig.Tag().Id() != agent.BootstrapControllerId || !coreagent.IsAllowedControllerTag(agentConfig.Tag().Kind()) {
		return nil, errors.Errorf("InitializeState not called with bootstrap controller's configuration")
	}
	servingInfo, ok := agentConfig.StateServingInfo()
	if !ok {
		return nil, errors.Errorf("state serving information not available")
	}
	// N.B. no users are set up when we're initializing the state,
	// so don't use any tag or password when opening it.
	info, ok := agentConfig.MongoInfo()
	if !ok {
		return nil, errors.Errorf("state info not available")
	}
	info.Tag = nil
	info.Password = agentConfig.OldPassword()

	stateParams := b.stateInitializationParams

	// Add the controller model cloud and credential to the database.
	cloudCred, cloudCredTag, err := b.getCloudCredential()
	if err != nil {
		return nil, errors.Annotate(err, "getting cloud credentials from args")
	}

	controllerUUID, err := uuid.UUIDFromString(stateParams.ControllerConfig.ControllerUUID())
	if err != nil {
		return nil, fmt.Errorf("parsing controller uuid %q: %w", stateParams.ControllerConfig.ControllerUUID(), err)
	}

	controllerModelUUID := model.UUID(
		stateParams.ControllerModelConfig.UUID(),
	)

	// Add initial Admin user to the database. This will return Admin user UUID
	// and a function to insert it into the database.
	adminUserUUID, addAdminUser := userbootstrap.AddUserWithPassword(
		b.adminUser.Name(),
		auth.NewPassword(info.Password),
		permission.AccessSpec{
			Access: permission.SuperuserAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        controllerUUID.String(),
			},
		},
	)

	controllerModelArgs := modeldomain.ModelCreationArgs{
		AgentVersion: stateParams.AgentVersion,
		Name:         stateParams.ControllerModelConfig.Name(),
		Owner:        adminUserUUID,
		Cloud:        stateParams.ControllerCloud.Name,
		CloudRegion:  stateParams.ControllerCloudRegion,
		Credential:   credential.KeyFromTag(cloudCredTag),
	}
	controllerModelCreateFunc := modelbootstrap.CreateModel(controllerModelUUID, controllerModelArgs)

	controllerModelDefaults := modeldefaultsbootstrap.ModelDefaultsProvider(
		stateParams.ControllerInheritedConfig,
		stateParams.RegionInheritedConfig[stateParams.ControllerCloudRegion])

	isCAAS := cloud.CloudIsCAAS(stateParams.ControllerCloud)
	modelType := state.ModelTypeIAAS
	if isCAAS {
		modelType = state.ModelTypeCAAS
	}

	databaseBootstrapOptions := []database.BootstrapOpt{
		// The controller config needs to be inserted before the admin users
		// because the admin users permissions require the controller UUID.
		ccbootstrap.InsertInitialControllerConfig(stateParams.ControllerConfig, controllerModelUUID),
		// The admin user needs to be added before everything else that
		// requires being owned by a Juju user.
		addAdminUser,
		cloudbootstrap.InsertCloud(b.adminUser.Name(), stateParams.ControllerCloud),
		credbootstrap.InsertCredential(credential.KeyFromTag(cloudCredTag), cloudCred),
		cloudbootstrap.SetCloudDefaults(stateParams.ControllerCloud.Name, stateParams.ControllerInheritedConfig),
		secretbackendbootstrap.CreateDefaultBackends(model.ModelType(modelType)),
		controllerModelCreateFunc,
		modelbootstrap.CreateReadOnlyModel(controllerModelUUID, controllerUUID),
		modelconfigbootstrap.SetModelConfig(controllerModelUUID, stateParams.ControllerModelConfig.AllAttrs(), controllerModelDefaults),
	}
	if !isCAAS {
		// TODO(wallyworld) - this is just a placeholder for now
		databaseBootstrapOptions = append(databaseBootstrapOptions,
			machinebootstrap.InsertMachine(agent.BootstrapControllerId),
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
		return nil, errors.Trace(err)
	}

	session, err := b.initMongo(info.Info, b.mongoDialOpts, info.Password)
	if err != nil {
		return nil, errors.Annotate(err, "failed to initialize mongo")
	}
	defer session.Close()

	b.logger.Debugf("initializing address %v", info.Addrs)

	ctrl, err := state.Initialize(state.InitializeParams{
		Clock: clock.WallClock,
		ControllerModelArgs: state.ModelArgs{
			Type:                    modelType,
			Owner:                   b.adminUser,
			Config:                  stateParams.ControllerModelConfig,
			Constraints:             stateParams.ModelConstraints,
			CloudName:               stateParams.ControllerCloud.Name,
			CloudRegion:             stateParams.ControllerCloudRegion,
			CloudCredential:         cloudCredTag,
			StorageProviderRegistry: b.storageProviderRegistry,
			EnvironVersion:          stateParams.ControllerModelEnvironVersion,
		},
		StoragePools:              stateParams.StoragePools,
		CloudName:                 stateParams.ControllerCloud.Name,
		ControllerConfig:          stateParams.ControllerConfig,
		ControllerInheritedConfig: stateParams.ControllerInheritedConfig,
		RegionInheritedConfig:     stateParams.RegionInheritedConfig,
		MongoSession:              session,
		AdminPassword:             info.Password,
		NewPolicy:                 b.stateNewPolicy,
	}, b.configSchemaSourceGetter)
	if err != nil {
		return nil, errors.Errorf("failed to initialize state: %v", err)
	}
	b.logger.Debugf("connected to initial state")
	defer func() {
		if resultErr != nil {
			_ = ctrl.Close()
		}
	}()
	servingInfo.SharedSecret = b.sharedSecret
	b.agentConfig.SetStateServingInfo(servingInfo)

	// Filter out any LXC or LXD bridge addresses from the machine addresses.
	filteredBootstrapMachineAddresses := network.FilterBridgeAddresses(b.bootstrapMachineAddresses)

	st, err := ctrl.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := st.SetStateServingInfo(servingInfo); err != nil {
		return nil, errors.Errorf("cannot set state serving info: %v", err)
	}

	cloudSpec, err := environscloudspec.MakeCloudSpec(
		stateParams.ControllerCloud,
		stateParams.ControllerCloudRegion,
		stateParams.ControllerCloudCredential,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpec.IsControllerCloud = true

	provider, err := b.provider(cloudSpec.Type)
	if err != nil {
		return nil, errors.Annotate(err, "getting environ provider")
	}

	var controllerNode bootstrapController
	if isCAAS {
		if controllerNode, err = b.initBootstrapNode(st); err != nil {
			return nil, errors.Annotate(err, "cannot initialize bootstrap controller")
		}
		if err := b.initControllerCloudService(ctx, cloudSpec, provider, st); err != nil {
			return nil, errors.Annotate(err, "cannot initialize cloud service")
		}
	} else {
		if controllerNode, err = b.initBootstrapMachine(st, filteredBootstrapMachineAddresses); err != nil {
			return nil, errors.Annotate(err, "cannot initialize bootstrap machine")
		}
	}

	// Sanity check.
	if controllerNode.Id() != agent.BootstrapControllerId {
		return nil, errors.Errorf("bootstrap controller expected id 0, got %q", controllerNode.Id())
	}

	// Read the machine agent's password and change it to
	// a new password (other agents will change their password
	// via the API connection).
	b.logger.Debugf("create new random password for controller %v", controllerNode.Id())

	newPassword, err := password.RandomPassword()
	if err != nil {
		return nil, err
	}
	if err := controllerNode.SetPassword(newPassword); err != nil {
		return nil, err
	}
	if err := controllerNode.SetMongoPassword(newPassword); err != nil {
		return nil, err
	}
	b.agentConfig.SetPassword(newPassword)

	return ctrl, nil
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
			return cloud.Credential{}, cloudCredentialTag, errors.NotValidf("cloud credential ID %q", id)
		}
		cloudCredentialTag = names.NewCloudCredentialTag(id)
		return *stateParams.ControllerCloudCredential, cloudCredentialTag, nil
	}
	return cloud.Credential{}, cloudCredentialTag, nil
}

func (b *AgentBootstrap) getEnviron(
	ctx stdcontext.Context,
	controllerUUID string,
	cloudSpec environscloudspec.CloudSpec,
	modelConfig *config.Config,
	provider environs.EnvironProvider,
) (env environs.BootstrapEnviron, err error) {
	openParams := environs.OpenParams{
		ControllerUUID: controllerUUID,
		Cloud:          cloudSpec,
		Config:         modelConfig,
	}
	if cloud.CloudTypeIsCAAS(cloudSpec.Type) {
		return caas.Open(ctx, provider, openParams)
	}
	return environs.Open(ctx, provider, openParams)
}

// initMongo dials the initial MongoDB connection, setting a
// password for the admin user, and returning the session.
func (b *AgentBootstrap) initMongo(info mongo.Info, dialOpts mongo.DialOpts, password string) (*mgo.Session, error) {
	session, err := mongo.DialWithInfo(mongo.MongoInfo{Info: info}, dialOpts)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := mongo.SetAdminMongoPassword(session, mongo.AdminUser, password); err != nil {
		session.Close()
		return nil, errors.Trace(err)
	}
	if err := mongo.Login(session, mongo.AdminUser, password); err != nil {
		session.Close()
		return nil, errors.Trace(err)
	}
	return session, nil
}

// initBootstrapMachine initializes the initial bootstrap machine in state.
func (b *AgentBootstrap) initBootstrapMachine(
	st *state.State,
	bootstrapMachineAddresses corenetwork.ProviderAddresses,
) (bootstrapController, error) {
	stateParams := b.stateInitializationParams
	b.logger.Infof("initialising bootstrap machine with config: %+v", stateParams)

	jobs := make([]state.MachineJob, len(b.bootstrapMachineJobs))
	for i, job := range b.bootstrapMachineJobs {
		machineJob, err := machineJobFromParams(job)
		if err != nil {
			return nil, errors.Errorf("invalid bootstrap machine job %q: %v", job, err)
		}
		jobs[i] = machineJob
	}
	var hardware instance.HardwareCharacteristics
	if stateParams.BootstrapMachineHardwareCharacteristics != nil {
		hardware = *stateParams.BootstrapMachineHardwareCharacteristics
	}

	base, err := coreos.HostBase()
	if err != nil {
		return nil, errors.Trace(err)
	}

	prechecker, err := b.precheckerGetter(st)
	if err != nil {
		return nil, errors.Annotate(err, "getting instance prechecker")
	}

	m, err := st.AddOneMachine(prechecker, state.MachineTemplate{
		Base:                    state.Base{OS: base.OS, Channel: base.Channel.String()},
		Nonce:                   agent.BootstrapNonce,
		Constraints:             stateParams.BootstrapMachineConstraints,
		InstanceId:              stateParams.BootstrapMachineInstanceId,
		HardwareCharacteristics: hardware,
		Jobs:                    jobs,
		DisplayName:             stateParams.BootstrapMachineDisplayName,
	})
	if err != nil {
		return nil, errors.Annotate(err, "cannot create bootstrap machine in state")
	}
	return m, nil
}

// initControllerCloudService creates cloud service for controller service.
func (b *AgentBootstrap) initControllerCloudService(
	ctx stdcontext.Context,
	cloudSpec environscloudspec.CloudSpec,
	provider environs.EnvironProvider,
	st *state.State,
) error {
	stateParams := b.stateInitializationParams
	controllerUUID := stateParams.ControllerConfig.ControllerUUID()
	env, err := b.getEnviron(ctx, controllerUUID, cloudSpec, stateParams.ControllerModelConfig, provider)
	if err != nil {
		return errors.Annotate(err, "getting environ")
	}

	broker, ok := env.(caas.ServiceManager)
	if !ok {
		// this should never happen.
		return errors.Errorf("environ %T does not implement ServiceManager interface", env)
	}
	svc, err := broker.GetService(ctx, k8sconstants.JujuControllerStackName, true)
	if err != nil {
		return errors.Trace(err)
	}

	if len(svc.Addresses) == 0 {
		// this should never happen because we have already checked in k8s controller bootstrap stacker.
		return errors.NotProvisionedf("k8s controller service %q address", svc.Id)
	}
	addrs := b.getAlphaSpaceAddresses(svc.Addresses)

	svcId := controllerUUID
	b.logger.Infof("creating cloud service for k8s controller %q", svcId)
	cloudSvc, err := st.SaveCloudService(state.SaveCloudServiceArgs{
		Id:         svcId,
		ProviderId: svc.Id,
		Addresses:  addrs,
	})
	b.logger.Debugf("created cloud service %v for controller", cloudSvc)
	return errors.Trace(err)
}

// getAlphaSpaceAddresses returns a SpaceAddresses created from the input
// providerAddresses and using the alpha space ID as their SpaceID.
// We set all the spaces of the output SpaceAddresses to be the alpha space ID.
func (b *AgentBootstrap) getAlphaSpaceAddresses(providerAddresses corenetwork.ProviderAddresses) corenetwork.SpaceAddresses {
	sas := make(corenetwork.SpaceAddresses, len(providerAddresses))
	for i, pa := range providerAddresses {
		sas[i] = corenetwork.SpaceAddress{MachineAddress: pa.MachineAddress}
		if pa.SpaceName != "" {
			sas[i].SpaceID = corenetwork.AlphaSpaceId
		}
	}
	return sas
}

// initBootstrapNode initializes the initial caas bootstrap controller in state.
func (b *AgentBootstrap) initBootstrapNode(
	st *state.State,
) (bootstrapController, error) {
	b.logger.Debugf("initialising bootstrap node for with config: %+v", b.stateInitializationParams)

	node, err := st.AddControllerNode()
	if err != nil {
		return nil, errors.Annotate(err, "cannot create bootstrap controller in state")
	}
	return node, nil
}

// machineJobFromParams returns the job corresponding to model.MachineJob.
// TODO(dfc) this function should live in apiserver/params, move there once
// state does not depend on apiserver/params
func machineJobFromParams(job model.MachineJob) (state.MachineJob, error) {
	switch job {
	case model.JobHostUnits:
		return state.JobHostUnits, nil
	case model.JobManageModel:
		return state.JobManageModel, nil
	default:
		return -1, errors.Errorf("invalid machine job %q", job)
	}
}
