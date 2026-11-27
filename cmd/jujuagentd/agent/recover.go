// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agentbootstrap"
	agentconfig "github.com/juju/juju/agent/config"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	corelogger "github.com/juju/juju/core/logger"
	jujuversion "github.com/juju/juju/core/version"
	controllernodeservice "github.com/juju/juju/domain/controllernode/service"
	controllernodestate "github.com/juju/juju/domain/controllernode/state"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	internalrecovery "github.com/juju/juju/internal/recovery"
)

// RecoverDqliteFunc initialises the replacement controller's Dqlite database.
// It is injectable for tests.
type RecoverDqliteFunc func(
	ctx context.Context,
	mgr database.BootstrapNodeManager,
	bindAddress string,
	machineName string,
	logger corelogger.Logger,
) error

// RecoverCommand prepares a replacement controller's identity. For machine
// controllers it also initialises Dqlite, without importing archived state.
type RecoverCommand struct {
	cmd.CommandBase
	agentconf.AgentConf

	Timeout                   time.Duration
	MachineId                 string
	RecoverDqlite             RecoverDqliteFunc
	DqliteBindAddressResolver agentbootstrap.DqliteBindAddressResolverFunc
}

// NewRecoverCommand returns a new RecoverCommand that has been initialised.
func NewRecoverCommand() *RecoverCommand {
	return &RecoverCommand{
		AgentConf:                 agentconf.NewAgentConf(""),
		MachineId:                 agent.BootstrapControllerId,
		RecoverDqlite:             recoverDqlite,
		DqliteBindAddressResolver: agentbootstrap.NewDqliteBindAddressResolver(logger),
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
	f.DurationVar(&c.Timeout, "timeout", 0, "set the recovery timeout")
	f.StringVar(&c.MachineId, "machine-id", agent.BootstrapControllerId, "the archived controller machine ID")
}

// Init initialises the command for running.
func (c *RecoverCommand) Init(args []string) error {
	if err := cmd.CheckEmpty(args); err != nil {
		return err
	}
	return c.AgentConf.CheckArgs(args)
}

// Run initialises state for a recovered controller.
func (c *RecoverCommand) Run(ctx *cmd.Context) error {
	isRecovery, err := agent.IsRecovery(c.DataDir())
	if err != nil {
		return errors.Errorf("checking recovery marker: %w", err)
	}
	if !isRecovery {
		return errors.New("agent data directory holds no recovery marker")
	}

	paramsData, err := os.ReadFile(filepath.Join(c.DataDir(), internalrecovery.ParamsFile))
	if err != nil {
		return errors.Errorf("reading recovery parameters: %w", err)
	}
	if len(bytes.TrimSpace(paramsData)) == 0 {
		// A replacement pod uses an empty marker as its recovery fence.
		// Only this path needs the separate bootstrap parameters.
		data, err := os.ReadFile(filepath.Join(c.DataDir(), cloudconfig.FileNameBootstrapParams))
		if err != nil {
			return errors.Errorf("reading bootstrap params file: %w", err)
		}
		var node instancecfg.StateInitializationParams
		if err := node.Unmarshal(data); err != nil {
			return errors.Capture(err)
		}
		if !node.RecoveryMode {
			return errors.New("bootstrap params are not for a recovery allocation")
		}
		if node.ControllerCloud.Type != cloud.CloudTypeKubernetes {
			return errors.New("recovery parameters are empty")
		}
		return errors.Capture(c.initialiseK8sRecoveryController(ctx))
	}

	var params internalrecovery.Params
	if err := params.Unmarshal(paramsData); err != nil {
		return errors.Capture(err)
	}
	if !params.Node.RecoveryMode {
		return errors.New("recovery params are not for a recovery allocation")
	}
	if params.Node.ControllerCloud.Type == cloud.CloudTypeKubernetes {
		return errors.Errorf("Kubernetes controller recovery %w", coreerrors.NotSupported)
	}
	return errors.Capture(c.initialiseRecoveryDatabase(ctx, params))
}

// initialiseRecoveryDatabase validates the uploaded archive and initialises
// the replacement Dqlite controller database with its controller node row.
func (c *RecoverCommand) initialiseRecoveryDatabase(ctx *cmd.Context, params internalrecovery.Params) error {
	if started, err := internalrecovery.HasMarker(
		c.DataDir(), internalrecovery.StartedFile, params.SHA256,
	); err != nil {
		return errors.Capture(err)
	} else if started {
		return errors.New("recovery initialisation was already started; provision a new replacement")
	}

	stdCtx := ctx.Context
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		stdCtx, cancel = context.WithTimeout(stdCtx, c.Timeout)
		defer cancel()
	}

	info, err := internalrecovery.ValidateArchive(stdCtx, logger, params.ArchivePath, params.SHA256)
	if err != nil {
		return errors.Capture(err)
	}
	if err := info.CheckAgentVersion(jujuversion.Current); err != nil {
		return errors.Capture(err)
	}

	node := params.Node
	if err := agentconfig.ReadAgentConfig(c, params.MachineName); err != nil {
		return errors.Errorf("reading recovery agent config: %w", err)
	}
	if err := agent.WriteSystemIdentityFile(c.CurrentConfig()); err != nil {
		return errors.Capture(err)
	}

	cloudSpec, err := environscloudspec.MakeCloudSpec(
		node.ControllerCloud, node.ControllerCloudRegion, node.ControllerCloudCredential,
	)
	if err != nil {
		return errors.Capture(err)
	}
	cloudSpec.IsControllerCloud = true
	openParams := environs.OpenParams{
		ControllerUUID: node.ControllerConfig.ControllerUUID(),
		Cloud:          cloudSpec,
		Config:         node.ControllerModelConfig,
	}
	env, err := environsNewIAAS(stdCtx, openParams, environs.NoopCredentialInvalidator())
	if err != nil {
		return errors.Capture(err)
	}

	addresses, err := bootstrapControllerAddresses(stdCtx, env, node.BootstrapMachineInstanceId)
	if err != nil {
		return errors.Errorf("getting replacement controller addresses: %w", err)
	}
	bindAddress, err := c.DqliteBindAddressResolver(stdCtx, addresses)
	if err != nil {
		return errors.Capture(err)
	}

	if err := internalrecovery.BeginInitialisation(c.DataDir(), params.SHA256); err != nil {
		return errors.Capture(err)
	}

	agentConfig := c.CurrentConfig()
	certs, ok := agentConfig.ControllerAgentInfo()
	if !ok {
		return errors.New("recovery machine config has no controller agent information")
	}
	mgr := database.NewNodeManager(database.NodeManagerConfig{
		DataDir:              agentConfig.DataDir(),
		CACert:               agentConfig.CACert(),
		ControllerCert:       certs.Cert,
		ControllerPrivateKey: certs.PrivateKey,
	}, logger, coredatabase.NoopSlowQueryLogger{})
	if err := c.RecoverDqlite(stdCtx, mgr, bindAddress, params.MachineName, logger); err != nil {
		return errors.Capture(err)
	}

	logger.Infof(ctx, "recovery database import is not available in this build; stopped after Dqlite initialisation")
	return nil
}

func (c *RecoverCommand) initialiseK8sRecoveryController(ctx *cmd.Context) error {
	if err := c.readK8sRecoveryConfig(); err != nil {
		return errors.Errorf("cannot read config: %w", err)
	}
	info, ok := c.CurrentConfig().ControllerAgentInfo()
	if !ok {
		return errors.New("bootstrap machine config has no state serving info")
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

func (c *RecoverCommand) readK8sRecoveryConfig() error {
	err := agentconfig.ReadAgentConfig(c, c.MachineId)
	if err == nil || c.MachineId != agent.BootstrapControllerId {
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

// recoverDqlite wires the controller node service to the recovery session.
func recoverDqlite(
	ctx context.Context,
	mgr database.BootstrapNodeManager,
	bindAddress string,
	machineName string,
	logger corelogger.Logger,
) error {
	newControllerNodeService := func(runner coredatabase.TxnRunner) database.ControllerNodeService {
		state := controllernodestate.NewState(func(context.Context) (coredatabase.TxnRunner, error) {
			return runner, nil
		})
		return controllernodeservice.NewService(state, logger)
	}
	return database.RecoverDqlite(ctx, mgr, newControllerNodeService, bindAddress, machineName, logger)
}
