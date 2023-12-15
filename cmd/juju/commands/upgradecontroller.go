// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api/client/modelconfig"
	"github.com/juju/juju/api/client/modelupgrader"
	apicontroller "github.com/juju/juju/api/controller/controller"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/sync"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
	jujuversion "github.com/juju/juju/version"
)

var usageUpgradeControllerSummary = `
Upgrades Juju on a controller.`[1:]

var usageUpgradeControllerDetails = `
This command upgrades the Juju agent for a controller.

A controller's agent version can be shown with `[1:] + "`juju model-config -m controller agent-\nversion`" + `.
A version is denoted by: major.minor.patch

You can upgrade the controller to a new patch version by specifying
the '--agent-version' flag. If not specified, the upgrade candidate
will default to the most recent patch version matching the current 
major and minor version. Upgrading to a new major or minor version is
not supported.

The command will abort if an upgrade is in progress. It will also abort if
a previous upgrade was not fully completed (e.g.: if one of the
controllers in a high availability model failed to upgrade).

`

const usageUpgradeControllerExamples = `
    juju upgrade-controller --dry-run
    juju upgrade-controller --agent-version 2.0.1
`

func newUpgradeControllerCommand(options ...modelcmd.WrapControllerOption) cmd.Command {
	command := &upgradeControllerCommand{}
	return modelcmd.WrapController(command, options...)
}

// upgradeControllerCommand upgrades the controller agents in a juju installation.
type upgradeControllerCommand struct {
	modelcmd.ControllerCommandBase
	baseUpgradeCommand

	controllerAPI ControllerAPI

	Dev                               bool
	JujudControllerSnapPath           string
	JujudControllerSnapAssertionsPath string

	fullControllerModelName string
	controllerModelDetails  *jujuclient.ModelDetails

	devSrcDir string
}

// ControllerAPI defines the controller API methods.
type ControllerAPI interface {
	CloudSpec(modelTag names.ModelTag) (environscloudspec.CloudSpec, error)
	ControllerConfig() (controller.Config, error)
	ModelConfig() (map[string]interface{}, error)
	Close() error
}

func (c *upgradeControllerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "upgrade-controller",
		Purpose:  usageUpgradeControllerSummary,
		Doc:      usageUpgradeControllerDetails,
		Examples: usageUpgradeControllerExamples,
		SeeAlso: []string{
			"upgrade-model",
		},
	})
}

func (c *upgradeControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	c.baseUpgradeCommand.SetFlags(f)
	f.StringVar(&c.JujudControllerSnapPath, "snap", "",
		"Path to a locally built .snap to use as the internal jujud-controller service.")
	f.StringVar(&c.JujudControllerSnapAssertionsPath, "snap-asserts", "", "Path to a local .assert file or dangerous. Requires --snap")
	if jujuversion.Current.Build > 0 {
		f.BoolVar(&c.Dev, "dev", false, "Use local build for development")
	}
}

func (c *upgradeControllerCommand) Init(args []string) error {
	err := c.baseUpgradeCommand.Init(args)
	if err != nil {
		return err
	}

	if c.Dev {
		_, b, _, _ := runtime.Caller(0)
		modCmd := exec.Command("go", "list", "-m", "-json")
		modCmd.Dir = filepath.Dir(b)
		modInfo, err := modCmd.Output()
		if err != nil {
			return fmt.Errorf("--dev requires juju binary to be built locally: %w", err)
		}
		mod := struct {
			Path string `json:"Path"`
			Dir  string `json:"Dir"`
		}{}
		err = json.Unmarshal(modInfo, &mod)
		if err != nil {
			return fmt.Errorf("--dev requires juju binary to be built locally: %w", err)
		}
		if mod.Path != "github.com/juju/juju" {
			return fmt.Errorf("cannot use juju binary built for --dev")
		}
		c.devSrcDir = mod.Dir
	}
	if c.AgentVersionParam != "" && c.Dev {
		return errors.New("--agent-version and --dev can't be used together")
	}
	return cmd.CheckEmpty(args)
}

func (c *upgradeControllerCommand) Run(ctx *cmd.Context) (err error) {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	store := c.ClientStore()
	accDetails, err := store.AccountDetails(controllerName)
	if err != nil {
		return errors.Trace(err)
	}

	if !permission.Access(accDetails.LastKnownAccess).EqualOrGreaterControllerAccessThan(permission.SuperuserAccess) {
		return errors.Errorf("upgrade not possible missing"+
			" permissions, current level %q, need: %q", accDetails.LastKnownAccess, permission.SuperuserAccess)
	}
	controllerModel := jujuclient.JoinOwnerModelName(
		names.NewUserTag(environs.AdminUser), bootstrap.ControllerModelName)
	c.controllerModelDetails, err = store.ModelByName(controllerName, controllerModel)
	if err != nil {
		return errors.Annotatef(err, "cannot get controller model")
	}
	c.fullControllerModelName = modelcmd.JoinModelName(controllerName, controllerModel)

	return c.upgradeController(ctx, c.Timeout, c.controllerModelDetails.ModelType)
}

