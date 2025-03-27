// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/client/modelconfig"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cmd"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

var usageUpgradeJujuSummary = `
Upgrades Juju on all machines in a model.`[1:]

var usageUpgradeJujuDetails = `
Juju provides agent software to every machine it creates. This command
upgrades that software across an entire model, which is, by default, the
current model.
A model's agent version can be shown with `[1:] + "`juju model-config agent-version`" + `.
A version is denoted by: major.minor.patch

If '--agent-version' is not specified, then the upgrade candidate is
selected to be the exact version the controller itself is running.

If the controller is without internet access, the client must first supply
the software to the controller's cache via the ` + "`juju sync-agent-binary`" + ` command.
The command will abort if an upgrade is in progress. It will also abort if
a previous upgrade was not fully completed (e.g.: if one of the
controllers in a high availability model failed to upgrade).

When looking for an agent to upgrade to, Juju will check the currently
configured agent stream for that model. It's possible to overwrite this for
the lifetime of this upgrade using --agent-stream

Backups are recommended prior to upgrading.

`

const usageUpgradeJujuExamples = `
    juju upgrade-model --dry-run
    juju upgrade-model --agent-version 2.0.1
    juju upgrade-model --agent-stream proposed
`

const upgradeModelMessage = "upgrade to this version by running\n    juju upgrade-model"

func newUpgradeModelCommand() cmd.Command {
	command := &upgradeModelCommand{}
	return modelcmd.Wrap(command)
}

// upgradeModelCommand upgrades the agents in a juju installation.
type upgradeModelCommand struct {
	modelcmd.ModelCommandBase

	vers        string
	Version     semversion.Number
	DryRun      bool
	AssumeYes   bool
	AgentStream string
	timeout     time.Duration
	// IgnoreAgentVersions is used to allow an admin to request an agent
	// version without waiting for all agents to be at the right version.
	IgnoreAgentVersions bool

	// model config API for the current model
	modelConfigAPI   ModelConfigAPI
	modelUpgraderAPI ModelUpgraderAPI
	// model config API for the controller model
	controllerModelConfigAPI ModelConfigAPI
}

func (c *upgradeModelCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "upgrade-model",
		Purpose:  usageUpgradeJujuSummary,
		Doc:      usageUpgradeJujuDetails,
		Examples: usageUpgradeJujuExamples,
		SeeAlso: []string{
			"sync-agent-binary",
		},
	})
}

func (c *upgradeModelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)

	f.StringVar(&c.vers, "agent-version", "", "Upgrade to specific version")
	f.StringVar(&c.AgentStream, "agent-stream", "", "Check this agent stream for upgrades")
	f.BoolVar(&c.DryRun, "dry-run", false, "Don't change anything, just report what would be changed")
	f.BoolVar(&c.AssumeYes, "y", false, "Answer 'yes' to confirmation prompts")
	f.BoolVar(&c.AssumeYes, "yes", false, "")
	f.BoolVar(&c.IgnoreAgentVersions, "ignore-agent-versions", false,
		"Don't check if all agents have already reached the current version")
	f.DurationVar(&c.timeout, "timeout", 10*time.Minute, "Timeout before upgrade is aborted")
}

func (c *upgradeModelCommand) Init(args []string) error {
	if c.vers != "" {
		vers, err := semversion.Parse(c.vers)
		if err != nil {
			return err
		}
		c.Version = vers
	}
	return cmd.CheckEmpty(args)
}

const (
	errUpToDate errors.ConstError = "no upgrades available"
)

// ModelConfigAPI defines the model config API methods.
type ModelConfigAPI interface {
	ModelGet(ctx context.Context) (map[string]interface{}, error)
	Close() error
}

// ModelUpgraderAPI defines model upgrader API methods.
type ModelUpgraderAPI interface {
	UpgradeModel(
		ctx context.Context,
		modelUUID string, targetVersion semversion.Number, stream string, ignoreAgentVersions, druRun bool,
	) (semversion.Number, error)
	UploadTools(ctx context.Context, r io.Reader, vers semversion.Binary) (coretools.List, error)

	Close() error
}

