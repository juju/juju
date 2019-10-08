// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/jujuclient"
)

var usageRemoveCAASSummary = `
Removes a k8s endpoint from Juju.`[1:]

var usageRemoveCAASDetails = `
Removes the specified k8s cloud from this client.
If --controller is used, also removes the cloud 
from the specified controller (if it is not in use).

If you just want to update your current client and not
a running controller, use the --client option.

Examples:
    juju remove-k8s myk8scloud
    juju remove-k8s myk8scloud --client
    juju remove-k8s --controller mycontroller myk8scloud
    
See also:
    add-k8s
`

// RemoveCloudCredentialAPI is implemented by cloudapi.Client.
type RemoveCloudCredentialAPI interface {
	RemoveCloud(string) error
	Close() error
}

// RemoveCAASCommand is the command that allows you to remove a k8s cloud.
type RemoveCAASCommand struct {
	modelcmd.OptionalControllerCommand

	// cloudName is the name of the caas cloud to remove.
	cloudName string

	cloudMetadataStore CloudMetadataStore
	apiFunc            func() (RemoveCloudCredentialAPI, error)
}

// NewRemoveCAASCommand returns a command to add caas information.
func NewRemoveCAASCommand(cloudMetadataStore CloudMetadataStore) cmd.Command {
	store := jujuclient.NewFileClientStore()
	cmd := &RemoveCAASCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store:       store,
			EnabledFlag: feature.MultiCloud,
		},

		cloudMetadataStore: cloudMetadataStore,
	}
	cmd.apiFunc = func() (RemoveCloudCredentialAPI, error) {
		root, err := cmd.NewAPIRoot(cmd.Store, cmd.ControllerName, "")
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

// Init populates the command with the args from the command line.
func (c *RemoveCAASCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.Errorf("missing k8s name.")
	}
	c.cloudName = args[0]
	c.ControllerName, err = c.ControllerNameFromArg()
	if err != nil && errors.Cause(err) != modelcmd.ErrNoControllersDefined {
		return errors.Trace(err)
	}
	return cmd.CheckEmpty(args[1:])
}

// Run is defined on the Command interface.
func (c *RemoveCAASCommand) Run(ctxt *cmd.Context) error {
	if c.ControllerName == "" && !c.Local {
		return errors.Errorf(
			"There are no controllers running.\nTo remove cloud %q from the current client, use the --client option.", c.cloudName)
	}
	if err := removeCloudFromLocal(c.cloudMetadataStore, c.cloudName); err != nil {
		return errors.Annotatef(err, "cannot remove cloud from current client")
	}

	if err := c.Store.UpdateCredential(c.cloudName, cloud.CloudCredential{}); err != nil {
		return errors.Annotatef(err, "cannot remove credential from current client")
	}
	if c.ControllerName == "" {
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
	// delete RBAC resources.
	_ = clientconfig.DeleteJujuAdminRBACResources(c.cloudName)
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
