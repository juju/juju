// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agentbootstrap"
	agentconfig "github.com/juju/juju/agent/config"
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
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	internallogger "github.com/juju/juju/internal/logger"
	pkissh "github.com/juju/juju/internal/pki/ssh"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
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
	DqliteInitialiser agentbootstrap.DqliteInitialiserFunc
}

// NewBootstrapCommand returns a new BootstrapCommand that has been initialised.
func NewBootstrapCommand() *BootstrapCommand {
	return &BootstrapCommand{
		AgentConf: agentconf.NewAgentConf(""),

		BootstrapAgent:    agentbootstrap.NewAgentBootstrap,
		DqliteInitialiser: database.BootstrapDqlite,
	}
}

// Info returns a description of the command.
func (c *BootstrapCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "bootstrap-state",
		Purpose: "initialise juju state",
	})
}

// SetFlags adds the flags for this command to the passed gnuflag.FlagSet.
func (c *BootstrapCommand) SetFlags(f *gnuflag.FlagSet) {
	c.AgentConf.AddFlags(f)
	f.DurationVar(&c.Timeout, "timeout", time.Duration(0), "set the bootstrap timeout")
}

// Init initialises the command for running.
func (c *BootstrapCommand) Init(args []string) error {
	if err := cmd.CheckEmpty(args); err != nil {
		return err
	}
	return c.AgentConf.CheckArgs(args)
}

func copyFile(dest, source string) error {
	df, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
	if err != nil {
		return errors.Capture(err)
	}
	defer df.Close()

	f, err := os.Open(source)
	if err != nil {
		return errors.Capture(err)
	}
	defer f.Close()

	_, err = io.Copy(df, f)
	return errors.Capture(err)
}