func (c *upgradeControllerCommand) getModelUpgraderAPI() (ModelUpgraderAPI, error) {
	if c.modelUpgraderAPI != nil {
		return c.modelUpgraderAPI, nil
	}

	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelupgrader.NewClient(root), nil
}

func (c *upgradeControllerCommand) getModelConfigAPI() (ModelConfigAPI, error) {
	if c.modelConfigAPI != nil {
		return c.modelConfigAPI, nil
	}

	api, err := c.NewModelAPIRoot(c.fullControllerModelName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelconfig.NewClient(api), nil
}

func (c *upgradeControllerCommand) getControllerAPI() (ControllerAPI, error) {
	if c.controllerAPI != nil {
		return c.controllerAPI, nil
	}

	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apicontroller.NewClient(api), nil
}

func (c *upgradeControllerCommand) upgradeController(
	ctx *cmd.Context, fetchTimeout time.Duration,
	modelType model.ModelType,
) (err error) {
	targetVersion := c.Version
	defer func() {
		if err == nil {
			fmt.Fprintf(ctx.Stderr, "best version:\n    %v\n", targetVersion)
			if c.DryRun {
				if c.Dev {
					fmt.Fprintf(ctx.Stderr, "%s --dev\n", c.upgradeMessage)
				} else {
					fmt.Fprintf(ctx.Stderr, "%s\n", c.upgradeMessage)
				}
			} else {
				fmt.Fprintf(ctx.Stdout, "started upgrade to %s\n", targetVersion)
			}
		}

		if err == errUpToDate {
			ctx.Infof(err.Error())
			err = nil
		}
		if err != nil {
			logger.Debugf("upgradeController failed %v", err)
		}
	}()

	modelUpgrader, err := c.getModelUpgraderAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer modelUpgrader.Close()

	controllerClient, err := c.getControllerAPI()
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
		return errors.New("incomplete controller model configuration")
	}
	if targetVersion == agentVersion {
		return errUpToDate
	}

	if c.Dev {
		if targetVersion != version.Zero {
			return errors.Errorf("--dev cannot be used with --agent-version together")
		}
		if targetVersion, err = c.uploadTools(modelUpgrader, targetVersion, c.DryRun); err != nil {
			return block.ProcessBlockedError(err, block.BlockChange)
		}
		builtMsg := " (built from source)"
		fmt.Fprintf(ctx.Stdout,
			"no prepackaged agent binaries available, using local agent binary %v%s\n",
			targetVersion, builtMsg,
		)
		targetVersion, err = c.notifyControllerUpgrade(ctx, modelUpgrader, targetVersion, c.DryRun)
		return err
	}

	targetVersion, err = c.notifyControllerUpgrade(
		ctx, modelUpgrader,
		version.Zero, // no target version provided, we figure it out on the server side.
		c.DryRun,
	)
	if errors.Is(err, errors.NotFound) {
		return errUpToDate
	} else if err != nil {
		return err
	}

	// All good!
	// Upgraded to a next stable version or the newest stable version.
	logger.Debugf("upgraded to a next version or latest stable version")
	return nil
}

func (c *upgradeControllerCommand) notifyControllerUpgrade(
	ctx *cmd.Context, modelUpgrader ModelUpgraderAPI, targetVersion version.Number, dryRun bool,
) (chosenVersion version.Number, err error) {
	modelTag := names.NewModelTag(c.controllerModelDetails.ModelUUID)

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
				"upgrade-controller command with the --reset-previous-upgrade option.", err,
			)
		}
		if errors.Is(err, errors.AlreadyExists) {
			err = errUpToDate
		}
		return chosenVersion, block.ProcessBlockedError(err, block.BlockChange)
	}
	return chosenVersion, nil
}

func (c *upgradeControllerCommand) uploadTools(modelUpgrader ModelUpgraderAPI, agentVersion version.Number, dryRun bool) (version.Number, error) {
	// TODO: arch handling here
	builtTools, err := sync.BuildAgentTarball(c.devSrcDir, "upgrade", arch.AMD64)
	if err != nil {
		return version.Zero, errors.Trace(err)
	}
	defer os.RemoveAll(builtTools.Dir)

	if dryRun {
		logger.Debugf("dryrun, skipping upload agent binary")
		return version.Zero, nil
	}

	toolsPath := path.Join(builtTools.Dir, builtTools.StorageName)
	logger.Infof("uploading agent binary %v (%dkB) to Juju controller", builtTools.Version, (builtTools.Size+512)/1024)
	f, err := os.Open(toolsPath)
	if err != nil {
		return version.Zero, errors.Trace(err)
	}
	defer f.Close()

	_, err = modelUpgrader.UploadTools(f, builtTools.Version)
	if err != nil {
		return version.Zero, errors.Trace(err)
	}
	return builtTools.Version.Number, nil
}
