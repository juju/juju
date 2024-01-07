// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	stderrors "errors"
	"fmt"
	"io"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api/client/modelconfig"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	coretools "github.com/juju/juju/tools"
)

var usageUpgradeJujuSummary = `
Upgrades Juju on all machines in a model.`[1:]

var usageUpgradeJujuDetails = `
Juju provides agent software to every machine it creates. This command
upgrades that software across an entire model, which is, by default, the
current model.
A model's agent version can be shown with `[1:] + "`juju model-config agent-\nversion`" + `.
A version is denoted by: major.minor.patch

If '--agent-version' is not specified, then the upgrade candidate is
selected to be the exact version the controller itself is running.

If the controller is without internet access, the client must first supply
the software to the controller's cache via the ` + "`juju sync-agent-binary`" + ` command.
The command will abort if an upgrade is in progress. It will also abort if
a previous upgrade was not fully completed (e.g.: if one of the
controllers in a high availability model failed to upgrade).

When looking for an agent to upgrade to Juju will check the currently
configured agent stream for that model. It's possible to overwrite this for
the lifetime of this upgrade using --agent-stream

If a failed upgrade has been resolved, '--reset-previous-upgrade' can be
used to allow the upgrade to proceed.
Backups are recommended prior to upgrading.

`

const usageUpgradeJujuExamples = `
    juju upgrade-model --dry-run
    juju upgrade-model --agent-version 2.0.1
    juju upgrade-model --agent-stream proposed
`

func newUpgradeJujuCommand() cmd.Command {
	command := &upgradeJujuCommand{}
	return modelcmd.Wrap(command)
}

// upgradeJujuCommand upgrades the agents in a juju installation.
type upgradeJujuCommand struct {
	modelcmd.ModelCommandBase
	baseUpgradeCommand
}

func (c *upgradeJujuCommand) Info() *cmd.Info {
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

func (c *upgradeJujuCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.baseUpgradeCommand.SetFlags(f)
}

var (
	errUpToDate = stderrors.New("no upgrades available")
)

// ModelConfigAPI defines the model config API methods.
type ModelConfigAPI interface {
	ModelGet() (map[string]interface{}, error)
	Close() error
}

// ModelUpgraderAPI defines model upgrader API methods.
type ModelUpgraderAPI interface {
	UpgradeModel(
		modelUUID string, targetVersion version.Number, stream string, ignoreAgentVersions, dryRun bool,
	) (version.Number, error)
	AbortModelUpgrade(modelUUID string) error
	UploadTools(r io.ReadSeeker, vers version.Binary) (coretools.List, error)

	Close() error
}

func (c *upgradeJujuCommand) getModelUpgraderAPI() (ModelUpgraderAPI, error) {
	if c.modelUpgraderAPI != nil {
		return c.modelUpgraderAPI, nil
	}

	return c.NewModelUpgraderAPIClient()
}

func (c *upgradeJujuCommand) getModelConfigAPI() (ModelConfigAPI, error) {
	if c.modelConfigAPI != nil {
		return c.modelConfigAPI, nil
	}

	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelconfig.NewClient(api), nil
}

// Run changes the version proposed for the juju envtools.
func (c *upgradeJujuCommand) Run(ctx *cmd.Context) (err error) {
	return c.upgradeModel(ctx, c.Timeout)
}

func (c *upgradeJujuCommand) upgradeModel(ctx *cmd.Context, fetchTimeout time.Duration) (err error) {
	targetVersion := c.Version
	defer func() {
		if err == nil {
			fmt.Fprintf(ctx.Stderr, "best version:\n    %v\n", targetVersion)
			if c.DryRun {
				fmt.Fprintf(ctx.Stderr, "%s\n", c.upgradeMessage)
			} else {
				fmt.Fprintf(ctx.Stdout, "started upgrade to %s\n", targetVersion)
			}
		}

		if err == errUpToDate {
			ctx.Infof(err.Error())
			err = nil
		}
		if err != nil {
			logger.Debugf("upgradeModel failed %v", err)
		}
	}()

	modelUpgrader, err := c.getModelUpgraderAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer modelUpgrader.Close()

	modelConfigClient, err := c.getModelConfigAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer modelConfigClient.Close()

	attrs, err := modelConfigClient.ModelGet()
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

	targetVersion, err = c.notifyControllerUpgrade(ctx, modelUpgrader, c.Version, c.DryRun)
	if err == nil {
		// All good!
		// Upgraded to a next stable version or the newest stable version.
		logger.Debugf("upgraded to a next version or latest stable version")
		return nil
	}
	if errors.Is(err, errors.NotFound) {
		return errUpToDate
	}
	return err
}

func (c *upgradeJujuCommand) notifyControllerUpgrade(
	ctx *cmd.Context, modelUpgrader ModelUpgraderAPI, targetVersion version.Number, dryRun bool,
) (chosenVersion version.Number, err error) {
	_, details, err := c.ModelCommandBase.ModelDetails()
	if err != nil {
		return chosenVersion, errors.Trace(err)
	}
	modelTag := names.NewModelTag(details.ModelUUID)

	if c.ResetPrevious {
		if ok, err := c.confirmResetPreviousUpgrade(ctx); !ok || err != nil {
			const message = "previous upgrade not reset and no new upgrade triggered"
			if err != nil {
				return chosenVersion, errors.Annotate(err, message)
			}
			return chosenVersion, errors.New(message)
		}
		if err := modelUpgrader.AbortModelUpgrade(modelTag.Id()); err != nil {
			return chosenVersion, block.ProcessBlockedError(err, block.BlockChange)
		}
	}
	if chosenVersion, err = modelUpgrader.UpgradeModel(
		modelTag.Id(), targetVersion, c.AgentStream, c.IgnoreAgentVersions, dryRun,
	); err != nil {
		if params.IsCodeUpgradeInProgress(err) {
			return chosenVersion, errors.Errorf("%s\n\n"+
				"Please wait for the upgrade to complete or if there was a problem with\n"+
				"the last upgrade that has been resolved, consider running the\n"+
				"upgrade-model command with the --reset-previous-upgrade option.", err,
			)
		}
		if errors.Is(err, errors.AlreadyExists) {
			err = errUpToDate
		}
		return chosenVersion, block.ProcessBlockedError(err, block.BlockChange)
	}
	return chosenVersion, nil
}
