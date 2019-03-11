// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	cloudapi "github.com/juju/juju/api/cloud"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

type updateCloudCommand struct {
	modelcmd.CommandBase

	cloudMetadataStore CloudMetadataStore

	// Update action to actually perform
	commandAction func(*cmd.Context) error

	// Cloud is the name of the cloud to update
	Cloud string

	// CloudFile is the name of the cloud YAML file
	CloudFile string

	// Used when updating controllers' cloud details
	controllerName     string
	store              jujuclient.ClientStore
	updateCloudAPIFunc func(controllerName string) (updateCloudAPI, error)
}

var updateCloudDoc = `
Update cloud information either locally or on the controller.

Updating the local cache requires a <cloud name> and a yaml file containing the
cloud details.

To update a cloud on the controller you can provide just the <cloud name> which
will use the cloud defined in the local cache or you can provide a cloud
definition yaml file from which to retrieve the cloud details.

When <cloud definition file> is provided with <cloud name> but --controller is
not specified, Juju stores that definition in its internal cache directly after
validating the contents.

Examples:

    juju update-cloud mymaas -f path/to/maas.yaml
    juju update-cloud mymaas -f path/to/maas.yaml --controller mycontroller
    juju update-cloud mymaas --controller mycontroller

See also:
    add-cloud
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
	c := &updateCloudCommand{
		cloudMetadataStore: cloudMetadataStore,
		store:              jujuclient.NewFileClientStore(),
	}
	c.updateCloudAPIFunc = c.updateCloudAPI

	return modelcmd.WrapBase(c)
}

func (c *updateCloudCommand) updateCloudAPI(controllerName string) (updateCloudAPI, error) {
	root, err := c.NewAPIRoot(c.store, controllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cloudapi.NewClient(root), nil
}

// Init populates the command with the args from the command line.
func (c *updateCloudCommand) Init(args []string) error {
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

	// Condense arguments into an action,
	c.commandAction = c.updateLocalCacheFromFile
	if c.controllerName != "" {
		if c.CloudFile != "" && c.Cloud != "" {
			c.commandAction = c.updateControllerFromFile
		} else if c.Cloud != "" {
			c.commandAction = c.updateControllerCacheFromLocalCache
		} else {
			return errors.BadRequestf("cloud name and/or cloud definition file required")
		}
	} else if c.CloudFile == "" {
		return errors.BadRequestf("cloud definition file or controller name required")
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
	c.CommandBase.SetFlags(f)
	f.StringVar(&c.CloudFile, "f", "", "The path to a cloud definition file")
	f.StringVar(&c.controllerName, "c", "", "Controller to operate in")
	f.StringVar(&c.controllerName, "controller", "", "")
}

func (c *updateCloudCommand) Run(ctxt *cmd.Context) error {
	return c.commandAction(ctxt)
}

func (c *updateCloudCommand) updateLocalCacheFromFile(ctxt *cmd.Context) error {
	r := cloudFileReader{
		cloudMetadataStore: c.cloudMetadataStore,
	}
	newCloud, err := r.readCloudFromFile(c.Cloud, c.CloudFile, ctxt, true)
	if err != nil {
		return errors.Trace(err)
	}
	return addLocalCloud(c.cloudMetadataStore, *newCloud)
}

func (c *updateCloudCommand) updateControllerFromFile(ctxt *cmd.Context) error {
	r := cloudFileReader{
		cloudMetadataStore: c.cloudMetadataStore,
	}
	newCloud, err := r.readCloudFromFile(c.Cloud, c.CloudFile, ctxt, true)
	if err != nil {
		return errors.Trace(err)
	}
	return c.updateController(newCloud)
}

func (c *updateCloudCommand) updateControllerCacheFromLocalCache(ctxt *cmd.Context) error {
	newCloud, err := cloudFromLocal(c.Cloud)
	if err != nil {
		return errors.Trace(err)
	}
	return c.updateController(newCloud)
}

func (c updateCloudCommand) updateController(cloud *jujucloud.Cloud) error {
	api, err := c.updateCloudAPIFunc(c.controllerName)
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()
	return api.UpdateCloud(*cloud)
}
