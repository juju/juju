// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/cloud"

	cloudapi "github.com/juju/juju/api/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

var usageRemoveCAASSummary = `
Removes a k8s endpoint from Juju.`[1:]

var usageRemoveCAASDetails = `
Removes the specified k8s cloud from this client.
If --controller is used, also removes the cloud 
from the specfied controller (if it is not in use).

Examples:
    juju remove-k8s myk8scloud
    juju remove-k8s --controller mycontroller myk8scloud
    
See also:
    add-k8s
`

// RemoveCloudAPI is implemented by cloudapi.Client.
type RemoveCloudAPI interface {
	RemoveCloud(string) error
	Close() error
}

// RemoveCAASCommand is the command that allows you to remove a k8s cloud.
type RemoveCAASCommand struct {
	modelcmd.CommandBase

	// cloudName is the name of the caas cloud to remove.
	cloudName string

	controllerName     string
	store              jujuclient.ClientStore
	cloudMetadataStore CloudMetadataStore
	apiFunc            func() (RemoveCloudAPI, error)
}

// NewRemoveCAASCommand returns a command to add caas information.
func NewRemoveCAASCommand(cloudMetadataStore CloudMetadataStore) cmd.Command {
	cmd := &RemoveCAASCommand{
		cloudMetadataStore: cloudMetadataStore,
		store:              jujuclient.NewFileClientStore(),
	}
	cmd.apiFunc = func() (RemoveCloudAPI, error) {
		root, err := cmd.NewAPIRoot(cmd.store, cmd.controllerName, "")
		if err != nil {
			return nil, errors.Trace(err)
		}
		return cloudapi.NewClient(root), nil
	}
	return modelcmd.WrapBase(cmd)
}

// Info returns help information about the command.
func (c *RemoveCAASCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove-k8s",
		Args:    "<k8s name>",
		Purpose: usageRemoveCAASSummary,
		Doc:     usageRemoveCAASDetails,
	})
}

// SetFlags initializes the flags supported by the command.
func (c *RemoveCAASCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.StringVar(&c.controllerName, "c", "", "Controller to operate in")
	f.StringVar(&c.controllerName, "controller", "", "")
}

// Init populates the command with the args from the command line.
func (c *RemoveCAASCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.Errorf("missing k8s name.")
	}
	c.cloudName = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Run is defined on the Command interface.
func (c *RemoveCAASCommand) Run(ctxt *cmd.Context) error {
	if err := removeCloudFromLocal(c.cloudMetadataStore, c.cloudName); err != nil {
		return errors.Annotatef(err, "cannot remove cloud from local cache")
	}

	if err := c.store.UpdateCredential(c.cloudName, cloud.CloudCredential{}); err != nil {
		return errors.Annotatef(err, "cannot remove credential from local cache")
	}
	if c.controllerName == "" {
		return nil
	}

	cloudAPI, err := c.apiFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer cloudAPI.Close()

	if err := cloudAPI.RemoveCloud(c.cloudName); err != nil {
		return errors.Annotatef(err, "cannot remove k8s cloud from controller")
	}
	return nil
}

func removeCloudFromLocal(cloudMetadataStore CloudMetadataStore, cloudName string) error {
	personalClouds, err := cloudMetadataStore.PersonalCloudMetadata()
	if err != nil {
		return errors.Trace(err)
	}
	if personalClouds == nil {
		return nil
	}
	_, ok := personalClouds[cloudName]
	if !ok {
		return nil
	}
	delete(personalClouds, cloudName)
	return cloudMetadataStore.WritePersonalCloudMetadata(personalClouds)
}
