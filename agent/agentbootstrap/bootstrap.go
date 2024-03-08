// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbootstrap

import (
	stdcontext "context"
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v5"
	utilseries "github.com/juju/os/v2/series"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller/modelmanager"
	coreagent "github.com/juju/juju/core/agent"
	corebase "github.com/juju/juju/core/base"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	coremodel "github.com/juju/juju/core/model"
	corenetwork "github.com/juju/juju/core/network"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	ccbootstrap "github.com/juju/juju/domain/controllerconfig/bootstrap"
	"github.com/juju/juju/domain/credential"
	credbootstrap "github.com/juju/juju/domain/credential/bootstrap"
	machinebootstrap "github.com/juju/juju/domain/machine/bootstrap"
	modeldomain "github.com/juju/juju/domain/model"
	modelbootstrap "github.com/juju/juju/domain/model/bootstrap"
	modelconfigbootstrap "github.com/juju/juju/domain/modelconfig/bootstrap"
	modeldefaultsbootstrap "github.com/juju/juju/domain/modeldefaults/bootstrap"
	userbootstrap "github.com/juju/juju/domain/user/bootstrap"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/space"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/internal/network"
	"github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/state"
)

// DqliteInitializerFunc is a function that initializes the dqlite database
// for the controller.
type DqliteInitializerFunc func(
	ctx stdcontext.Context,
	mgr database.BootstrapNodeManager,
	logger database.Logger,
	concerns ...database.BootstrapConcern,
) error

// Logger describes methods for emitting log output.
type Logger interface {
	Errorf(string, ...any)
	Warningf(string, ...any)
	Debugf(string, ...any)
	Infof(string, ...any)

	// Logf is used to proxy Dqlite logs via this b.logger.
	Logf(level loggo.Level, msg string, args ...any)
}

// ProviderFunc is a function that returns an EnvironProvider.
type ProviderFunc func(string) (environs.EnvironProvider, error)

type bootstrapController interface {
	state.Authenticator
	Id() string
	SetMongoPassword(password string) error
}

// AgentBootstrap is used to initialize the state for a new controller.
type AgentBootstrap struct {
	bootstrapEnviron environs.BootstrapEnviron
	adminUser        names.UserTag
	agentConfig      agent.ConfigSetter
	mongoDialOpts    mongo.DialOpts
	stateNewPolicy   state.NewPolicyFunc
	precheckerGetter func(*state.State) (environs.InstancePrechecker, error)
	bootstrapDqlite  DqliteInitializerFunc

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
	logger                  Logger
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
	Logger                    Logger

	// Deprecated: use InstancePrechecker
	StateNewPolicy           state.NewPolicyFunc
	InstancePrecheckerGetter func(*state.State) (environs.InstancePrechecker, error)
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

		stateNewPolicy:   args.StateNewPolicy,
		precheckerGetter: args.InstancePrecheckerGetter,
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

	controllerModelType := coremodel.IAAS
	if cloud.CloudIsCAAS(stateParams.ControllerCloud) {
		controllerModelType = coremodel.CAAS
	}

	// Add initial Admin user to the database. This will return Admin user UUID
	// and a function to insert it into the database.
	adminUserUUID, addAdminUser := userbootstrap.AddUserWithPassword(b.adminUser.Name(), auth.NewPassword(info.Password))

	controllerModelUUID := coremodel.UUID(
		stateParams.ControllerModelConfig.UUID(),
	)
	controllerModelArgs := modeldomain.ModelCreationArgs{
		AgentVersion: stateParams.AgentVersion,
		Name:         stateParams.ControllerModelConfig.Name(),
		Owner:        adminUserUUID,
		Cloud:        stateParams.ControllerCloud.Name,
		CloudRegion:  stateParams.ControllerCloudRegion,
		Credential:   credential.IdFromTag(cloudCredTag),
		Type:         controllerModelType,
		UUID:         controllerModelUUID,
	}
	_, controllerModelCreateFunc := modelbootstrap.CreateModel(controllerModelArgs)

	controllerModelDefaults := modeldefaultsbootstrap.ModelDefaultsProvider(
		nil,
		stateParams.ControllerInheritedConfig,
		stateParams.RegionInheritedConfig[stateParams.ControllerCloudRegion])

	databaseBootstrapConcerns := []database.BootstrapConcern{
		database.BootstrapControllerConcern(
			// The admin user needs to be added before everything else that
			// requires being owned by a Juju user.
			addAdminUser,
			ccbootstrap.InsertInitialControllerConfig(stateParams.ControllerConfig),
			cloudbootstrap.InsertCloud(stateParams.ControllerCloud),
			credbootstrap.InsertCredential(credential.IdFromTag(cloudCredTag), cloudCred),
			cloudbootstrap.SetCloudDefaults(stateParams.ControllerCloud.Name, stateParams.ControllerInheritedConfig),
			controllerModelCreateFunc,
		),
		database.BootstrapModelConcern(controllerModelUUID,
			modelconfigbootstrap.SetModelConfig(stateParams.ControllerModelConfig, controllerModelDefaults),
		),
	}
	isCAAS := cloud.CloudIsCAAS(stateParams.ControllerCloud)
	if !isCAAS {
		// TODO(wallyworld) - this is just a placeholder for now
		databaseBootstrapConcerns = append(databaseBootstrapConcerns,
			database.BootstrapModelConcern(controllerModelUUID,
				machinebootstrap.InsertMachine(agent.BootstrapControllerId),
			))
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
		b.logger,
		databaseBootstrapConcerns...,
	); err != nil {
		return nil, errors.Trace(err)
	}

	session, err := b.initMongo(info.Info, b.mongoDialOpts, info.Password)
	if err != nil {
		return nil, errors.Annotate(err, "failed to initialize mongo")
	}
	defer session.Close()

	b.logger.Debugf("initializing address %v", info.Addrs)

	modelType := state.ModelTypeIAAS
	if isCAAS {
		modelType = state.ModelTypeCAAS
	}
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
	})
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

	// Fetch spaces from substrate.
	// We need to do this before setting the API host-ports,
	// because any space names in the bootstrap machine addresses must be
	// reconcilable with space IDs at that point.
	callContext := envcontext.WithoutCredentialInvalidator(ctx)
	if err = space.ReloadSpaces(callContext, space.NewState(st), b.bootstrapEnviron); err != nil {
		if !errors.Is(err, errors.NotSupported) {
			return nil, errors.Trace(err)
		}
		b.logger.Debugf("Not performing spaces load on a non-networking environment")
	}

	// Verify model config DefaultSpace exists now that
	// spaces have been loaded.
	if err := b.verifyModelConfigDefaultSpace(st); err != nil {
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

	if err := b.ensureInitialModel(ctx, cloudSpec, provider, st, ctrl, cloudCredTag); err != nil {
		return nil, errors.Annotate(err, "ensuring initial model")
	}
	return ctrl, nil
}

