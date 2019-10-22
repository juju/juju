// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v3"

	cloudapi "github.com/juju/juju/api/cloud"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

type updateCloudCommand struct {
	modelcmd.OptionalControllerCommand

	cloudMetadataStore CloudMetadataStore

	// Update action to actually perform
	commandAction func(*cmd.Context) error

	// Cloud is the name of the cloud to update
	Cloud string

	// CloudFile is the name of the cloud YAML file
	CloudFile string

	// Used when updating controllers' cloud details
	updateCloudAPIFunc func() (updateCloudAPI, error)
}

var updateCloudDoc = `
Update cloud information on this client and/or on a controller.

A cloud can be updated from a file. This requires a <cloud name> and a yaml file
containing the cloud details. 
This method can be used for cloud updates on the client side and on a controller. 

A cloud on the controller can also be updated just by using a name of a cloud
from this client.

If a current controller can be detected, a user will be prompted to confirm 
if specified cloud needs to be updated on it. 
If the prompt is not needed and the cloud is always to be updated on
the current controller if that controller is detected, use --no-prompt option.

Use --controller option to update a cloud on a different controller. 

Use --controller-only option to only update controller copy of the cloud.

Use --client-only to update cloud definition on this client.

Examples:

    juju update-cloud mymaas -f path/to/maas.yaml
    juju update-cloud mymaas -f path/to/maas.yaml --controller mycontroller
    juju update-cloud mymaas --controller mycontroller
    juju update-cloud mymaas --no-prompt --controller-only
    juju update-cloud mymaas --client-only -f path/to/maas.yaml

See also:
    add-cloud
    remove-cloud
    list-clouds
`

type updateCloudAPI interface {
	UpdateCloud(jujucloud.Cloud) error
	Close() error
}

// NewUpdateCloudCommand returns a command to update cloud information.
var NewUpdateCloudCommand = func(cloudMetadataStore CloudMetadataStore) cmd.Command {
	return newUpdateCloudCommand(cloudMetadataStore)
}

func newUpdateCloudCommand(cloudMetadataStore CloudMetadataStore) cmd.Command {
	store := jujuclient.NewFileClientStore()
	c := &updateCloudCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store: store,
		},
		cloudMetadataStore: cloudMetadataStore,
	}
	c.updateCloudAPIFunc = c.updateCloudAPI

	return modelcmd.WrapBase(c)
}

func (c *updateCloudCommand) updateCloudAPI() (updateCloudAPI, error) {
	root, err := c.NewAPIRoot(c.Store, c.ControllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cloudapi.NewClient(root), nil
}

// Init populates the command with the args from the command line.
func (c *updateCloudCommand) Init(args []string) error {
	if err := c.OptionalControllerCommand.Init(args); err != nil {
		return err
	}
	if len(args) < 1 {
		return errors.BadRequestf("cloud name required")
	}

	c.Cloud = args[0]
	if ok := names.IsValidCloud(c.Cloud); !ok {
		return errors.NotValidf("cloud name %q", c.Cloud)
	}
	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return err
	}
	return nil
}

func (c *updateCloudCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "update-cloud",
		Args:    "<cloud name>",
		Purpose: "Updates cloud information available to Juju.",
		Doc:     updateCloudDoc,
	})
}

func (c *updateCloudCommand) SetFlags(f *gnuflag.FlagSet) {
	c.OptionalControllerCommand.SetFlags(f)
	f.StringVar(&c.CloudFile, "f", "", "The path to a cloud definition file")
}

func (c *updateCloudCommand) Run(ctxt *cmd.Context) error {
	if c.BothClientAndController || c.ControllerOnly {
		if c.ControllerName == "" {
			// The user may have specified the controller via a --controller option.
			// If not, let's see if there is a current controller that can be detected.
			var err error
			c.ControllerName, err = c.MaybePromptCurrentController(ctxt, fmt.Sprintf("update cloud %q on", c.Cloud))
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	if c.ControllerName == "" && !c.ClientOnly {
		ctxt.Infof("To update cloud %q on this client, use the --client-only option.", c.Cloud)
	}
	var returnErr error
	runAction := func() {
		if err := c.commandAction(ctxt); err != nil {
			ctxt.Infof("%v", err)
			returnErr = cmd.ErrSilent
		}
	}
	if c.BothClientAndController || c.ClientOnly {
		if c.CloudFile == "" {
			ctxt.Infof("To update cloud %q on this client, a cloud definition file is required.", c.Cloud)
			returnErr = cmd.ErrSilent
		} else {
			c.commandAction = c.updateLocalCacheFromFile
			runAction()
		}
	}
	if c.BothClientAndController || c.ControllerOnly {
		if c.ControllerName != "" {
			if c.CloudFile != "" {
				logger.Infof("Updating cloud %q on controller %q from a file.", c.Cloud, c.ControllerName)
				c.commandAction = c.updateControllerFromFile
				runAction()
			} else {
				logger.Infof("Updating cloud %q on controller %q from a cloud %q on this client.", c.Cloud, c.ControllerName, c.Cloud)
				c.commandAction = c.updateControllerCacheFromLocalCache
				runAction()
			}
		} else {
			return errors.BadRequestf("To update cloud definition on a controller, a controller name is required.")
		}
	}
	return returnErr
}

func (c *updateCloudCommand) updateLocalCacheFromFile(ctxt *cmd.Context) error {
	r := &cloudFileReader{
		cloudMetadataStore: c.cloudMetadataStore,
		cloudName:          c.Cloud,
	}
	newCloud, err := r.readCloudFromFile(c.CloudFile, ctxt)
	if err != nil {
		return errors.Annotatef(err, "could not read cloud definition from file for an update on this client")
	}
	c.Cloud = r.cloudName
	if err := addLocalCloud(c.cloudMetadataStore, *newCloud); err != nil {
		return err
	}
	ctxt.Infof("Cloud %q updated on this client.", c.Cloud)
	return nil
}

func (c *updateCloudCommand) updateControllerFromFile(ctxt *cmd.Context) error {
	r := &cloudFileReader{
		cloudMetadataStore: c.cloudMetadataStore,
		cloudName:          c.Cloud,
	}
	newCloud, err := r.readCloudFromFile(c.CloudFile, ctxt)
	if err != nil {
		return errors.Annotatef(err, "could not read cloud definition from file for an update on a controller")
	}
	c.Cloud = r.cloudName
	return c.updateController(ctxt, newCloud)
}

func (c *updateCloudCommand) updateControllerCacheFromLocalCache(ctxt *cmd.Context) error {
	newCloud, err := cloudFromLocal(c.Store, c.Cloud)
	if err != nil {
		return errors.Trace(err)
	}
	return c.updateController(ctxt, newCloud)
}

func (c updateCloudCommand) updateController(ctxt *cmd.Context, cloud *jujucloud.Cloud) error {
	api, err := c.updateCloudAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()
	err = api.UpdateCloud(*cloud)
	if err != nil {
		return errors.Trace(err)
	}
	ctxt.Infof("Cloud %q updated on controller %q.", c.Cloud, c.ControllerName)
	return nil
}
