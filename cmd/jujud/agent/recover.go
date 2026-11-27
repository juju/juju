// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/controller"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/controllerruntimeconfig"
)

// RecoverCommand prepares the identity and local configuration of a
// replacement controller allocated from a recovery archive. It deliberately
// stops before importing any state: database import is not available in this
// build, and the controller application's dependency engine does not start
// while the recovery marker exists.
type RecoverCommand struct {
	cmd.CommandBase
	agentconf.AgentConf
}

// NewRecoverCommand returns a new RecoverCommand that has been initialised.
func NewRecoverCommand() *RecoverCommand {
	return &RecoverCommand{
		AgentConf: agentconf.NewAgentConf(""),
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
}

// Init initialises the command for running.
func (c *RecoverCommand) Init(args []string) error {
	if err := cmd.CheckEmpty(args); err != nil {
		return errors.Trace(err)
	}
	return c.AgentConf.CheckArgs(args)
}

// Run reads the archived bootstrap parameters and prepares the replacement
// controller's local configuration.
func (c *RecoverCommand) Run(ctx *cmd.Context) error {
	bootstrapParamsData, err := os.ReadFile(bootstrap.BootstrapParamsPath(c.DataDir()))
	if err != nil {
		return errors.Annotate(err, "reading bootstrap params file")
	}
	var args instancecfg.StateInitializationParams
	if err := args.Unmarshal(bootstrapParamsData); err != nil {
		return errors.Annotate(err, "unmarshalling bootstrap params")
	}
	return errors.Annotate(c.initialiseRecoveryController(ctx, args), "cannot initialise recovery controller")
}

// initialiseRecoveryController persists the archived controller identity and
// local configuration. It never opens the provider, bootstraps the controller,
// or initialises Dqlite: state import is a later recovery stage. The runtime
// config is never written as an agent.conf under agents/controller-<id>.
func (c *RecoverCommand) initialiseRecoveryController(
	ctx *cmd.Context, args instancecfg.StateInitializationParams,
) error {
	// The bootstrap params file is written on every fresh bootstrap, so
	// recovery must refuse params that were not produced for a recovery
	// allocation: rewriting the runtime config and system identity from
	// ordinary bootstrap params would corrupt the controller.
	if !args.RecoveryMode {
		return errors.New("bootstrap params are not for a recovery allocation")
	}
	// The recovery marker is staged immediately before recover-state runs;
	// its absence means the controller is not a recovery allocation.
	isRecovery, err := agent.IsRecovery(os.Getenv("SNAP_COMMON"))
	if err != nil {
		return errors.Annotate(err, "checking recovery marker")
	}
	if !isRecovery {
		return errors.New("controller data directories hold no recovery marker")
	}
	if args.ControllerCloud.Type == cloud.CloudTypeKubernetes {
		return errors.NotSupportedf("Kubernetes controller recovery")
	}

	// Resolve the runtime.conf path. For IAAS snap controllers, SNAP_DATA
	// overrides the --data-dir flag (see runFromRuntimeConf).
	runtimeCfgDir := c.DataDir()
	if snapData := os.Getenv("SNAP_DATA"); snapData != "" {
		runtimeCfgDir = snapData
	}
	runtimeCfgPath := filepath.Join(runtimeCfgDir, controllerruntimeconfig.Filename)

	runtimeCfg, err := controllerruntimeconfig.ReadControllerRuntimeConfig(runtimeCfgPath)
	if err != nil {
		return errors.Annotate(err, "reading controller runtime config")
	}

	// Generate a fresh system identity for the replacement controller. The
	// archived controller config carries no system identity, so the
	// replacement gets its own key pair; the public half is not propagated
	// into the archived controller config.
	info := controller.ControllerAgentInfo{
		Cert:           runtimeCfg.ControllerCert,
		PrivateKey:     runtimeCfg.ControllerPrivateKey,
		CAPrivateKey:   runtimeCfg.CAPrivateKey,
		APIPort:        runtimeCfg.APIPort,
		SystemIdentity: runtimeCfg.SystemIdentity,
	}
	if err := ensureKeys(false, &args, &info); err != nil {
		return errors.Trace(err)
	}

	// Persist the generated system identity back to runtime.conf. The
	// controller application reads its identity from there at engine start.
	if err := controllerruntimeconfig.ChangeControllerRuntimeConfig(runtimeCfgPath, func(cfg *controllerruntimeconfig.ControllerRuntimeConfig) error {
		cfg.SystemIdentity = info.SystemIdentity
		return nil
	}); err != nil {
		return errors.Annotate(err, "persisting identity mutations to runtime config")
	}

	// Write the system identity file to the snap-private DataDir. The
	// in-memory agent config is never written to disk as agent.conf.
	controllerTag := names.NewControllerTag(runtimeCfg.ControllerUUID)
	modelTag := names.NewModelTag(runtimeCfg.ControllerModelUUID)
	controllerAgentTag := names.NewControllerAgentTag(runtimeCfg.ControllerID)
	agentConfigParams := agent.AgentConfigParams{
		Paths: agent.Paths{
			DataDir: runtimeCfg.DataDir,
			LogDir:  runtimeCfg.LogDir,
		},
		Tag:               controllerAgentTag,
		UpgradedToVersion: jujuversion.Current,
		Password:          runtimeCfg.AgentPassword,
		Controller:        controllerTag,
		Model:             modelTag,
		APIAddresses:      runtimeCfg.APIAddresses,
		CACert:            runtimeCfg.CACert,
	}
	agentCfgWriter, err := agent.NewStateMachineConfig(agentConfigParams, info)
	if err != nil {
		return errors.Annotate(err, "building in-memory controller agent config")
	}
	if err := agent.WriteSystemIdentityFile(agentCfgWriter); err != nil {
		return errors.Trace(err)
	}

	logger.Infof(ctx, "recovery database import is not available in this build")
	return nil
}