func (b *AgentBootstrap) verifyModelConfigDefaultSpace(st *state.State) error {
	m, err := st.Model()
	if err != nil {
		return err
	}
	mCfg, err := m.Config()
	if err != nil {
		return err
	}

	name := mCfg.DefaultSpace()
	if name == "" {
		// No need to verify if a space isn't defined.
		return nil
	}

	_, err = st.SpaceByName(name)
	if errors.Is(err, errors.NotFound) {
		return fmt.Errorf("model %q default space %q %w", m.Name(), name, errors.NotFound)
	} else if err != nil {
		return fmt.Errorf("cannot verify default space %q for model %q: %w", name, m.Name(), err)
	}
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
			return cloud.Credential{}, cloudCredentialTag, errors.NotValidf("cloud credential ID %q", id)
		}
		cloudCredentialTag = names.NewCloudCredentialTag(id)
		return *stateParams.ControllerCloudCredential, cloudCredentialTag, nil
	}
	return cloud.Credential{}, cloudCredentialTag, nil
}

// ensureInitialModel ensures the initial model.
func (b *AgentBootstrap) ensureInitialModel(
	ctx stdcontext.Context,
	cloudSpec environscloudspec.CloudSpec,
	provider environs.EnvironProvider,
	st *state.State,
	ctrl *state.Controller,
	cloudCredentialTag names.CloudCredentialTag,
) error {
	stateParams := b.stateInitializationParams
	if len(stateParams.InitialModelConfig) == 0 {
		b.logger.Debugf("no initial model configured")
		return nil
	}

	// Create the initial hosted model, with the model config passed to
	// bootstrap, which contains the UUID, name for the model,
	// and any user supplied config. We also copy the authorized-keys
	// from the controller model.
	attrs := make(map[string]any)
	for k, v := range stateParams.InitialModelConfig {
		attrs[k] = v
	}
	attrs[config.AuthorizedKeysKey] = stateParams.ControllerModelConfig.AuthorizedKeys()

	creator := modelmanager.ModelConfigCreator{Provider: b.provider}
	initialModelConfig, err := creator.NewModelConfig(
		cloudSpec, stateParams.ControllerModelConfig, attrs,
	)
	if err != nil {
		return errors.Annotate(err, "creating initial model config")
	}
	controllerUUID := stateParams.ControllerConfig.ControllerUUID()

	initialModelEnv, err := b.getEnviron(ctx, controllerUUID, cloudSpec, initialModelConfig, provider)
	if err != nil {
		return errors.Annotate(err, "opening initial model environment")
	}

	callCtx := envcontext.WithoutCredentialInvalidator(ctx)
	if err := initialModelEnv.Create(
		callCtx,
		environs.CreateParams{
			ControllerUUID: controllerUUID,
		}); err != nil {
		return errors.Annotate(err, "creating initial model environment")
	}

	ctrlModel, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	model, initialModelState, err := ctrl.NewModel(state.ModelArgs{
		Type:                    ctrlModel.Type(),
		Owner:                   b.adminUser,
		Config:                  initialModelConfig,
		Constraints:             stateParams.ModelConstraints,
		CloudName:               stateParams.ControllerCloud.Name,
		CloudRegion:             stateParams.ControllerCloudRegion,
		CloudCredential:         cloudCredentialTag,
		StorageProviderRegistry: b.storageProviderRegistry,
		EnvironVersion:          provider.Version(),
	})
	if err != nil {
		return errors.Annotate(err, "creating initial model")
	}
	defer func() { _ = initialModelState.Close() }()

	if err := model.AutoConfigureContainerNetworking(initialModelEnv); err != nil {
		return errors.Annotate(err, "autoconfiguring container networking")
	}

	// TODO(wpk) 2017-05-24 Copy subnets/spaces from controller model
	if err = space.ReloadSpaces(callCtx, space.NewState(initialModelState), initialModelEnv); err != nil {
		if errors.Is(err, errors.NotSupported) {
			b.logger.Debugf("Not performing spaces load on a non-networking environment")
		} else {
			return errors.Annotate(err, "fetching initial model spaces")
		}
	}
	return nil
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

	hostSeries, err := utilseries.HostSeries()
	if err != nil {
		return nil, errors.Trace(err)
	}

	base, err := corebase.GetBaseFromSeries(hostSeries)
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
