// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/jujuclient"
)

var usageRemoveCloudSummary = `
Removes a user-defined cloud from Juju.`[1:]

var usageRemoveCloudDetails = `
Remove a named, user-defined cloud from Juju's internal cache.

If the multi-cloud feature flag is enabled, the cloud is removed from a controller.
The current controller is used unless the --controller option is specified.
If --local is specified, Juju removes the cloud from internal cache.

Examples:
    juju remove-cloud mycloud
    juju remove-cloud mycloud --local
    juju remove-cloud mycloud --controller mycontroller

See also:
    add-cloud
    list-clouds`

type removeCloudCommand struct {
	modelcmd.OptionalControllerCommand

	// Cloud is the name fo the cloud to remove.
	Cloud string

	// Used when querying a controller for its cloud details
	controllerName     string
	store              jujuclient.ClientStore
	removeCloudAPIFunc func(controllerName string) (removeCloudAPI, error)
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
			Store:       store,
			EnabledFlag: feature.MultiCloud,
		},
		store: store,
	}
	c.removeCloudAPIFunc = c.cloudAPI
	return modelcmd.WrapBase(c)
}

func (c *removeCloudCommand) cloudAPI(controllerName string) (removeCloudAPI, error) {
	root, err := c.NewAPIRoot(c.store, controllerName, "")
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
	if len(args) < 1 {
		return errors.New("Usage: juju remove-cloud <cloud name>")
	}
	c.Cloud = args[0]
	c.ControllerName, err = c.ControllerNameFromArg()
	if err != nil && errors.Cause(err) != modelcmd.ErrNoControllersDefined {
		return errors.Trace(err)
	}
	return cmd.CheckEmpty(args[1:])
}

func (c *removeCloudCommand) Run(ctxt *cmd.Context) error {
	if c.ControllerName == "" {
		if c.ControllerName == "" && !c.Local {
			return errors.Errorf(
				"There are no controllers running.\nTo remove cloud %q from the local cache, use the --local option.", c.Cloud)
		}
		return c.removeLocalCloud(ctxt)
	}
	return c.removeControllerCloud(ctxt)
}

func (c *removeCloudCommand) removeLocalCloud(ctxt *cmd.Context) error {
	personalClouds, err := cloud.PersonalCloudMetadata()
	if err != nil {
		return err
	}
	if _, ok := personalClouds[c.Cloud]; !ok {
		ctxt.Infof("No personal cloud called %q exists", c.Cloud)
		return nil
	}
	delete(personalClouds, c.Cloud)
	if err := cloud.WritePersonalCloudMetadata(personalClouds); err != nil {
		return errors.Trace(err)
	}
	ctxt.Infof("Removed details of personal cloud %q", c.Cloud)
	return nil
}

func (c *removeCloudCommand) removeControllerCloud(ctxt *cmd.Context) error {
	api, err := c.removeCloudAPIFunc(c.ControllerName)
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
