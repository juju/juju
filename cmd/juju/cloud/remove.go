// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

var usageRemoveCloudSummary = `
Removes a cloud from Juju.`[1:]

var usageRemoveCloudDetails = `
Remove a cloud from Juju.

If --controller is used, also remove the cloud from the specified controller,
if it is not in use.

If --controller option was not used and the current controller can be detected, 
a user will be prompted to confirm if specified cloud needs to be removed from it. 
If the prompt is not needed and the cloud is always to be removed from
the current controller if that controller is detected, use --no-prompt option.

If you just want to update your controller and not your current client, 
use the --controller-only option.

If --client-only is specified, Juju removes the cloud from this client only.

Examples:
    juju remove-cloud mycloud
    juju remove-cloud mycloud --controller-only --no-prompt
    juju remove-cloud mycloud --client-only
    juju remove-cloud mycloud --controller mycontroller

See also:
    add-cloud
	update-cloud
    list-clouds`

type removeCloudCommand struct {
	modelcmd.OptionalControllerCommand

	// Cloud is the name fo the cloud to remove.
	Cloud string

	// Used when querying a controller for its cloud details
	removeCloudAPIFunc func() (removeCloudAPI, error)
}

type removeCloudAPI interface {
	RemoveCloud(cloud string) error
	Close() error
}

// NewRemoveCloudCommand returns a command to remove cloud information.
func NewRemoveCloudCommand() cmd.Command {
	store := jujuclient.NewFileClientStore()
	c := &removeCloudCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store: store,
		},
	}
	c.removeCloudAPIFunc = c.cloudAPI
	return modelcmd.WrapBase(c)
}

func (c *removeCloudCommand) cloudAPI() (removeCloudAPI, error) {
	root, err := c.NewAPIRoot(c.Store, c.ControllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cloudapi.NewClient(root), nil
}

func (c *removeCloudCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove-cloud",
		Args:    "<cloud name>",
		Purpose: usageRemoveCloudSummary,
		Doc:     usageRemoveCloudDetails,
	})
}

func (c *removeCloudCommand) Init(args []string) (err error) {
	if err := c.OptionalControllerCommand.Init(args); err != nil {
		return err
	}
	if len(args) < 1 {
		return errors.New("Usage: juju remove-cloud <cloud name>")
	}
	c.Cloud = args[0]
	return cmd.CheckEmpty(args[1:])
}

func (c *removeCloudCommand) Run(ctxt *cmd.Context) error {
	if c.BothClientAndController || c.ControllerOnly {
		if c.ControllerName == "" {
			// The user may have specified the controller via a --controller option.
			// If not, let's see if there is a current controller that can be detected.
			var err error
			c.ControllerName, err = c.MaybePromptCurrentController(ctxt, fmt.Sprintf("remove cloud %v from", c.Cloud))
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	if c.ControllerName == "" && !c.ClientOnly {
		ctxt.Infof("To remove cloud %q from this client, use the --client-only option.", c.Cloud)
	}
	var returnErr error
	if c.BothClientAndController || c.ClientOnly {
		if err := c.removeLocalCloud(ctxt); err != nil {
			ctxt.Warningf("%v", err)
			returnErr = cmd.ErrSilent
		}
	}
	if c.BothClientAndController || c.ControllerOnly {
		if c.ControllerName == "" {
			ctxt.Infof("Could not remote a cloud from a controller: no controller was specified.")
			returnErr = cmd.ErrSilent
		} else {
			return c.removeControllerCloud(ctxt)
		}
	}
	return returnErr
}

func (c *removeCloudCommand) removeLocalCloud(ctxt *cmd.Context) error {
	personalClouds, err := cloud.PersonalCloudMetadata()
	if err != nil {
		return err
	}
	if _, ok := personalClouds[c.Cloud]; !ok {
		ctxt.Infof("No cloud called %q exists on this client", c.Cloud)
		return nil
	}
	delete(personalClouds, c.Cloud)
	if err := cloud.WritePersonalCloudMetadata(personalClouds); err != nil {
		return errors.Trace(err)
	}
	ctxt.Infof("Removed details of cloud %q from the client", c.Cloud)
	return nil
}

func (c *removeCloudCommand) removeControllerCloud(ctxt *cmd.Context) error {
	api, err := c.removeCloudAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()
	err = api.RemoveCloud(c.Cloud)
	if err != nil {
		return err
	}
	ctxt.Infof("Cloud %q on controller %q removed", c.Cloud, c.ControllerName)
	return nil
}
