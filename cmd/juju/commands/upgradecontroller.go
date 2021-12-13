// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/jujuclient"
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
	command := &upgradeControllerCommand{}
	return modelcmd.WrapController(command, options...)
}

// upgradeControllerCommand upgrades the controller agents in a juju installation.
type upgradeControllerCommand struct {
	modelcmd.ControllerCommandBase
	baseUpgradeCommand

	jujuClientAPI ClientAPI
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
	fullControllerModelName := modelcmd.JoinModelName(controllerName, controllerModel)
	if err != nil {
		return errors.Trace(err)
	}
	return c.upgradeController(ctx, fullControllerModelName)
}

func (c *upgradeControllerCommand) upgradeController(ctx *cmd.Context, controllerModel string) error {
	jcmd := &upgradeJujuCommand{
		baseUpgradeCommand: baseUpgradeCommand{
			upgradeMessage: "upgrade to this version by running\n    juju upgrade-controller",
		},
		jujuClientAPI: c.jujuClientAPI,
	}
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
	args = append(args, "--timeout", c.timeout.String())
	code := cmd.Main(wrapped, ctx, args)
	if code == 0 {
		return nil
	}
	return cmd.ErrSilent
}
