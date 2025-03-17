// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"bytes"
	stdcontext "context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"
	"github.com/juju/utils/v3/ssh"
	"github.com/juju/version/v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agentbootstrap"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/caas"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cloudconfig/instancecfg"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/jujud/agent/agentconf"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/worker/peergrouper"
	"github.com/juju/juju/mongo"
	pkissh "github.com/juju/juju/pki/ssh"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

var (
	initiateMongoServer  = peergrouper.InitiateMongoServer
	agentInitializeState = agentbootstrap.InitializeState
	sshGenerateKey       = ssh.GenerateKey
	minSocketTimeout     = 1 * time.Minute
)

const adminUserName = "admin"

// BootstrapCommand represents a jujud bootstrap command.
type BootstrapCommand struct {
	cmd.CommandBase
	agentconf.AgentConf
	BootstrapParamsFile string
	Timeout             time.Duration
}

// NewBootstrapCommand returns a new BootstrapCommand that has been initialized.
func NewBootstrapCommand() *BootstrapCommand {
	return &BootstrapCommand{
		AgentConf: agentconf.NewAgentConf(""),
	}
}

// Info returns a description of the command.
func (c *BootstrapCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "bootstrap-state",
		Purpose: "initialize juju state",
	})
}

// SetFlags adds the flags for this command to the passed gnuflag.FlagSet.
func (c *BootstrapCommand) SetFlags(f *gnuflag.FlagSet) {
	c.AgentConf.AddFlags(f)
	f.DurationVar(&c.Timeout, "timeout", time.Duration(0), "set the bootstrap timeout")
}

// Init initializes the command for running.
func (c *BootstrapCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("bootstrap-params file must be specified")
	}
	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return err
	}
	c.BootstrapParamsFile = args[0]
	return c.AgentConf.CheckArgs(args[1:])
}

func copyFile(dest, source string) error {
	df, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0660)
	if err != nil {
		return errors.Trace(err)
	}
	defer df.Close()

	f, err := os.Open(source)
	if err != nil {
		return errors.Trace(err)
	}
	defer f.Close()

	_, err = io.Copy(df, f)
	return errors.Trace(err)
}

