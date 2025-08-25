// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	cloudapi "github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

var usageRemoveCloudSummary = `
Removes a cloud from Juju.`[1:]

var usageRemoveCloudDetails = `
Remove a cloud from Juju.

If ` + "`--controller`" + ` is used, also remove the cloud from the specified controller,
if it is not in use.

If ` + "`--client`" + ` is specified, Juju removes the cloud from this client.

`

const usageRemoveCloudExamples = `
    juju remove-cloud mycloud
    juju remove-cloud mycloud --client
    juju remove-cloud mycloud --controller mycontroller
`

type removeCloudCommand struct {
	modelcmd.OptionalControllerCommand

	// Cloud is the name fo the cloud to remove.
	Cloud string

	// Used when querying a controller for its cloud details
	removeCloudAPIFunc func() (RemoveCloudAPI, error)

	// targetController holds a controller name when removing
	// a cloud from a controller managed by JAAS.
	targetController string
}

type RemoveCloudAPI interface {
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

func (c *removeCloudCommand) cloudAPI() (RemoveCloudAPI, error) {
	root, err := c.NewAPIRoot(c.Store, c.ControllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cloudapi.NewClient(root), nil
}

func (c *removeCloudCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-cloud",
		Args:     "<cloud name>",
		Purpose:  usageRemoveCloudSummary,
		Doc:      usageRemoveCloudDetails,
		Examples: usageRemoveCloudExamples,
		SeeAlso: []string{
			"add-cloud",
			"update-cloud",
			"clouds",
		},
	})
}

// SetFlags initializes the flags supported by the command.
func (c *removeCloudCommand) SetFlags(f *gnuflag.FlagSet) {
	c.OptionalControllerCommand.SetFlags(f)
	f.StringVar(&c.targetController, "target-controller", "", "The name of a JAAS managed controller to remove a cloud from")
}

func (c *removeCloudCommand) Init(args []string) (err error) {
	if c.targetController != "" {
		return cmd.ErrCommandMissing
	}

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
	if err := c.MaybePrompt(ctxt, fmt.Sprintf("remove cloud %v from", c.Cloud)); err != nil {
		return errors.Trace(err)
	}
	var returnErr error
	if c.Client {
		if err := c.removeLocalCloud(ctxt); err != nil {
			ctxt.Infof("ERROR %v", err)
			returnErr = cmd.ErrSilent
		}
	}
	if c.ControllerName != "" {
		if err := c.removeControllerCloud(ctxt); err != nil {
			if errors.IsNotFound(err) {
				ctxt.Infof("No cloud called %q exists on controller %q", c.Cloud, c.ControllerName)
			} else {
				ctxt.Infof("ERROR %v", err)
				returnErr = cmd.ErrSilent
			}
		}
	}
	return returnErr
}

func (c *removeCloudCommand) removeLocalCloud(ctxt *cmd.Context) error {
	cloudDetails, err := listLocalCloudDetails(c.Store)
	if err != nil {
		return err
	}
	if _, ok := cloudDetails.personal[c.Cloud]; ok {
		return c.removeLocalPersonalCloud(ctxt)
	}
	if _, ok := cloudDetails.builtin[c.Cloud]; ok {
		ctxt.Infof("Cannot remove built-in cloud %q from client", c.Cloud)
		return nil
	}
	if cloudDetails, ok := cloudDetails.public[c.Cloud]; ok {
		ctxt.Infof("Cannot remove public cloud %q from client", c.Cloud)
		if cloudDetails.CredentialCount != 0 {
			ctxt.Infof("To hide this cloud, remove it's credentials with `juju remove-credential`")
		}
		return nil
	}
	ctxt.Infof("No cloud called %q exists on this client", c.Cloud)
	return nil
}

func (c *removeCloudCommand) removeLocalPersonalCloud(ctxt *cmd.Context) error {
	personalClouds, err := cloud.PersonalCloudMetadata()
	if err != nil {
		return err
	}
	if _, ok := personalClouds[c.Cloud]; !ok {
		ctxt.Infof("No local cloud called %q exists on this client", c.Cloud)
		return nil
	}
	delete(personalClouds, c.Cloud)
	if err := cloud.WritePersonalCloudMetadata(personalClouds); err != nil {
		return errors.Trace(err)
	}
	ctxt.Infof("Removed details of cloud %q from this client", c.Cloud)
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
	ctxt.Infof("Removed details of cloud %q from controller %q", c.Cloud, c.ControllerName)
	return nil
}
