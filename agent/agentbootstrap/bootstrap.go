// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbootstrap

import (
	stdcontext "context"
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v4"
	utilseries "github.com/juju/os/v2/series"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/controller/modelmanager"
	coreagent "github.com/juju/juju/core/agent"
	corebase "github.com/juju/juju/core/base"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/database"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	ccbootstrap "github.com/juju/juju/domain/controllerconfig/bootstrap"
	credbootstrap "github.com/juju/juju/domain/credential/bootstrap"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/space"
	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/internal/network"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.agent.agentbootstrap")

// InitializeStateParams holds parameters used for initializing the state
// database.
type InitializeStateParams struct {
	instancecfg.StateInitializationParams

	// BootstrapMachineAddresses holds the bootstrap machine's addresses.
	BootstrapMachineAddresses corenetwork.ProviderAddresses

	// BootstrapMachineJobs holds the jobs that the bootstrap machine
	// agent will run.
	BootstrapMachineJobs []model.MachineJob

	// SharedSecret is the Mongo replica set shared secret (keyfile).
	SharedSecret string

	// Provider is called to obtain an EnvironProvider.
	Provider func(string) (environs.EnvironProvider, error)

	// StorageProviderRegistry is used to determine and store the
	// details of the default storage pools.
	StorageProviderRegistry storage.ProviderRegistry
}

type bootstrapController interface {
	state.Authenticator
	Id() string
	SetMongoPassword(password string) error
}

// DqliteInitializerFunc is a function that initializes the dqlite database
// for the controller.
type DqliteInitializerFunc func(
	ctx stdcontext.Context,
	mgr database.BootstrapNodeManager,
	logger database.Logger,
	preferLoopback bool,
	ops ...database.BootstrapOpt,
) error

// AgentInitializer is used to initialize the state database.
type AgentInitializerConfig struct {
	BootstrapEnviron      environs.BootstrapEnviron
	AdminUser             names.UserTag
	AgentConfig           agent.ConfigSetter
	InitializeStateParams InitializeStateParams
	MongoDialOpts         mongo.DialOpts
	StateNewPolicy        state.NewPolicyFunc
	BootrapDqlite         DqliteInitializerFunc
}

