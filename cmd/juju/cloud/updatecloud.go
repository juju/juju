// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	cloudapi "github.com/juju/juju/api/client/cloud"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

type updateCloudCommand struct {
	modelcmd.OptionalControllerCommand

	cloudMetadataStore CloudMetadataStore

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

Use --controller option to update a cloud on a controller. 

Use --client to update cloud definition on this client.

Examples:

    juju update-cloud mymaas -f path/to/maas.yaml
    juju update-cloud mymaas -f path/to/maas.yaml --controller mycontroller
    juju update-cloud mymaas --controller mycontroller
    juju update-cloud mymaas --client --controller mycontroller
    juju update-cloud mymaas --client -f path/to/maas.yaml

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
	var newCloud *jujucloud.Cloud
	if c.CloudFile != "" {
		r := &CloudFileReader{
			CloudMetadataStore: c.cloudMetadataStore,
			CloudName:          c.Cloud,
		}
		var err error
		if newCloud, err = r.ReadCloudFromFile(c.CloudFile, ctxt); err != nil {
			return errors.Annotatef(err, "could not read cloud definition from provided file")
		}
		c.Cloud = r.CloudName
	}
	if err := c.MaybePrompt(ctxt, fmt.Sprintf("update cloud %q on", c.Cloud)); err != nil {
		return errors.Trace(err)
	}
	var returnErr error
	processErr := func(err error, successMsg string) {
		if err != nil {
			ctxt.Infof("%v", err)
			returnErr = cmd.ErrSilent
			return
		}
		ctxt.Infof("%s", successMsg)
	}
	if c.isPublicCloud(c.Cloud) {
		ctxt.Infof("To ensure this client's copy or any controller copies of public cloud information is up to date with the latest region information, use `juju update-public-clouds`.")
	}
	if c.Client {
		if c.CloudFile == "" {
			ctxt.Infof("To update cloud %q on this client, a cloud definition file is required.", c.Cloud)
			returnErr = cmd.ErrSilent
		} else {
			err := addLocalCloud(c.cloudMetadataStore, *newCloud)
			processErr(err, fmt.Sprintf("Cloud %q updated on this client using provided file.", c.Cloud))
		}
	}
	if c.ControllerName != "" {
		if c.CloudFile != "" {
			err := c.updateController(newCloud)
			processErr(err, fmt.Sprintf("Cloud %q updated on controller %q using provided file.", c.Cloud, c.ControllerName))
		} else {
			err := c.updateControllerCacheFromLocalCache()
			processErr(err, fmt.Sprintf("Cloud %q updated on controller %q using client cloud definition.", c.Cloud, c.ControllerName))
		}
	}
	return returnErr
}

func (c *updateCloudCommand) isPublicCloud(cloudName string) bool {
	all, _ := clientPublicClouds()
	for oneName := range all {
		if oneName == cloudName {
			return true
		}
	}
	return false
}

func (c *updateCloudCommand) updateControllerCacheFromLocalCache() error {
	newCloud, err := cloudFromLocal(c.Store, c.Cloud)
	if err != nil {
		return errors.Trace(err)
	}
	return c.updateController(newCloud)
}

func (c updateCloudCommand) updateController(cloud *jujucloud.Cloud) error {
	api, err := c.updateCloudAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()
	err = api.UpdateCloud(*cloud)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