func copyFileFromTemplate(to, from string) (err error) {
	if _, err := os.Stat(to); os.IsNotExist(err) {
		logger.Debugf(context.TODO(), "copying file from %q to %s", from, to)
		if err := copyFile(to, from); err != nil {
			return errors.Capture(err)
		}
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (c *BootstrapCommand) ensureConfigFilesForK8s() error {
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
	} {
		if err := copyFileFromTemplate(v.to, v.from); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

var (
	environsNewIAAS = environs.New
	environsNewK8s  = caas.New
)

// Run initialises state for an environment.
func (c *BootstrapCommand) Run(ctx *cmd.Context) error {
	bootstrapParamsData, err := os.ReadFile(path.Join(c.DataDir(), cloudconfig.FileNameBootstrapParams))
	if err != nil {
		return errors.Errorf("reading bootstrap params file: %w", err)
	}
	var args instancecfg.StateInitializationParams
	if err := args.Unmarshal(bootstrapParamsData); err != nil {
		return errors.Capture(err)
	}
	// Choose fresh bootstrap or restoration here, before preparing identity or
	// writing agent configuration. Each workflow owns its complete sequence.
	return errors.Capture(c.initialiseFreshController(ctx, args))
}

// initialiseFreshController prepares a new controller's identity and local
// configuration, seeds its databases, and persists the resulting agent password.
func (c *BootstrapCommand) initialiseFreshController(ctx *cmd.Context, args instancecfg.StateInitializationParams) error {
	// We need to set IsControllerCloud on the controller cloud from params.
	// This is so K8s environs work correctly for the moment. This SHOULD be
	// removed in the future.
	// Fixes: lp2040947
	args.ControllerCloud.IsControllerCloud = true

	// The JWKS refresh URL is a public key that we trust for federated
	// auth. This is conventionally a JIMM controller. Check that JIMM
	// is reachable to fail fast and validate the URL.
	jwksRefreshURL := args.ControllerConfig.LoginTokenRefreshURL()
	if jwksRefreshURL != "" {
		if err := checkJWKSReachable(jwksRefreshURL); err != nil {
			return errors.Capture(err)
		}
	}

	isK8s := args.ControllerCloud.Type == cloud.CloudTypeKubernetes

	if isK8s {
		if err := c.ensureConfigFilesForK8s(); err != nil {
			return errors.Capture(err)
		}
	}

	// Get the bootstrap machine's addresses from the provider.
	cloudSpec, err := environscloudspec.MakeCloudSpec(
		args.ControllerCloud,
		args.ControllerCloudRegion,
		args.ControllerCloudCredential,
	)
	if err != nil {
		return errors.Capture(err)
	}
	cloudSpec.IsControllerCloud = true

	openParams := environs.OpenParams{
		ControllerUUID: args.ControllerConfig.ControllerUUID(),
		Cloud:          cloudSpec,
		Config:         args.ControllerModelConfig,
	}

	var env environs.BootstrapEnviron
	if isK8s {
		env, err = environsNewK8s(ctx, openParams, environs.NoopCredentialInvalidator())
	} else {
		env, err = environsNewIAAS(ctx, openParams, environs.NoopCredentialInvalidator())
	}
	if err != nil {
		return errors.Capture(err)
	}

	controllerModelConfigAttrs := make(map[string]any)

	// Check to see if a newer agent version has been requested
	// by the bootstrap client.
	desiredVersion, ok := args.ControllerModelConfig.AgentVersion()
	if ok && desiredVersion != jujuversion.Current {
		if isK8s {
			currentVersion := jujuversion.Current
			currentVersion.Build = 0
			if desiredVersion != currentVersion {
				// For K8s, the agent-version in controller config should
				// always equals to current juju version.
				return jujuerrors.NotSupportedf(
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
			if errors.Is(toolsErr, jujuerrors.NotFound) {
				// Newer tools not available, so revert to using the tools
				// matching the current agent version.
				logger.Warningf(context.TODO(), "newer agent binaries for %q not available, sticking with version %q", desiredVersion, jujuversion.Current)
				controllerModelConfigAttrs["agent-version"] = jujuversion.Current.String()
			} else if toolsErr != nil {
				logger.Errorf(context.TODO(), "cannot find newer agent binaries: %v", toolsErr)
				return errors.Capture(toolsErr)
			}
		}
	}

	if err := agentconfig.ReadAgentConfig(c, agent.BootstrapControllerId); err != nil {
		return errors.Errorf("cannot read config: %w", err)
	}
	agentConfig := c.CurrentConfig()
	info, ok := agentConfig.ControllerAgentInfo()
	if !ok {
		return fmt.Errorf("bootstrap machine config has no state serving info")
	}
	if err := ensureKeys(isK8s, &args, &info); err != nil {
		return errors.Capture(err)
	}
	if err := ensureSSHServerHostKey(&args); err != nil {
		return errors.Capture(err)
	}
	addrs, err := bootstrapControllerAddresses(
		ctx, env, args.BootstrapMachineInstanceId,
	)
	if err != nil {
		return errors.Errorf("getting bootstrap controller addresses: %w", err)
	}

	if err = c.ChangeConfig(func(agentConfig agent.ConfigSetter) error {
		agentConfig.SetControllerAgentInfo(info)

		agentConfig.SetQueryTracingEnabled(args.ControllerConfig.QueryTracingEnabled())
		agentConfig.SetQueryTracingThreshold(args.ControllerConfig.QueryTracingThreshold())
		agentConfig.SetDqliteBusyTimeout(args.ControllerConfig.DqliteBusyTimeout())
		agentConfig.SetOpenTelemetryEnabled(agent.DefaultOpenTelemetryEnabled)
		agentConfig.SetOpenTelemetryHTTPEndpoint("")
		agentConfig.SetOpenTelemetryGRPCEndpoint("")
		agentConfig.SetOpenTelemetryInsecure(agent.DefaultOpenTelemetryInsecure)
		agentConfig.SetOpenTelemetryStackTraces(agent.DefaultOpenTelemetryStackTraces)
		agentConfig.SetOpenTelemetrySampleRatio(agent.DefaultOpenTelemetrySampleRatio)
		agentConfig.SetOpenTelemetryTailSamplingThreshold(agent.DefaultOpenTelemetryTailSamplingThreshold)

		return nil
	}); err != nil {
		return fmt.Errorf("cannot write agent config: %v", err)
	}

	agentConfig = c.CurrentConfig()

	// Create system-identity file
	if err := agent.WriteSystemIdentityFile(agentConfig); err != nil {
		return errors.Capture(err)
	}

	controllerModelCfg, err := env.Config().Apply(controllerModelConfigAttrs)
	if err != nil {
		return errors.Errorf("failed to update model config: %w", err)
	}
	args.ControllerModelConfig = controllerModelCfg

	// Initialise state, and store any agent config (e.g. password) changes.
	err = c.ChangeConfig(func(agentConfig agent.ConfigSetter) error {
		adminTag := names.NewLocalUserTag(coreuser.AdminUserName.Name())
		bootstrap, err := c.BootstrapAgent(agentbootstrap.AgentBootstrapArgs{
			AgentConfig:               agentConfig,
			BootstrapEnviron:          env,
			AdminUser:                 adminTag,
			StateInitialisationParams: args,
			BootstrapMachineAddresses: addrs,
			BootstrapDqlite:           c.DqliteInitialiser,
			Logger:                    internallogger.GetLogger("juju.agent.bootstrap"),
		})
		if err != nil {
			return errors.Capture(err)
		}
		return bootstrap.Initialise(ctx)
	})
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

func bootstrapControllerAddresses(
	ctx context.Context, env environs.BootstrapEnviron, providerID instance.Id,
) (network.ProviderAddresses, error) {
	addressFinder, err := environs.NewBootstrapAddressFinder(env, providerID)
	if err != nil {
		return nil, errors.Errorf("selecting bootstrap address provider: %w", err)
	}
	addresses, err := addressFinder.BootstrapControllerAddresses(ctx)
	return addresses, errors.Capture(err)
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
		return errors.Errorf("failed to ensure ssh server host key: %w", err)
	}
	args.SSHServerHostKey = string(hostKey)
	return nil
}

func ensureKeys(
	isK8s bool,
	args *instancecfg.StateInitializationParams,
	info *controller.ControllerAgentInfo,
) error {
	if isK8s {
		return nil
	}
	// Generate a private SSH key for the controllers, and add
	// the public key to the environment config. We'll add the
	// private key to StateServingInfo below.
	privateKey, publicKey, err := sshGenerateKey(controllerdomain.ControllerSSHKeyComment)
	if err != nil {
		return errors.Errorf("failed to generate system key: %w", err)
	}
	info.SystemIdentity = privateKey

	args.ControllerConfig[controller.SystemSSHKeys] = publicKey

	return nil
}
