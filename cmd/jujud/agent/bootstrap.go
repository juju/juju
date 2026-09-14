// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agentbootstrap"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
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
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/controllerruntimeconfig"
	"github.com/juju/juju/internal/database"
	internallogger "github.com/juju/juju/internal/logger"
	pkissh "github.com/juju/juju/internal/pki/ssh"
	"github.com/juju/juju/internal/tools"
)

var (
	sshGenerateKey     = ssh.GenerateKey
	checkJWKSReachable = agentbootstrap.CheckJWKSReachable
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

var (
	environsNewIAAS = environs.New
	environsNewCAAS = caas.New
)

// Run initializes state for an environment.
func (c *BootstrapCommand) Run(ctx *cmd.Context) error {
	bootstrapParamsData, err := os.ReadFile(bootstrap.BootstrapParamsPath(c.DataDir()))
	if err != nil {
		return errors.Annotate(err, "reading bootstrap params file")
	}
	var args instancecfg.StateInitializationParams
	if err := args.Unmarshal(bootstrapParamsData); err != nil {
		return errors.Trace(err)
	}
	// We need to set IsControllerCloud on the controller cloud from params.
	// This is so caas environs work correctly for the moment. This SHOULD be
	// removed in the future.
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

	controllerModelConfigAttrs := make(map[string]any)

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

	// Both IAAS snap and CAAS controllers read controller startup values
	// from runtime.conf instead of agent.conf.
	return c.runFromRuntimeConf(ctx, args, env, controllerModelConfigAttrs, isCAAS)
}

// runFromRuntimeConf runs bootstrap state initialization from runtime.conf.
// It reads the controller startup configuration from the data directory's
// runtime.conf and persists mutations back to that file. It never reads,
// writes, or creates a controller agent.conf under agents/controller-<id>.
func (c *BootstrapCommand) runFromRuntimeConf(
	ctx *cmd.Context,
	args instancecfg.StateInitializationParams,
	env environs.BootstrapEnviron,
	controllerModelConfigAttrs map[string]any,
	isCAAS bool,
) error {
	// Resolve the runtime.conf path. For IAAS snap controllers, SNAP_DATA
	// overrides the --data-dir flag. For CAAS, --data-dir is the controller
	// root (/var/lib/juju/controller).
	runtimeCfgDir := c.DataDir()
	if snapData := os.Getenv("SNAP_DATA"); snapData != "" {
		runtimeCfgDir = snapData
	}
	runtimeCfgPath := filepath.Join(runtimeCfgDir, controllerruntimeconfig.Filename)

	runtimeCfg, err := controllerruntimeconfig.ReadControllerRuntimeConfig(runtimeCfgPath)
	if err != nil {
		return errors.Annotate(err, "reading controller runtime config")
	}

	// Build the ControllerAgentInfo from runtime.conf values. This is the narrow
	// in-memory state required by AgentBootstrap. It must never be written as
	// agent.conf.
	info := controller.ControllerAgentInfo{
		Cert:           runtimeCfg.ControllerCert,
		PrivateKey:     runtimeCfg.ControllerPrivateKey,
		CAPrivateKey:   runtimeCfg.CAPrivateKey,
		APIPort:        runtimeCfg.APIPort,
		SystemIdentity: runtimeCfg.SystemIdentity,
	}

	if err := ensureKeys(isCAAS, &args, &info); err != nil {
		return errors.Trace(err)
	}
	if err := ensureSSHServerHostKey(&args); err != nil {
		return errors.Trace(err)
	}
	addrs, err := getInstanceAddresses(isCAAS, env, ctx, args)
	if err != nil {
		return errors.Trace(err)
	}

	// Persist the key and address mutations back to runtime.conf.
	// API agent config requires host:port forms (see agent.checkAddrs), while
	// ProviderAddresses.Values() returns raw hosts only.
	apiAddrs := make([]string, 0, len(addrs))
	apiPort := strconv.Itoa(info.APIPort)
	for _, host := range addrs.Values() {
		apiAddrs = append(apiAddrs, net.JoinHostPort(host, apiPort))
	}
	if err := controllerruntimeconfig.ChangeControllerRuntimeConfig(runtimeCfgPath, func(cfg *controllerruntimeconfig.ControllerRuntimeConfig) error {
		cfg.ControllerCert = info.Cert
		cfg.ControllerPrivateKey = info.PrivateKey
		cfg.CAPrivateKey = info.CAPrivateKey
		cfg.SystemIdentity = info.SystemIdentity
		cfg.APIAddresses = apiAddrs
		cfg.QueryTracingEnabled = args.ControllerConfig.QueryTracingEnabled()
		cfg.QueryTracingThreshold = args.ControllerConfig.QueryTracingThreshold()
		cfg.DqliteBusyTimeout = args.ControllerConfig.DqliteBusyTimeout()
		return nil
	}); err != nil {
		return errors.Annotate(err, "persisting key mutations to runtime config")
	}

	// Build the in-memory agent.ConfigSetterWriter from values already in
	// hand: the original runtime.conf read, the mutated ControllerAgentInfo,
	// and the computed API addresses. This avoids a second disk read.
	controllerTag := names.NewControllerTag(runtimeCfg.ControllerUUID)
	modelTag := names.NewModelTag(runtimeCfg.ControllerModelUUID)
	controllerAgentTag := names.NewControllerAgentTag(runtimeCfg.ControllerID)

	agentConfigParams := agent.AgentConfigParams{
		Paths: agent.Paths{
			DataDir: runtimeCfg.DataDir,
			LogDir:  runtimeCfg.LogDir,
		},
		Tag:                   controllerAgentTag,
		UpgradedToVersion:     jujuversion.Current,
		Password:              runtimeCfg.AgentPassword,
		Controller:            controllerTag,
		Model:                 modelTag,
		APIAddresses:          apiAddrs,
		CACert:                runtimeCfg.CACert,
		QueryTracingEnabled:   args.ControllerConfig.QueryTracingEnabled(),
		QueryTracingThreshold: args.ControllerConfig.QueryTracingThreshold(),
		DqliteBusyTimeout:     args.ControllerConfig.DqliteBusyTimeout(),
	}
	agentCfgWriter, err := agent.NewStateMachineConfig(agentConfigParams, info)
	if err != nil {
		return errors.Annotate(err, "building in-memory controller agent config")
	}

	// Write the system identity file to the snap-private DataDir.
	if err := agent.WriteSystemIdentityFile(agentCfgWriter); err != nil {
		return errors.Trace(err)
	}

	controllerModelCfg, err := env.Config().Apply(controllerModelConfigAttrs)
	if err != nil {
		return errors.Annotate(err, "failed to update model config")
	}
	args.ControllerModelConfig = controllerModelCfg

	// Initialise state. AgentBootstrap.Initialize mutates agentCfgWriter
	// (SetControllerAgentInfo); for the IAAS snap path it does not rotate
	// the password (see AgentBootstrap.Initialize for why). The in-memory
	// config is never written to disk as agent.conf.
	adminTag := names.NewLocalUserTag(coreuser.AdminUserName.Name())
	bootstrapAgent, err := c.BootstrapAgent(agentbootstrap.AgentBootstrapArgs{
		AgentConfig:               agentCfgWriter,
		BootstrapEnviron:          env,
		AdminUser:                 adminTag,
		StateInitializationParams: args,
		BootstrapMachineAddresses: addrs,
		BootstrapDqlite:           c.DqliteInitializer,
		Logger:                    internallogger.GetLogger("juju.agent.bootstrap"),
	})
	if err != nil {
		return errors.Trace(err)
	}
	if err := bootstrapAgent.Initialize(ctx); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func getInstanceAddresses(
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
	info *controller.ControllerAgentInfo,
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

	return nil
}
