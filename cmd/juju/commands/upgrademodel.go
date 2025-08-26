// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"
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

A model's agent version can be shown with `[1:] + "`juju model-config agent-version`" + `.
A version is denoted by: ` + "`major.minor.patch`" + `

If ` + "`--agent-version`" + ` is not specified, then the upgrade candidate is
selected to be the exact version the controller itself is running.

If the controller is without internet access, the client must first supply
the software to the controller's cache via the ` + "`juju sync-agent-binary`" + ` command.
The command will abort if an upgrade is in progress. It will also abort if
a previous upgrade was not fully completed (e.g., if one of the
controllers in a high availability model failed to upgrade).

When looking for an agent to upgrade to, Juju will check the currently
configured agent stream for that model. It's possible to overwrite this for
the lifetime of this upgrade using ` + "`--agent-stream`" + `.

If a failed upgrade has been resolved, ` + "`--reset-previous-upgrade`" + ` can be
used to allow the upgrade to proceed.
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

	vers          string
	Version       version.Number
	DryRun        bool
	ResetPrevious bool
	AssumeYes     bool
	AgentStream   string
	timeout       time.Duration
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
	f.BoolVar(&c.ResetPrevious, "reset-previous-upgrade", false, "Clear the previous (incomplete) upgrade status (use with care)")
	f.BoolVar(&c.AssumeYes, "y", false, "Answer 'yes' to confirmation prompts")
	f.BoolVar(&c.AssumeYes, "yes", false, "")
	f.BoolVar(&c.IgnoreAgentVersions, "ignore-agent-versions", false,
		"Don't check if all agents have already reached the current version")
	f.DurationVar(&c.timeout, "timeout", 10*time.Minute, "Timeout before upgrade is aborted")
}

func (c *upgradeModelCommand) Init(args []string) error {
	if c.vers != "" {
		vers, err := version.Parse(c.vers)
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
	ModelGet() (map[string]interface{}, error)
	Close() error
}

// ModelUpgraderAPI defines model upgrader API methods.
type ModelUpgraderAPI interface {
	UpgradeModel(
		modelUUID string, targetVersion version.Number, stream string, ignoreAgentVersions, druRun bool,
	) (version.Number, error)
	AbortModelUpgrade(modelUUID string) error
	UploadTools(r io.ReadSeeker, vers version.Binary) (coretools.List, error)

	Close() error
}

func (c *upgradeModelCommand) getModelUpgraderAPI() (ModelUpgraderAPI, error) {
	if c.modelUpgraderAPI != nil {
		return c.modelUpgraderAPI, nil
	}

	return c.NewModelUpgraderAPIClient()
}

func (c *upgradeModelCommand) getModelConfigAPI() (ModelConfigAPI, error) {
	if c.modelConfigAPI != nil {
		return c.modelConfigAPI, nil
	}

	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelconfig.NewClient(api), nil
}

func (c *upgradeModelCommand) getControllerModelConfigAPI() (ModelConfigAPI, error) {
	if c.controllerModelConfigAPI != nil {
		return c.controllerModelConfigAPI, nil
	}

	api, err := c.NewControllerAPIRoot()
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
			logger.Debugf("upgradeModel failed %v", err)
		}
	}()

	modelUpgrader, err := c.getModelUpgraderAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer modelUpgrader.Close()

	controllerClient, err := c.getControllerModelConfigAPI()
	if err != nil {
		return err
	}
	defer controllerClient.Close()

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

	controllerModelConfig, err := controllerClient.ModelGet()
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
		logger.Debugf("upgraded to %s", targetVersion)
		return nil
	}
	if errors.Is(err, errors.NotFound) {
		return errUpToDate
	}
	return err
}

func (c *upgradeModelCommand) notifyControllerUpgrade(
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

const modelResetPreviousUpgradeMessage = `
WARNING! using --reset-previous-upgrade when an upgrade is in progress
will cause the upgrade to fail. Only use this option to clear an
incomplete upgrade where the root cause has been resolved.

Continue [y/N]? `

func (c *upgradeModelCommand) confirmResetPreviousUpgrade(ctx *cmd.Context) (bool, error) {
	if c.AssumeYes {
		return true, nil
	}
	fmt.Fprint(ctx.Stdout, modelResetPreviousUpgradeMessage)
	scanner := bufio.NewScanner(ctx.Stdin)
	scanner.Scan()
	err := scanner.Err()
	if err != nil && err != io.EOF {
		return false, err
	}
	answer := strings.ToLower(scanner.Text())
	return answer == "y" || answer == "yes", nil
}