func copyFileFromTemplate(to, from string) (err error) {
	if _, err := os.Stat(to); os.IsNotExist(err) {
		logger.Debugf("copying file from %q to %s", from, to)
		if err := copyFile(to, from); err != nil {
			return errors.Trace(err)
		}
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *BootstrapCommand) ensureConfigFilesForCaas() error {
	tag := names.NewControllerAgentTag(agent.BootstrapControllerId)
	for _, v := range []struct {
		to, from string
	}{
		{
			// ensure agent.conf
			to: agent.ConfigPath(c.AgentConf.DataDir(), tag),
			from: filepath.Join(
				agent.Dir(c.AgentConf.DataDir(), tag),
				k8sconstants.TemplateFileNameAgentConf,
			),
		},
		{
			// ensure server.pem
			to:   filepath.Join(c.AgentConf.DataDir(), mongo.FileNameDBSSLKey),
			from: filepath.Join(c.AgentConf.DataDir(), k8sprovider.TemplateFileNameServerPEM),
		},
	} {
		if err := copyFileFromTemplate(v.to, v.from); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

var (
	environsNewIAAS = environs.New
	environsNewCAAS = caas.New
)

// Run initializes state for an environment.
func (c *BootstrapCommand) Run(ctx *cmd.Context) error {
	bootstrapParamsData, err := os.ReadFile(c.BootstrapParamsFile)
	if err != nil {
		return errors.Annotate(err, "reading bootstrap params file")
	}
	var args instancecfg.StateInitializationParams
	if err := args.Unmarshal(bootstrapParamsData); err != nil {
		return errors.Trace(err)
	}

	isCAAS := args.ControllerCloud.Type == k8sconstants.CAASProviderType

	var controllerUnitPassword string
	if isCAAS {
		if err := c.ensureConfigFilesForCaas(); err != nil {
			return errors.Trace(err)
		}
		controllerUnitPassword = os.Getenv(k8sconstants.EnvJujuK8sUnitPassword)
	}

	// Get the bootstrap machine's addresses from the provider.
	cloudSpec, err := environscloudspec.MakeCloudSpec(
		args.ControllerCloud,
		args.ControllerCloudRegion,
		args.ControllerCloudCredential,
	)
	if err != nil {
		return errors.Trace(err)
	}
	cloudSpec.IsControllerCloud = true

	openParams := environs.OpenParams{
		ControllerUUID: args.ControllerConfig.ControllerUUID(),
		Cloud:          cloudSpec,
		Config:         args.ControllerModelConfig,
	}

	var env environs.BootstrapEnviron
	if isCAAS {
		env, err = environsNewCAAS(ctx, openParams)
	} else {
		env, err = environsNewIAAS(ctx, openParams)
	}
	if err != nil {
		return errors.Trace(err)
	}

	newConfigAttrs := make(map[string]interface{})

	// Check to see if a newer agent version has been requested
	// by the bootstrap client.
	desiredVersion, ok := args.ControllerModelConfig.AgentVersion()
	if ok && desiredVersion != jujuversion.Current {
		if isCAAS {
			currentVersion := jujuversion.Current
			currentVersion.Build = 0
			if desiredVersion != currentVersion {
				// For CAAS, the agent-version in controller config should
				// always equals to current juju version.
				return errors.NotSupportedf(
					"desired juju version %q, current version %q for k8s controllers",
					desiredVersion, currentVersion,
				)
			}
			// Old juju clients will use the version without build number when
			// selecting the controller OCI image tag. In this case, the current controller
			// version was the correct version.
			newConfigAttrs["agent-version"] = jujuversion.Current.String()
		} else {
			// If we have been asked for a newer version, ensure the newer
			// tools can actually be found, or else bootstrap won't complete.
			streams := envtools.PreferredStreams(&desiredVersion, args.ControllerModelConfig.Development(), args.ControllerModelConfig.AgentStream())
			logger.Infof("newer agent binaries requested, looking for %v in streams: %v", desiredVersion, strings.Join(streams, ","))
			filter := tools.Filter{
				Number: desiredVersion,
				Arch:   arch.HostArch(),
				OSType: coreos.HostOSTypeName(),
			}
			ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
			_, toolsErr := envtools.FindTools(ss, env, -1, -1, streams, filter)
			if toolsErr == nil {
				logger.Infof("agent binaries are available, upgrade will occur after bootstrap")
			}
			if errors.IsNotFound(toolsErr) {
				// Newer tools not available, so revert to using the tools
				// matching the current agent version.
				logger.Warningf("newer agent binaries for %q not available, sticking with version %q", desiredVersion, jujuversion.Current)
				newConfigAttrs["agent-version"] = jujuversion.Current.String()
			} else if toolsErr != nil {
				logger.Errorf("cannot find newer agent binaries: %v", toolsErr)
				return errors.Trace(toolsErr)
			}
		}
	}

	callCtx := context.NewCloudCallContext(ctx)
	// At this stage, cloud credential has not yet been stored server-side
	// as there is no server-side. If these cloud calls will fail with
	// invalid credential, just log it.
	callCtx.InvalidateCredentialFunc = func(reason string) error {
		logger.Errorf("Cloud credential %q is not accepted by cloud provider: %v", args.ControllerCloudCredentialName, reason)
		return nil
	}

	if err := agentconf.ReadAgentConfig(c, agent.BootstrapControllerId); err != nil {
		return errors.Annotate(err, "cannot read config")
	}
	agentConfig := c.CurrentConfig()
	info, ok := agentConfig.StateServingInfo()
	if !ok {
		return fmt.Errorf("bootstrap machine config has no state serving info")
	}
	if err := ensureKeys(isCAAS, &args, &info, newConfigAttrs); err != nil {
		return errors.Trace(err)
	}
	if err := ensureSSHServerHostKey(&args); err != nil {
		return errors.Trace(err)
	}
	addrs, err := getAddressesForMongo(isCAAS, env, callCtx, args)
	if err != nil {
		return errors.Trace(err)
	}

	if err = c.ChangeConfig(func(agentConfig agent.ConfigSetter) error {
		agentConfig.SetStateServingInfo(info)

		mmprof, err := mongo.NewMemoryProfile(args.ControllerConfig.MongoMemoryProfile())
		if err != nil {
			logger.Errorf("could not set requested memory profile: %v", err)
		} else {
			agentConfig.SetMongoMemoryProfile(mmprof)
		}

		agentConfig.SetJujuDBSnapChannel(args.ControllerConfig.JujuDBSnapChannel())
		agentConfig.SetQueryTracingEnabled(args.ControllerConfig.QueryTracingEnabled())
		agentConfig.SetQueryTracingThreshold(args.ControllerConfig.QueryTracingThreshold())
		return nil
	}); err != nil {
		return fmt.Errorf("cannot write agent config: %v", err)
	}

	agentConfig = c.CurrentConfig()

	// Create system-identity file
	if err := agent.WriteSystemIdentityFile(agentConfig); err != nil {
		return errors.Trace(err)
	}

	if err := c.startMongo(ctx, isCAAS, addrs, agentConfig); err != nil {
		return errors.Annotate(err, "failed to start mongo")
	}

	controllerModelCfg, err := env.Config().Apply(newConfigAttrs)
	if err != nil {
		return errors.Annotate(err, "failed to update model config")
	}
	args.ControllerModelConfig = controllerModelCfg

	// Initialise state, and store any agent config (e.g. password) changes.
	var controller *state.Controller
	err = c.ChangeConfig(func(agentConfig agent.ConfigSetter) error {
		var stateErr error
		dialOpts := mongo.DefaultDialOpts()

		// Set a longer socket timeout than usual, as the machine
		// will be starting up and disk I/O slower than usual. This
		// has been known to cause timeouts in queries.
		dialOpts.SocketTimeout = c.Timeout
		if dialOpts.SocketTimeout < minSocketTimeout {
			dialOpts.SocketTimeout = minSocketTimeout
		}

		// We shouldn't attempt to dial peers until we have some.
		dialOpts.Direct = true

		adminTag := names.NewLocalUserTag(adminUserName)
		controller, stateErr = agentInitializeState(
			env,
			adminTag,
			agentConfig,
			agentbootstrap.InitializeStateParams{
				StateInitializationParams: args,
				BootstrapMachineAddresses: addrs,
				BootstrapMachineJobs:      agentConfig.Jobs(),
				SharedSecret:              info.SharedSecret,
				Provider:                  environs.Provider,
				StorageProviderRegistry:   stateenvirons.NewStorageProviderRegistry(env),
			},
			dialOpts,
			stateenvirons.GetNewPolicyFunc(),
		)
		return stateErr
	})
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = controller.Close() }()
	st, err := controller.SystemState()
	if err != nil {
		return errors.Trace(err)
	}

	// Set up default container networking mode
	model, err := st.Model()
	if err != nil {
		return err
	}
	if err = model.AutoConfigureContainerNetworking(env); err != nil {
		if errors.IsNotSupported(err) {
			logger.Debugf("Not performing container networking auto-configuration on a non-networking environment")
		} else {
			return err
		}
	}

	if !isCAAS {
		// Populate the tools catalogue.
		if err := c.populateTools(st); err != nil {
			return errors.Trace(err)
		}
		// Add custom image metadata to environment storage.
		if len(args.CustomImageMetadata) > 0 {
			if err := c.saveCustomImageMetadata(st, env, args.CustomImageMetadata); err != nil {
				return err
			}
		}
	}

	// Deploy and set up the controller charm and application.
	charmArgs := controllerCharmArgs{
		cons:         args.BootstrapMachineConstraints,
		charmPath:    args.ControllerCharmPath,
		channel:      args.ControllerCharmChannel,
		isCAAS:       isCAAS,
		unitPassword: controllerUnitPassword,
	}
	if err := c.deployControllerCharm(st, charmArgs); err != nil {
		return errors.Annotate(err, "cannot deploy controller application")
	}

	// bootstrap nodes always get the vote
	node, err := st.ControllerNode(agent.BootstrapControllerId)
	if err != nil {
		return errors.Trace(err)
	}
	return node.SetHasVote(true)
}

func getAddressesForMongo(
	isCAAS bool,
	env environs.BootstrapEnviron,
	callCtx *context.CloudCallContext,
	args instancecfg.StateInitializationParams,
) (network.ProviderAddresses, error) {
	if isCAAS {
		return network.NewMachineAddresses([]string{"localhost"}).AsProviderAddresses(), nil
	}

	instanceLister, ok := env.(environs.InstanceLister)
	if !ok {
		// this should never happened.
		return nil, errors.NotValidf("InstanceLister missing for IAAS controller provider")
	}
	instances, err := instanceLister.Instances(callCtx, []instance.Id{args.BootstrapMachineInstanceId})
	if err != nil {
		return nil, errors.Annotate(err, "getting bootstrap instance")
	}
	addrs, err := instances[0].Addresses(callCtx)
	if err != nil {
		return nil, errors.Annotate(err, "getting bootstrap instance addresses")
	}
	return addrs, nil
}

// ensureSSHServerHostKey ensures that either a) a user has provided a host key
// or b) one has been generated for the controller.
func ensureSSHServerHostKey(args *instancecfg.StateInitializationParams) error {
	if args.SSHServerHostKey != "" {
		return nil
	}
	// Generate the embedded SSH server host key and store it within StateInitializationParams.
	hostKey, err := pkissh.NewMarshalledED25519()
	if err != nil {
		return errors.Annotatef(err, "failed to ensure ssh server host key")
	}
	args.SSHServerHostKey = string(hostKey)
	return nil
}

func ensureKeys(
	isCAAS bool,
	args *instancecfg.StateInitializationParams,
	info *controller.StateServingInfo,
	newConfigAttrs map[string]interface{},
) error {
	if isCAAS {
		return nil
	}
	// Generate a private SSH key for the controllers, and add
	// the public key to the environment config. We'll add the
	// private key to StateServingInfo below.
	privateKey, publicKey, err := sshGenerateKey(config.JujuSystemKey)
	if err != nil {
		return errors.Annotate(err, "failed to generate system key")
	}
	info.SystemIdentity = privateKey

	authorizedKeys := config.ConcatAuthKeys(args.ControllerModelConfig.AuthorizedKeys(), publicKey)
	newConfigAttrs[config.AuthorizedKeysKey] = authorizedKeys

	// Generate a shared secret for the Mongo replica set, and write it out.
	sharedSecret, err := mongo.GenerateSharedSecret()
	if err != nil {
		return errors.Trace(err)
	}
	info.SharedSecret = sharedSecret

	return nil
}

func (c *BootstrapCommand) startMongo(ctx stdcontext.Context, isCAAS bool, addrs network.ProviderAddresses, agentConfig agent.Config) error {
	logger.Debugf("starting mongo")

	info, ok := agentConfig.MongoInfo()
	if !ok {
		return fmt.Errorf("no state info available")
	}
	// When bootstrapping, we need to allow enough time for mongo
	// to start as there's no retry loop in place.
	// 5 minutes should suffice.
	mongoDialOpts := mongo.DialOpts{Timeout: 5 * time.Minute}
	dialInfo, err := mongo.DialInfo(info.Info, mongoDialOpts)
	if err != nil {
		return err
	}
	servingInfo, ok := agentConfig.StateServingInfo()
	if !ok {
		return fmt.Errorf("agent config has no state serving info")
	}
	// Use localhost to dial the mongo server, because it's running in
	// auth mode and will refuse to perform any operations unless
	// we dial that address.
	// TODO(macgreagoir) IPv6. Ubuntu still always provides IPv4 loopback,
	// and when/if this changes localhost should resolve to IPv6 loopback
	// in any case (lp:1644009). Review.
	dialInfo.Addrs = []string{
		net.JoinHostPort("localhost", fmt.Sprint(servingInfo.StatePort)),
	}

	if !isCAAS {
		logger.Debugf("calling EnsureMongoServerInstalled")
		ensureServerParams, err := cmdutil.NewEnsureMongoParams(agentConfig)
		if err != nil {
			return err
		}
		if err := cmdutil.EnsureMongoServerInstalled(ctx, ensureServerParams); err != nil {
			return err
		}
	}

	peerAddr := mongo.SelectPeerAddress(addrs)
	if peerAddr == "" {
		return fmt.Errorf("no appropriate peer address found in %q", addrs)
	}
	peerHostPort := net.JoinHostPort(peerAddr, fmt.Sprint(servingInfo.StatePort))

	if err := initiateMongoServer(peergrouper.InitiateMongoParams{
		DialInfo:       dialInfo,
		MemberHostPort: peerHostPort,
	}); err != nil {
		return err
	}
	logger.Infof("started mongo")
	return nil
}

// populateTools stores uploaded tools in provider storage
// and updates the tools metadata.
func (c *BootstrapCommand) populateTools(st *state.State) error {
	agentConfig := c.CurrentConfig()
	dataDir := agentConfig.DataDir()

	current := version.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: coreos.HostOSTypeName(),
	}
	agentTools, err := agenttools.ReadTools(dataDir, current)
	if err != nil {
		return errors.Trace(err)
	}

	data, err := os.ReadFile(filepath.Join(
		agenttools.SharedToolsDir(dataDir, current),
		"tools.tar.gz",
	))
	if err != nil {
		return errors.Trace(err)
	}

	toolStorage, err := st.ToolsStorage()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = toolStorage.Close() }()

	metadata := binarystorage.Metadata{
		Version: agentTools.Version.String(),
		Size:    agentTools.Size,
		SHA256:  agentTools.SHA256,
	}
	logger.Debugf("Adding agent binary: %v", agentTools.Version)
	if err := toolStorage.Add(bytes.NewReader(data), metadata); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// saveCustomImageMetadata stores the custom image metadata to the database,
func (c *BootstrapCommand) saveCustomImageMetadata(st *state.State, env environs.BootstrapEnviron, imageMetadata []*imagemetadata.ImageMetadata) error {
	logger.Debugf("saving custom image metadata")
	return storeImageMetadataInState(st, env, "custom", simplestreams.CUSTOM_CLOUD_DATA, imageMetadata)
}

// storeImageMetadataInState writes image metadata into state store.
func storeImageMetadataInState(st *state.State, env environs.BootstrapEnviron, source string, priority int, existingMetadata []*imagemetadata.ImageMetadata) error {
	if len(existingMetadata) == 0 {
		return nil
	}
	cfg := env.Config()
	metadataState := make([]cloudimagemetadata.Metadata, len(existingMetadata))
	for i, one := range existingMetadata {
		m := cloudimagemetadata.Metadata{
			MetadataAttributes: cloudimagemetadata.MetadataAttributes{
				Stream:          one.Stream,
				Region:          one.RegionName,
				Arch:            one.Arch,
				VirtType:        one.VirtType,
				RootStorageType: one.Storage,
				Source:          source,
				Version:         one.Version,
			},
			Priority: priority,
			ImageId:  one.Id,
		}
		if m.Stream == "" {
			m.Stream = cfg.ImageStream()
		}
		if m.Source == "" {
			m.Source = "custom"
		}
		metadataState[i] = m
	}
	if err := st.CloudImageMetadataStorage.SaveMetadata(metadataState); err != nil {
		return errors.Annotatef(err, "cannot cache image metadata")
	}
	return nil
}