func (c *upgradeModelCommand) getModelUpgraderAPI(ctx context.Context) (ModelUpgraderAPI, error) {
	if c.modelUpgraderAPI != nil {
		return c.modelUpgraderAPI, nil
	}

	return c.NewModelUpgraderAPIClient(ctx)
}

func (c *upgradeModelCommand) getModelConfigAPI(ctx context.Context) (ModelConfigAPI, error) {
	if c.modelConfigAPI != nil {
		return c.modelConfigAPI, nil
	}

	api, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelconfig.NewClient(api), nil
}

func (c *upgradeModelCommand) getControllerModelConfigAPI(ctx context.Context) (ModelConfigAPI, error) {
	if c.controllerModelConfigAPI != nil {
		return c.controllerModelConfigAPI, nil
	}

	api, err := c.NewControllerAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelconfig.NewClient(api), nil
}

// Run changes the version proposed for the juju envtools.
func (c *upgradeModelCommand) Run(ctx *cmd.Context) (err error) {
	return c.upgradeModel(ctx, c.timeout)
}

func (c *upgradeModelCommand) upgradeModel(ctx *cmd.Context, fetchTimeout time.Duration) (err error) {
	targetVersion := c.Version
	defer func() {
		if err == nil {
			fmt.Fprintf(ctx.Stderr, "best version:\n    %v\n", targetVersion)
			if c.DryRun {
				fmt.Fprintf(ctx.Stderr, "%s\n", upgradeModelMessage)
			} else {
				fmt.Fprintf(ctx.Stdout, "started upgrade to %s\n", targetVersion)
			}
		}

		if errors.Is(err, errUpToDate) {
			ctx.Infof("%s", err.Error())
			err = nil
		}
		if err != nil {
			logger.Debugf(context.TODO(), "upgradeModel failed %v", err)
		}
	}()

	modelUpgrader, err := c.getModelUpgraderAPI(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer modelUpgrader.Close()

	controllerClient, err := c.getControllerModelConfigAPI(ctx)
	if err != nil {
		return err
	}
	defer controllerClient.Close()

	modelConfigClient, err := c.getModelConfigAPI(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer modelConfigClient.Close()

	attrs, err := modelConfigClient.ModelGet(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return errors.Trace(err)
	}

	agentVersion, ok := cfg.AgentVersion()
	if !ok {
		// Can't happen. In theory.
		return errors.New("incomplete model configuration")
	}

	if c.Version == agentVersion {
		return errUpToDate
	}

	controllerModelConfig, err := controllerClient.ModelGet(ctx)
	if err != nil && !params.IsCodeUnauthorized(err) {
		return err
	}
	haveControllerModelPermission := err == nil
	isControllerModel := haveControllerModelPermission && cfg.UUID() == controllerModelConfig[config.UUIDKey]
	if isControllerModel {
		return errors.Errorf("use upgrade-controller to upgrade the controller model")
	}

	targetVersion, err = c.notifyControllerUpgrade(ctx, modelUpgrader, targetVersion, c.DryRun)
	if err == nil {
		logger.Debugf(context.TODO(), "upgraded to %s", targetVersion)
		return nil
	}
	if errors.Is(err, errors.NotFound) {
		return errUpToDate
	}
	return err
}

func (c *upgradeModelCommand) notifyControllerUpgrade(
	ctx *cmd.Context, modelUpgrader ModelUpgraderAPI, targetVersion semversion.Number, dryRun bool,
) (chosenVersion semversion.Number, err error) {
	_, details, err := c.ModelCommandBase.ModelDetails(ctx)
	if err != nil {
		return chosenVersion, errors.Trace(err)
	}
	modelTag := names.NewModelTag(details.ModelUUID)

	if chosenVersion, err = modelUpgrader.UpgradeModel(
		ctx,
		modelTag.Id(), targetVersion, c.AgentStream, c.IgnoreAgentVersions, dryRun,
	); err != nil {
		if params.IsCodeUpgradeInProgress(err) {
			return chosenVersion, errors.Errorf("%s\n\n"+
				"Please wait for the upgrade to complete.", err,
			)
		}
		if errors.Is(err, errors.AlreadyExists) {
			err = errUpToDate
		}
		return chosenVersion, block.ProcessBlockedError(err, block.BlockChange)
	}
	return chosenVersion, nil
}
