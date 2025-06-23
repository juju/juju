// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agentbootstrap"
	agentconfig "github.com/juju/juju/agent/config"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	cmdutil "github.com/juju/juju/cmd/jujud-controller/util"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	coreos "github.com/juju/juju/core/os"
	coreuser "github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	controllerdomain "github.com/juju/juju/domain/controller"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/simplestreams"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/database"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/mongo"
	pkissh "github.com/juju/juju/internal/pki/ssh"
	k8sprovider "github.com/juju/juju/internal/provider/kubernetes"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/worker/peergrouper"
)

var (
	initiateMongoServer = peergrouper.InitiateMongoServer
	sshGenerateKey      = ssh.GenerateKey
	minSocketTimeout    = 1 * time.Minute
	checkJWKSReachable  = agentbootstrap.CheckJWKSReachable
)

type BootstrapAgentFunc func(agentbootstrap.AgentBootstrapArgs) (*agentbootstrap.AgentBootstrap, error)

// BootstrapCommand represents a jujud bootstrap command.
type BootstrapCommand struct {
	cmd.CommandBase
	agentconf.AgentConf
	Timeout           time.Duration
	BootstrapAgent    BootstrapAgentFunc
	DqliteInitializer agentbootstrap.DqliteInitializerFunc
}

// NewBootstrapCommand returns a new BootstrapCommand that has been initialized.
func NewBootstrapCommand() *BootstrapCommand {
	return &BootstrapCommand{
		AgentConf: agentconf.NewAgentConf(""),

		BootstrapAgent:    agentbootstrap.NewAgentBootstrap,
		DqliteInitializer: database.BootstrapDqlite,
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
	if err := cmd.CheckEmpty(args); err != nil {
		return err
	}
	return c.AgentConf.CheckArgs(args)
}

func copyFile(dest, source string) error {
	df, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
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
		logger.Debugf(context.TODO(), "copying file from %q to %s", from, to)
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
	bootstrapParamsData, err := os.ReadFile(path.Join(c.DataDir(), cloudconfig.FileNameBootstrapParams))
	if err != nil {
		return errors.Annotate(err, "reading bootstrap params file")
	}
	var args instancecfg.StateInitializationParams
	if err := args.Unmarshal(bootstrapParamsData); err != nil {
		return errors.Trace(err)
	}
	// We need to set IsControllerCloud on the controller cloud from params.
	// This is so caas environs work correctly for the moment. This SHOULD be
	// removed with Mongo in time.
	// Fixes: lp2040947
	args.ControllerCloud.IsControllerCloud = true

	// The JWKS refresh URL is a public key that we trust for federated
	// auth. This is conventionally a JIMM controller. Check that JIMM
	// is reachable to fail fast and validate the URL.
	jwksRefreshURL := args.ControllerConfig.LoginTokenRefreshURL()
	if jwksRefreshURL != "" {
		if err := checkJWKSReachable(jwksRefreshURL); err != nil {
			return errors.Trace(err)
		}
	}

	isCAAS := args.ControllerCloud.Type == cloud.CloudTypeKubernetes

	if isCAAS {
		if err := c.ensureConfigFilesForCaas(); err != nil {
			return errors.Trace(err)
		}
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
		env, err = environsNewCAAS(ctx, openParams, environs.NoopCredentialInvalidator())
	} else {
		env, err = environsNewIAAS(ctx, openParams, environs.NoopCredentialInvalidator())
	}
	if err != nil {
		return errors.Trace(err)
	}

	controllerModelConfigAttrs := make(map[string]interface{})

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
			controllerModelConfigAttrs["agent-version"] = jujuversion.Current.String()
		} else {
			// If we have been asked for a newer version, ensure the newer
			// tools can actually be found, or else bootstrap won't complete.
			streams := envtools.PreferredStreams(&desiredVersion, args.ControllerModelConfig.Development(), args.ControllerModelConfig.AgentStream())
			logger.Infof(context.TODO(), "newer agent binaries requested, looking for %v in streams: %v", desiredVersion, strings.Join(streams, ","))
			filter := tools.Filter{
				Number: desiredVersion,
				Arch:   arch.HostArch(),
				OSType: coreos.HostOSTypeName(),
			}
			ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
			_, toolsErr := envtools.FindTools(ctx, ss, env, -1, -1, streams, filter)
			if toolsErr == nil {
				logger.Infof(context.TODO(), "agent binaries are available, upgrade will occur after bootstrap")
			}
			if errors.Is(toolsErr, errors.NotFound) {
				// Newer tools not available, so revert to using the tools
				// matching the current agent version.
				logger.Warningf(context.TODO(), "newer agent binaries for %q not available, sticking with version %q", desiredVersion, jujuversion.Current)
				controllerModelConfigAttrs["agent-version"] = jujuversion.Current.String()
			} else if toolsErr != nil {
				logger.Errorf(context.TODO(), "cannot find newer agent binaries: %v", toolsErr)
				return errors.Trace(toolsErr)
			}
		}
	}

	if err := agentconfig.ReadAgentConfig(c, agent.BootstrapControllerId); err != nil {
		return errors.Annotate(err, "cannot read config")
	}
	agentConfig := c.CurrentConfig()
	info, ok := agentConfig.StateServingInfo()
	if !ok {
		return fmt.Errorf("bootstrap machine config has no state serving info")
	}
	if err := ensureKeys(isCAAS, &args, &info); err != nil {
		return errors.Trace(err)
	}
	if err := ensureSSHServerHostKey(&args); err != nil {
		return errors.Trace(err)
	}
	addrs, err := getAddressesForMongo(isCAAS, env, ctx, args)
	if err != nil {
		return errors.Trace(err)
	}

	if err = c.ChangeConfig(func(agentConfig agent.ConfigSetter) error {
		agentConfig.SetStateServingInfo(info)

		agentConfig.SetJujuDBSnapChannel(args.ControllerConfig.JujuDBSnapChannel())
		agentConfig.SetQueryTracingEnabled(args.ControllerConfig.QueryTracingEnabled())
		agentConfig.SetQueryTracingThreshold(args.ControllerConfig.QueryTracingThreshold())
		agentConfig.SetOpenTelemetryEnabled(args.ControllerConfig.OpenTelemetryEnabled())
		agentConfig.SetOpenTelemetryEndpoint(args.ControllerConfig.OpenTelemetryEndpoint())
		agentConfig.SetOpenTelemetryInsecure(args.ControllerConfig.OpenTelemetryInsecure())
		agentConfig.SetOpenTelemetryStackTraces(args.ControllerConfig.OpenTelemetryStackTraces())
		agentConfig.SetOpenTelemetrySampleRatio(args.ControllerConfig.OpenTelemetrySampleRatio())
		agentConfig.SetOpenTelemetryTailSamplingThreshold(args.ControllerConfig.OpenTelemetryTailSamplingThreshold())
		agentConfig.SetObjectStoreType(args.ControllerConfig.ObjectStoreType())

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

	controllerModelCfg, err := env.Config().Apply(controllerModelConfigAttrs)
	if err != nil {
		return errors.Annotate(err, "failed to update model config")
	}
	args.ControllerModelConfig = controllerModelCfg

	// Initialise state, and store any agent config (e.g. password) changes.
	err = c.ChangeConfig(func(agentConfig agent.ConfigSetter) error {
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

		adminTag := names.NewLocalUserTag(coreuser.AdminUserName.Name())
		bootstrap, err := c.BootstrapAgent(agentbootstrap.AgentBootstrapArgs{
			AgentConfig:               agentConfig,
			BootstrapEnviron:          env,
			AdminUser:                 adminTag,
			StateInitializationParams: args,
			BootstrapMachineAddresses: addrs,
			SharedSecret:              info.SharedSecret,
			StorageProviderRegistry:   provider.NewStorageProviderRegistry(env),
			MongoDialOpts:             dialOpts,
			BootstrapDqlite:           c.DqliteInitializer,
			Provider:                  environs.Provider,
			Logger:                    internallogger.GetLogger("juju.agent.bootstrap"),
		})
		if err != nil {
			return errors.Trace(err)
		}
		return bootstrap.Initialize(ctx)
	})
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func getAddressesForMongo(
	isCAAS bool,
	env environs.BootstrapEnviron,
	ctx context.Context,
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
	instances, err := instanceLister.Instances(ctx, []instance.Id{args.BootstrapMachineInstanceId})
	if err != nil {
		return nil, errors.Annotate(err, "getting bootstrap instance")
	}
	addrs, err := instances[0].Addresses(ctx)
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
) error {
	if isCAAS {
		return nil
	}
	// Generate a private SSH key for the controllers, and add
	// the public key to the environment config. We'll add the
	// private key to StateServingInfo below.
	privateKey, publicKey, err := sshGenerateKey(controllerdomain.ControllerSSHKeyComment)
	if err != nil {
		return errors.Annotate(err, "failed to generate system key")
	}
	info.SystemIdentity = privateKey

	args.ControllerConfig[controller.SystemSSHKeys] = publicKey

	// Generate a shared secret for the Mongo replica set, and write it out.
	sharedSecret, err := mongo.GenerateSharedSecret()
	if err != nil {
		return errors.Trace(err)
	}
	info.SharedSecret = sharedSecret

	return nil
}

func (c *BootstrapCommand) startMongo(ctx context.Context, isCAAS bool, addrs network.ProviderAddresses, agentConfig agent.Config) error {
	logger.Debugf(context.TODO(), "starting mongo")

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
		logger.Debugf(context.TODO(), "calling EnsureMongoServerInstalled")
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
	logger.Infof(context.TODO(), "started mongo")
	return nil
}
