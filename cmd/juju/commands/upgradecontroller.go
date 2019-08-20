// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/version"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/cloudconfig/podcfg"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

var usageUpgradeControllerSummary = `
Upgrades Juju on a controller.`[1:]

var usageUpgradeControllerDetails = `
This command upgrades the Juju agent for a controller.

A controller's agent version can be shown with `[1:] + "`juju model-config -m controller agent-\nversion`" + `.
A version is denoted by: major.minor.patch
The upgrade candidate will be auto-selected if '--agent-version' is not
specified:
 - If the server major version matches the client major version, the
 version selected is minor+1. If such a minor version is not available then
 the next patch version is chosen.
 - If the server major version does not match the client major version,
 the version selected is that of the client version.
The command will abort if an upgrade is in progress. It will also abort if
a previous upgrade was not fully completed (e.g.: if one of the
controllers in a high availability model failed to upgrade).

Examples:
    juju upgrade-controller --dry-run
    juju upgrade-controller --agent-version 2.0.1
    
See also: 
    upgrade-model`

func newUpgradeControllerCommand(options ...modelcmd.WrapControllerOption) cmd.Command {
	cmd := &upgradeControllerCommand{}
	return modelcmd.WrapController(cmd, options...)
}

// upgradeControllerCommand upgrades the controller agents in a juju installation.
type upgradeControllerCommand struct {
	modelcmd.ControllerCommandBase
	baseUpgradeCommand

	upgradeJujuAPI upgradeJujuAPI
	rawArgs        []string
}

func (c *upgradeControllerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "upgrade-controller",
		Purpose: usageUpgradeControllerSummary,
		Doc:     usageUpgradeControllerDetails,
	})
}

func (c *upgradeControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	c.baseUpgradeCommand.SetFlags(f)
}