// InitializeAgent should be called with the bootstrap machine's agent
// configuration. It uses that information to create the controller, dial the
// controller, and initialize it. It also generates a new password for the
// bootstrap machine and calls Write to save the configuration.
//
// The cfg values will be stored in the state's ModelConfig; the
// machineCfg values will be used to configure the bootstrap Machine,
// and its constraints will be also be used for the model-level
// constraints. The connection to the controller will respect the
// given timeout parameter.
//
// InitializeAgent returns the newly initialized state and bootstrap machine.
// If it fails, the state may well be irredeemably compromised.
func InitializeAgent(ctx stdcontext.Context, cfg AgentInitializerConfig) (_ *state.Controller, resultErr error) {
	agentConfig := cfg.AgentConfig
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

	stateParams := cfg.InitializeStateParams

	// Add the controller model cloud and credential to the database.
	cloudCred, cloudCredTag, err := getCloudCredential(cfg.AdminUser, stateParams)
	if err != nil {
		return nil, errors.Annotate(err, "getting cloud credentials from args")
	}
	ctrlCloud := cfg.InitializeStateParams.ControllerCloud
	ctrlCloud.IsControllerCloud = true

	if err := cfg.BootrapDqlite(
		ctx,
		database.NewNodeManager(cfg.AgentConfig, logger, coredatabase.NoopSlowQueryLogger{}),
		logger,
		false,
		ccbootstrap.InsertInitialControllerConfig(stateParams.ControllerConfig),
		cloudbootstrap.InsertInitialControllerCloud(ctrlCloud),
		credbootstrap.InsertInitialControllerCredentials(cloudCredTag, cloudCred),
	); err != nil {
		return nil, errors.Trace(err)
	}

	session, err := initMongo(info.Info, cfg.MongoDialOpts, info.Password)
	if err != nil {
		return nil, errors.Annotate(err, "failed to initialize mongo")
	}
	defer session.Close()

	logger.Debugf("initializing address %v", info.Addrs)

	isCAAS := cloud.CloudIsCAAS(stateParams.ControllerCloud)
	modelType := state.ModelTypeIAAS
	if isCAAS {
		modelType = state.ModelTypeCAAS
	}
	ctrl, err := state.Initialize(state.InitializeParams{
		Clock: clock.WallClock,
		ControllerModelArgs: state.ModelArgs{
			Type:                    modelType,
			Owner:                   cfg.AdminUser,
			Config:                  stateParams.ControllerModelConfig,
			Constraints:             stateParams.ModelConstraints,
			CloudName:               stateParams.ControllerCloud.Name,
			CloudRegion:             stateParams.ControllerCloudRegion,
			CloudCredential:         cloudCredTag,
			StorageProviderRegistry: stateParams.StorageProviderRegistry,
			EnvironVersion:          stateParams.ControllerModelEnvironVersion,
		},
		StoragePools:              stateParams.StoragePools,
		CloudName:                 stateParams.ControllerCloud.Name,
		ControllerConfig:          stateParams.ControllerConfig,
		ControllerInheritedConfig: stateParams.ControllerInheritedConfig,
		RegionInheritedConfig:     stateParams.RegionInheritedConfig,
		MongoSession:              session,
		AdminPassword:             info.Password,
		NewPolicy:                 cfg.StateNewPolicy,
	})
	if err != nil {
		return nil, errors.Errorf("failed to initialize state: %v", err)
	}
	logger.Debugf("connected to initial state")
	defer func() {
		if resultErr != nil {
			_ = ctrl.Close()
		}
	}()
	servingInfo.SharedSecret = stateParams.SharedSecret
	cfg.AgentConfig.SetStateServingInfo(servingInfo)

	// Filter out any LXC or LXD bridge addresses from the machine addresses.
	stateParams.BootstrapMachineAddresses = network.FilterBridgeAddresses(stateParams.BootstrapMachineAddresses)

	st, err := ctrl.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Fetch spaces from substrate.
	// We need to do this before setting the API host-ports,
	// because any space names in the bootstrap machine addresses must be
	// reconcilable with space IDs at that point.
	callContext := context.CallContext(st)
	if err = space.ReloadSpaces(callContext, space.NewState(st), cfg.BootstrapEnviron); err != nil {
		if errors.Is(err, errors.NotSupported) {
			logger.Debugf("Not performing spaces load on a non-networking environment")
		} else {
			return nil, errors.Trace(err)
		}
	}

	// Verify model config DefaultSpace exists now that
	// spaces have been loaded.
	if err := verifyModelConfigDefaultSpace(st); err != nil {
		return nil, errors.Trace(err)
	}

	// Convert the provider addresses that we got from the bootstrap instance
	// to space ID decorated addresses.
	if err = initAPIHostPorts(
		st,
		stateParams.ControllerConfig,
		stateParams.BootstrapMachineAddresses,
		servingInfo.APIPort,
	); err != nil {
		return nil, err
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

	provider, err := stateParams.Provider(cloudSpec.Type)
	if err != nil {
		return nil, errors.Annotate(err, "getting environ provider")
	}

	var controllerNode bootstrapController
	if isCAAS {
		if controllerNode, err = initBootstrapNode(st, stateParams); err != nil {
			return nil, errors.Annotate(err, "cannot initialize bootstrap controller")
		}
		if err := initControllerCloudService(cloudSpec, provider, st, stateParams); err != nil {
			return nil, errors.Annotate(err, "cannot initialize cloud service")
		}
	} else {
		if controllerNode, err = initBootstrapMachine(st, stateParams); err != nil {
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
	logger.Debugf("create new random password for controller %v", controllerNode.Id())

	newPassword, err := utils.RandomPassword()
	if err != nil {
		return nil, err
	}
	if err := controllerNode.SetPassword(newPassword); err != nil {
		return nil, err
	}
	if err := controllerNode.SetMongoPassword(newPassword); err != nil {
		return nil, err
	}
	cfg.AgentConfig.SetPassword(newPassword)

	if err := ensureInitialModel(cloudSpec, provider, stateParams, st, ctrl, cfg.AdminUser, cloudCredTag); err != nil {
		return nil, errors.Annotate(err, "ensuring initial model")
	}
	return ctrl, nil
}

func verifyModelConfigDefaultSpace(st *state.State) error {
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
	return errors.Annotatef(err, "cannot verify %s", config.DefaultSpace)
}

func getCloudCredential(
	adminUser names.UserTag, args InitializeStateParams,
) (cloud.Credential, names.CloudCredentialTag, error) {
	var cloudCredentialTag names.CloudCredentialTag
	if args.ControllerCloudCredential != nil && args.ControllerCloudCredentialName != "" {
		id := fmt.Sprintf(
			"%s/%s/%s",
			args.ControllerCloud.Name,
			adminUser.Id(),
			args.ControllerCloudCredentialName,
		)
		if !names.IsValidCloudCredential(id) {
			return cloud.Credential{}, cloudCredentialTag, errors.NotValidf("cloud credential ID %q", id)
		}
		cloudCredentialTag = names.NewCloudCredentialTag(id)
		return *args.ControllerCloudCredential, cloudCredentialTag, nil
	}
	return cloud.Credential{}, cloudCredentialTag, nil
}

// ensureInitialModel ensures the initial model.
func ensureInitialModel(
	cloudSpec environscloudspec.CloudSpec,
	provider environs.EnvironProvider,
	args InitializeStateParams,
	st *state.State,
	ctrl *state.Controller,
	adminUser names.UserTag,
	cloudCredentialTag names.CloudCredentialTag,
) error {
	if len(args.InitialModelConfig) == 0 {
		logger.Debugf("no initial model configured")
		return nil
	}

	// Create the initial hosted model, with the model config passed to
	// bootstrap, which contains the UUID, name for the model,
	// and any user supplied config. We also copy the authorized-keys
	// from the controller model.
	attrs := make(map[string]interface{})
	for k, v := range args.InitialModelConfig {
		attrs[k] = v
	}
	attrs[config.AuthorizedKeysKey] = args.ControllerModelConfig.AuthorizedKeys()

	creator := modelmanager.ModelConfigCreator{Provider: args.Provider}
	InitialModelConfig, err := creator.NewModelConfig(
		cloudSpec, args.ControllerModelConfig, attrs,
	)
	if err != nil {
		return errors.Annotate(err, "creating initial model config")
	}
	controllerUUID := args.ControllerConfig.ControllerUUID()

	initialModelEnv, err := getEnviron(controllerUUID, cloudSpec, InitialModelConfig, provider)
	if err != nil {
		return errors.Annotate(err, "opening initial model environment")
	}

	if err := initialModelEnv.Create(
		context.CallContext(st),
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
		Owner:                   adminUser,
		Config:                  InitialModelConfig,
		Constraints:             args.ModelConstraints,
		CloudName:               args.ControllerCloud.Name,
		CloudRegion:             args.ControllerCloudRegion,
		CloudCredential:         cloudCredentialTag,
		StorageProviderRegistry: args.StorageProviderRegistry,
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
	ctx := context.CallContext(initialModelState)
	if err = space.ReloadSpaces(ctx, space.NewState(initialModelState), initialModelEnv); err != nil {
		if errors.Is(err, errors.NotSupported) {
			logger.Debugf("Not performing spaces load on a non-networking environment")
		} else {
			return errors.Annotate(err, "fetching initial model spaces")
		}
	}
	return nil
}

func getEnviron(
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
		return caas.Open(stdcontext.TODO(), provider, openParams)
	}
	return environs.Open(stdcontext.TODO(), provider, openParams)
}

// initMongo dials the initial MongoDB connection, setting a
// password for the admin user, and returning the session.
func initMongo(info mongo.Info, dialOpts mongo.DialOpts, password string) (*mgo.Session, error) {
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
func initBootstrapMachine(st *state.State, args InitializeStateParams) (bootstrapController, error) {
	logger.Infof("initialising bootstrap machine with config: %+v", args)

	jobs := make([]state.MachineJob, len(args.BootstrapMachineJobs))
	for i, job := range args.BootstrapMachineJobs {
		machineJob, err := machineJobFromParams(job)
		if err != nil {
			return nil, errors.Errorf("invalid bootstrap machine job %q: %v", job, err)
		}
		jobs[i] = machineJob
	}
	var hardware instance.HardwareCharacteristics
	if args.BootstrapMachineHardwareCharacteristics != nil {
		hardware = *args.BootstrapMachineHardwareCharacteristics
	}

	hostSeries, err := utilseries.HostSeries()
	if err != nil {
		return nil, errors.Trace(err)
	}

	spaceAddrs, err := args.BootstrapMachineAddresses.ToSpaceAddresses(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	base, err := corebase.GetBaseFromSeries(hostSeries)
	if err != nil {
		return nil, errors.Trace(err)
	}
	m, err := st.AddOneMachine(state.MachineTemplate{
		Addresses:               spaceAddrs,
		Base:                    state.Base{OS: base.OS, Channel: base.Channel.String()},
		Nonce:                   agent.BootstrapNonce,
		Constraints:             args.BootstrapMachineConstraints,
		InstanceId:              args.BootstrapMachineInstanceId,
		HardwareCharacteristics: hardware,
		Jobs:                    jobs,
		DisplayName:             args.BootstrapMachineDisplayName,
	})
	if err != nil {
		return nil, errors.Annotate(err, "cannot create bootstrap machine in state")
	}
	return m, nil
}

// initBootstrapNode initializes the initial caas bootstrap controller in state.
func initBootstrapNode(
	st *state.State,
	args InitializeStateParams,
) (bootstrapController, error) {
	logger.Debugf("initialising bootstrap node for with config: %+v", args)

	node, err := st.AddControllerNode()
	if err != nil {
		return nil, errors.Annotate(err, "cannot create bootstrap controller in state")
	}
	return node, nil
}

// initControllerCloudService creates cloud service for controller service.
func initControllerCloudService(
	cloudSpec environscloudspec.CloudSpec,
	provider environs.EnvironProvider,
	st *state.State,
	args InitializeStateParams,
) error {
	controllerUUID := args.ControllerConfig.ControllerUUID()
	env, err := getEnviron(controllerUUID, cloudSpec, args.ControllerModelConfig, provider)
	if err != nil {
		return errors.Annotate(err, "getting environ")
	}

	broker, ok := env.(caas.ServiceManager)
	if !ok {
		// this should never happen.
		return errors.Errorf("environ %T does not implement ServiceManager interface", env)
	}
	svc, err := broker.GetService(k8sconstants.JujuControllerStackName, true)
	if err != nil {
		return errors.Trace(err)
	}

	if len(svc.Addresses) == 0 {
		// this should never happen because we have already checked in k8s controller bootstrap stacker.
		return errors.NotProvisionedf("k8s controller service %q address", svc.Id)
	}
	addrs, err := svc.Addresses.ToSpaceAddresses(st)
	if err != nil {
		return errors.Trace(err)
	}

	svcId := controllerUUID
	logger.Infof("creating cloud service for k8s controller %q", svcId)
	cloudSvc, err := st.SaveCloudService(state.SaveCloudServiceArgs{
		Id:         svcId,
		ProviderId: svc.Id,
		Addresses:  addrs,
	})
	logger.Debugf("created cloud service %v for controller", cloudSvc)
	return errors.Trace(err)
}

// initAPIHostPorts sets the initial API host/port addresses in state.
func initAPIHostPorts(st *state.State, controllerConfig controller.Config, pAddrs corenetwork.ProviderAddresses, apiPort int) error {
	addrs, err := pAddrs.ToSpaceAddresses(st)
	if err != nil {
		return errors.Trace(err)
	}
	hostPorts := corenetwork.SpaceAddressesWithPort(addrs, apiPort)
	return st.SetAPIHostPorts(controllerConfig, []corenetwork.SpaceHostPorts{hostPorts})
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
