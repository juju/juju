// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

var usageRemoveCloudSummary = `
Removes a user-defined cloud from Juju.`[1:]

var usageRemoveCloudDetails = `
Remove a named, user-defined cloud from Juju.

Examples:
    juju remove-cloud mycloud

See also:
    add-cloud
    list-clouds`

type removeCloudCommand struct {
	modelcmd.CommandBase

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
	c := &removeCloudCommand{
		store: jujuclient.NewFileClientStore(),
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

func (c *removeCloudCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.StringVar(&c.controllerName, "c", "", "Controller to operate in")
	f.StringVar(&c.controllerName, "controller", "", "")
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
	return cmd.CheckEmpty(args[1:])
}

func (c *removeCloudCommand) Run(ctxt *cmd.Context) error {
	if c.controllerName == "" {
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
	api, err := c.removeCloudAPIFunc(c.controllerName)
	if err != nil {
		return err
	}
	defer api.Close()
	err = api.RemoveCloud(c.Cloud)
	if err != nil {
		return err
	}
	ctxt.Infof("Cloud %q on controller %q removed", c.Cloud, c.controllerName)
	return nil
}