func (c *upgradeControllerCommand) getUpgradeJujuAPI() (upgradeJujuAPI, error) {
	if c.upgradeJujuAPI != nil {
		return c.upgradeJujuAPI, nil
	}

	root, err := c.NewModelAPIRoot(bootstrap.ControllerModelName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return root.Client(), nil
}

func (c *upgradeControllerCommand) getModelConfigAPI() (modelConfigAPI, error) {
	if c.modelConfigAPI != nil {
		return c.modelConfigAPI, nil
	}

	api, err := c.NewModelAPIRoot(bootstrap.ControllerModelName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelconfig.NewClient(api), nil
}

func (c *upgradeControllerCommand) getControllerAPI() (controllerAPI, error) {
	if c.controllerAPI != nil {
		return c.controllerAPI, nil
	}

	return c.NewControllerAPIClient()
}

func (c *upgradeControllerCommand) Run(ctx *cmd.Context) (err error) {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	controllerModel := jujuclient.JoinOwnerModelName(
		names.NewUserTag(environs.AdminUser), bootstrap.ControllerModelName)
	_, err = c.ModelUUIDs([]string{controllerModel})
	if err != nil {
		return errors.Annotatef(err, "cannot get controller model uuid")
	}
	details, err := c.ClientStore().ModelByName(controllerName, controllerModel)
	if err != nil {
		return errors.Trace(err)
	}
	if details.ModelType == model.CAAS {
		return c.upgradeCAASController(ctx)
	}
	return c.upgradeIAASController(ctx)
}

func (c *upgradeControllerCommand) upgradeCAASController(ctx *cmd.Context) error {
	if c.BuildAgent {
		return errors.NotSupportedf("--build-agent for k8s controller upgrades")
	}
	client, err := c.getUpgradeJujuAPI()
	if err != nil {
		return err
	}
	defer client.Close()
	modelConfigClient, err := c.getModelConfigAPI()
	if err != nil {
		return err
	}
	defer modelConfigClient.Close()
	controllerAPI, err := c.getControllerAPI()
	if err != nil {
		return err
	}
	defer controllerAPI.Close()

	defer func() {
		if err == errUpToDate {
			ctx.Infof(err.Error())
			err = nil
		}
	}()

	// Determine the version to upgrade to.
	attrs, err := modelConfigClient.ModelGet()
	if err != nil {
		return err
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return err
	}

	currentAgentVersion, ok := cfg.AgentVersion()
	if !ok {
		// Can't happen. In theory.
		return errors.New("incomplete model configuration")
	}

	warnCompat, err := c.precheck(ctx, currentAgentVersion)
	if err != nil {
		return err
	}

	controllerCfg, err := controllerAPI.ControllerConfig()
	if err != nil {
		return err
	}

	c.upgradeMessage = "upgrade to this version by running\n    juju upgrade-controller"
	context, err := initCAASVersions(controllerCfg, c.Version, currentAgentVersion, warnCompat)
	if err != nil {
		return err
	}

	if err := context.maybeChoosePackagedAgent(); err != nil {
		ctx.Verbosef("%v", err)
		return err
	}

	if err := context.validate(); err != nil {
		return err
	}
	ctx.Verbosef("available agent images:\n%s", formatVersions(context.packagedAgents))
	fmt.Fprintf(ctx.Stderr, "best version:\n    %v\n", context.chosen)
	if warnCompat {
		fmt.Fprintf(ctx.Stderr, "version %s incompatible with this client (%s)\n", context.chosen, jujuversion.Current)
	}
	if c.DryRun {
		fmt.Fprintf(ctx.Stderr, "%s\n", c.upgradeMessage)
		return nil
	}
	return c.notifyControllerUpgrade(ctx, client, context)
}

// initCAASVersions collects state relevant to an upgrade decision. The returned
// agent and client versions, and the list of currently available operator images, will
// always be accurate; the chosen version, and the flag indicating development
// mode, may remain blank until uploadTools or validate is called.
func initCAASVersions(
	controllerCfg controller.Config, desiredVersion, agentVersion version.Number, filterOnPrior bool,
) (*upgradeContext, error) {
	if desiredVersion == agentVersion {
		return nil, errUpToDate
	}

	filterVersion := jujuversion.Current
	if desiredVersion != version.Zero {
		filterVersion = desiredVersion
	} else if filterOnPrior {
		filterVersion.Major--
	}
	logger.Debugf("searching for agent images with major: %d", filterVersion.Major)
	imagePath := podcfg.GetJujuOCIImagePath(controllerCfg, version.Zero)
	availableTags, err := docker.ListOperatorImages(imagePath)
	if err != nil {
		return nil, err
	}
	logger.Debugf("found available tags: %v", availableTags)
	var matchingTags tools.Versions
	for _, t := range availableTags {
		vers := t.AgentVersion()
		if filterVersion.Major != -1 && vers.Major != filterVersion.Major {
			continue
		}
		matchingTags = append(matchingTags, t)
	}

	logger.Debugf("found matching tags: %v", matchingTags)
	if len(matchingTags) == 0 {
		// No images found, so if we are not asking for a major upgrade,
		// pretend there is no more recent version available.
		if desiredVersion == version.Zero && agentVersion.Major == filterVersion.Major {
			return nil, errUpToDate
		}
		return nil, err
	}
	return &upgradeContext{
		agent:          agentVersion,
		client:         jujuversion.Current,
		chosen:         desiredVersion,
		packagedAgents: matchingTags,
	}, nil
}

func (c *upgradeControllerCommand) upgradeIAASController(ctx *cmd.Context) error {
	jcmd := &upgradeJujuCommand{baseUpgradeCommand: baseUpgradeCommand{
		upgradeMessage: "upgrade to this version by running\n    juju upgrade-controller",
	}}
	jcmd.SetClientStore(c.ClientStore())
	wrapped := modelcmd.Wrap(jcmd)
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	fullControllerModelName := modelcmd.JoinModelName(controllerName,
		bootstrap.ControllerModelName)
	args := append(c.rawArgs, "-m", fullControllerModelName)
	if c.vers != "" {
		args = append(args, "--agent-version", c.vers)
	}
	if c.AgentStream != "" {
		args = append(args, "--agent-stream", c.AgentStream)
	}
	if c.BuildAgent {
		args = append(args, "--build-agent")
	}
	if c.DryRun {
		args = append(args, "--dry-run")
	}
	if c.IgnoreAgentVersions {
		args = append(args, "--ignore-agent-versions")
	}
	if c.ResetPrevious {
		args = append(args, "--reset-previous-upgrade")
	}
	if c.AssumeYes {
		args = append(args, "--yes")
	}
	code := cmd.Main(wrapped, ctx, args)
	if code == 0 {
		return nil
	}
	return cmd.ErrSilent
}
