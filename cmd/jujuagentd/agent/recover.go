// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"
	"path"
	"path/filepath"

	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/agent"
	agentconfig "github.com/juju/juju/agent/config"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/controller"
	controllerdomain "github.com/juju/juju/domain/controller"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/errors"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
)

// RecoverCommand prepares the identity and local configuration of a
// replacement controller allocated from a recovery archive. It deliberately
// stops before importing any state: database import is not available in this
// build, and the machine agent's dependency engine does not start while the
// recovery marker exists.
type RecoverCommand struct {
	cmd.CommandBase
	agentconf.AgentConf
	MachineId string
}

// NewRecoverCommand returns a new RecoverCommand that has been initialised.
func NewRecoverCommand() *RecoverCommand {
	return &RecoverCommand{
		AgentConf: agentconf.NewAgentConf(""),
		MachineId: agent.BootstrapControllerId,
	}
}

// Info returns a description of the command.
func (c *RecoverCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "recover-state",
		Purpose: "initialise juju state for controller recovery",
	})
}

// SetFlags adds the flags for this command to the passed gnuflag.FlagSet.
func (c *RecoverCommand) SetFlags(f *gnuflag.FlagSet) {
	c.AgentConf.AddFlags(f)
	f.StringVar(&c.MachineId, "machine-id", agent.BootstrapControllerId, "the archived controller machine ID")
}

// Init initialises the command for running.
func (c *RecoverCommand) Init(args []string) error {
	if err := cmd.CheckEmpty(args); err != nil {
		return err
	}
	return c.AgentConf.CheckArgs(args)
}

// Run reads the archived bootstrap parameters and prepares the replacement
// controller's local configuration.
func (c *RecoverCommand) Run(ctx *cmd.Context) error {
	bootstrapParamsData, err := os.ReadFile(path.Join(c.DataDir(), cloudconfig.FileNameBootstrapParams))
	if err != nil {
		return errors.Errorf("reading bootstrap params file: %w", err)
	}
	var args instancecfg.StateInitializationParams
	if err := args.Unmarshal(bootstrapParamsData); err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(c.initialiseRecoveryController(ctx, args))
}

// initialiseRecoveryController persists the archived controller identity and
// local agent configuration. It never opens the provider, bootstraps the
// controller, or initialises Dqlite: state import is a later recovery stage.
func (c *RecoverCommand) initialiseRecoveryController(
	ctx *cmd.Context, args instancecfg.StateInitializationParams,
) error {
	// The bootstrap params file is written on every fresh bootstrap, so
	// recovery must refuse params that were not produced for a recovery
	// allocation: rewriting agent.conf and system-identity from ordinary
	// bootstrap params would corrupt the machine.
	if !args.RecoveryMode {
		return errors.New("bootstrap params are not for a recovery allocation")
	}
	// The recovery marker is written immediately before recover-state runs;
	// its absence means the machine is not a recovery allocation.
	isRecovery, err := agent.IsRecovery(c.DataDir())
	if err != nil {
		return errors.Errorf("checking recovery marker: %w", err)
	}
	if !isRecovery {
		return errors.New("agent data directory holds no recovery marker")
	}
	isK8s := args.ControllerCloud.Type == cloud.CloudTypeKubernetes
	if err := c.readRecoveryConfig(isK8s); err != nil {
		return errors.Errorf("cannot read config: %w", err)
	}

	agentConfig := c.CurrentConfig()
	info, ok := agentConfig.ControllerAgentInfo()
	if !ok {
		return errors.New("bootstrap machine config has no state serving info")
	}
	if err := ensureKeys(isK8s, &args, &info); err != nil {
		return errors.Capture(err)
	}

	if err := c.ChangeConfig(func(agentConfig agent.ConfigSetter) error {
		agentConfig.SetControllerAgentInfo(info)
		return nil
	}); err != nil {
		return errors.Errorf("cannot write agent config: %w", err)
	}

	if err := agent.WriteSystemIdentityFile(c.CurrentConfig()); err != nil {
		return errors.Capture(err)
	}

	logger.Infof(ctx, "recovery database import is not available in this build")
	return nil
}

func (c *RecoverCommand) readRecoveryConfig(isK8s bool) error {
	err := agentconfig.ReadAgentConfig(c, c.MachineId)
	if err == nil || !isK8s || c.MachineId != agent.BootstrapControllerId {
		return err
	}
	// Older pods could leave a truncated config if their template copy was
	// interrupted. The recovery fence is in place and the seed is available
	// throughout startup, so retry from it when the config is unreadable.
	tag := names.NewControllerAgentTag(agent.BootstrapControllerId)
	if err := copyK8sControllerConfigTemplate(agent.ConfigPath(c.DataDir(), tag)); err != nil {
		return errors.Capture(err)
	}
	return c.ReadConfig(tag.String())
}

// copyK8sControllerConfigTemplate restores a Kubernetes controller's
// agent.conf from its seeded template. The write is atomic: a missing
// template leaves no truncated config behind for the next retry.
func copyK8sControllerConfigTemplate(configPath string) error {
	template := filepath.Join(filepath.Dir(configPath), k8sconstants.TemplateFileNameAgentConf)
	data, err := os.ReadFile(template)
	if err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(utils.AtomicWriteFile(configPath, data, 0600))
}

var sshGenerateKey = ssh.GenerateKey

// ensureKeys generates the controller's system SSH identity for machine
// controllers. Kubernetes controllers provide their identity out of band.
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
