// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/version"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig/podcfg"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	envtools "github.com/juju/juju/environs/tools"
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

	upgradeJujuAPI jujuClientAPI
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

func (c *upgradeControllerCommand) getUpgradeJujuAPI() (jujuClientAPI, error) {
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
	accDetails, err := c.ClientStore().AccountDetails(controllerName)
	if err != nil {
		return errors.Trace(err)
	}

	if !permission.Access(accDetails.LastKnownAccess).EqualOrGreaterControllerAccessThan(permission.SuperuserAccess) {
		return errors.Errorf("upgrade not possible missing"+
			" permissions, current level %q, need: %q", accDetails.LastKnownAccess, permission.SuperuserAccess)
	}
	controllerModel := jujuclient.JoinOwnerModelName(
		names.NewUserTag(environs.AdminUser), bootstrap.ControllerModelName)
	_, err = c.ModelUUIDs([]string{controllerModel})
	if err != nil {
		return errors.Annotatef(err, "cannot get controller model uuid")
	}
	details, err := c.ClientStore().ModelByName(controllerName, controllerModel)
	fullControllerModelName := modelcmd.JoinModelName(controllerName, controllerModel)
	if err != nil {
		return errors.Trace(err)
	}
	if details.ModelType == model.CAAS {
		return c.upgradeCAASController(ctx)
	}
	return c.upgradeIAASController(ctx, fullControllerModelName)
}

// fetchStreamsVersions returns simplestreams agent metadata
// for the specified stream. timeout ensures we don't block forever.
func fetchStreamsVersions(
	client toolsAPI, majorVersion int, stream string, timeout time.Duration,
) (tools.List, error) {
	// Use a go routine so we can timeout.
	result := make(chan tools.List, 1)
	errChan := make(chan error, 1)
	go func() {
		findResult, err := client.FindTools(majorVersion, -1, "", "", stream)
		if err == nil {
			if findResult.Error != nil {
				err = findResult.Error
				// We need to deal with older controllers.
				if strings.HasSuffix(findResult.Error.Message, "not valid") {
					err = errors.NotValidf("finding stream data for this model")
				}
			}
		}
		if err != nil {
			errChan <- err
		} else {
			result <- findResult.List
		}
	}()

	select {
	case <-time.After(timeout):
		return nil, nil
	case err := <-errChan:
		return nil, err
	case resultList := <-result:
		return resultList, nil
	}
}

const caasStreamsTimeout = 20 * time.Second

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

	context, versionsErr := c.initVersions(client, controllerCfg, cfg, currentAgentVersion, warnCompat, caasStreamsTimeout, c.initCAASVersions)
	if versionsErr != nil && !params.IsCodeNotFound(versionsErr) {
		return versionsErr
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
		c.upgradeMessage = "upgrade to this version by running\n    juju upgrade-controller"
		fmt.Fprintf(ctx.Stderr, "%s\n", c.upgradeMessage)
		return nil
	}
	return c.notifyControllerUpgrade(ctx, client, context)
}

// initCAASVersions collects state relevant to an upgrade decision. The returned
// agent and client versions, and the list of currently available operator images, will
// always be accurate; the chosen version, and the flag indicating development
// mode, may remain blank until uploadTools or validate is called.
func (c *baseUpgradeCommand) initCAASVersions(
	controllerCfg controller.Config, majorVersion int, streamsAgents tools.List,
) (tools.Versions, error) {
	logger.Debugf("searching for agent images with major: %d", majorVersion)
	imagePath := podcfg.GetJujuOCIImagePath(controllerCfg, version.Zero, 0)
	availableTags, err := docker.ListOperatorImages(imagePath)
	if err != nil {
		return nil, err
	}
	streamsVersions := set.NewStrings()
	for _, a := range streamsAgents {
		streamsVersions.Add(a.Version.Number.String())
	}
	logger.Debugf("found available tags: %v", availableTags)
	var matchingTags tools.Versions
	for _, t := range availableTags {
		vers := t.AgentVersion()
		if majorVersion != -1 && vers.Major != majorVersion {
			continue
		}
		// Only include a docker image if we've published simple streams
		// metadata for that version.
		vers.Build = 0
		if streamsVersions.Size() > 0 {
			if !streamsVersions.Contains(vers.String()) {
				continue
			}
		} else {
			// Fallback for when we can't query the streams versions.
			// Ignore tagged (non-release) versions if agent stream is released.
			if (c.AgentStream == "" || c.AgentStream == envtools.ReleasedStream) && vers.Tag != "" {
				continue
			}
		}
		matchingTags = append(matchingTags, t)
	}
	return matchingTags, nil
}

func (c *upgradeControllerCommand) upgradeIAASController(ctx *cmd.Context, controllerModel string) error {
	jcmd := &upgradeJujuCommand{baseUpgradeCommand: baseUpgradeCommand{
		upgradeMessage: "upgrade to this version by running\n    juju upgrade-controller",
	}}
	jcmd.SetClientStore(c.ClientStore())
	wrapped := modelcmd.Wrap(jcmd)
	args := append(c.rawArgs, "-m", controllerModel)
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
