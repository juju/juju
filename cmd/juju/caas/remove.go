// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"context"
	"fmt"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"

	cloudapi "github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

var usageRemoveCAASSummary = `
Removes a k8s cloud from Juju.`[1:]

var usageRemoveCAASDetails = `
Removes the specified k8s cloud from this client.

If --controller is used, also removes the cloud 
from the specified controller (if it is not in use).

Use --client option to update your current client.

`

const usageRemoveCAASExamples = `
    juju remove-k8s myk8scloud
    juju remove-k8s myk8scloud --client
    juju remove-k8s --controller mycontroller myk8scloud
`

// RemoveCloudAPI is implemented by cloudapi.Client.
type RemoveCloudAPI interface {
	RemoveCloud(context.Context, string) error
	Close() error
}

// RemoveCAASCommand is the command that allows you to remove a k8s cloud.
type RemoveCAASCommand struct {
	modelcmd.OptionalControllerCommand

	// cloudName is the name of the caas cloud to remove.
	cloudName string

	cloudMetadataStore CloudMetadataStore
	credentialStoreAPI credentialGetter

	apiFunc func(ctx context.Context) (RemoveCloudAPI, error)
}

// NewRemoveCAASCommand returns a command to add caas information.
func NewRemoveCAASCommand(cloudMetadataStore CloudMetadataStore) cmd.Command {
	store := jujuclient.NewFileClientStore()
	command := &RemoveCAASCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store: store,
		},

		cloudMetadataStore: cloudMetadataStore,
		credentialStoreAPI: store,
	}
	command.apiFunc = func(ctx context.Context) (RemoveCloudAPI, error) {
		root, err := command.NewAPIRoot(ctx, command.Store, command.ControllerName, "")
		if err != nil {
			return nil, errors.Trace(err)
		}
		return cloudapi.NewClient(root), nil
	}
	return modelcmd.WrapBase(command)
}

// Info returns help information about the command.
func (c *RemoveCAASCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-k8s",
		Args:     "<k8s name>",
		Purpose:  usageRemoveCAASSummary,
		Doc:      usageRemoveCAASDetails,
		Examples: usageRemoveCAASExamples,
		SeeAlso: []string{
			"add-k8s",
		},
	})
}

// Init populates the command with the args from the command line.
func (c *RemoveCAASCommand) Init(args []string) (err error) {
	if err := c.OptionalControllerCommand.Init(args); err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.Errorf("missing k8s cloud name.")
	}
	c.cloudName = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Run is defined on the Command interface.
func (c *RemoveCAASCommand) Run(ctx *cmd.Context) error {
	if err := c.MaybePrompt(ctx, fmt.Sprintf("remove k8s cloud %v from ", c.cloudName)); err != nil {
		return errors.Trace(err)
	}

	if c.ControllerName == "" {
		ctx.Infof("There are no controllers running.\nTo remove cloud %q from the current client, use the --client option.", c.cloudName)
	}

	if c.ControllerName != "" && c.Client { // TODO(caas): only do RBAC cleanup for removing from both client and controller to less complexity.
		if err := cleanUpCredentialRBACResources(ctx, c.cloudName, c.cloudMetadataStore, c.credentialStoreAPI); err != nil {
			return errors.Trace(err)
		}
	}

	if c.Client {
		if err := removeCloudFromLocal(c.cloudName, c.cloudMetadataStore); err != nil {
			return errors.Annotatef(err, "cannot remove cloud from current client")
		}

		if err := c.Store.UpdateCredential(c.cloudName, cloud.CloudCredential{}); err != nil {
			return errors.Annotatef(err, "cannot remove credential from current client")
		}
	}
	if c.ControllerName != "" {
		if err := jujuclient.ValidateControllerName(c.ControllerName); err != nil {
			return errors.Trace(err)
		}
		cloudAPI, err := c.apiFunc(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		defer cloudAPI.Close()

		if err := cloudAPI.RemoveCloud(ctx, c.cloudName); err != nil {
			return errors.Annotatef(err, "cannot remove k8s cloud from controller")
		}
	}
	return nil
}

func cleanUpCredentialRBACResources(
	ctx context.Context,
	cloudName string,
	cloudMetadataStore CloudMetadataStore, credentialStoreAPI credentialGetter,
) error {
	personalClouds, err := cloudMetadataStore.PersonalCloudMetadata()
	if err != nil {
		return errors.Trace(err)
	}
	if personalClouds == nil {
		return nil
	}
	pCloud, ok := personalClouds[cloudName]
	if !ok {
		return nil
	}

	cloudCredentials, err := credentialStoreAPI.CredentialForCloud(cloudName)
	if err != nil {
		return errors.Trace(err)
	}
	if cloudCredentials == nil {
		return nil
	}
	for _, credential := range cloudCredentials.AuthCredentials {
		if err := cleanUpCredentialRBAC(ctx, pCloud, credential); err != nil {
			logger.Warningf(context.TODO(), "unable to remove RBAC resources for credential %q", credential.Label)
		}
	}
	return nil
}

func removeCloudFromLocal(cloudName string, cloudMetadataStore CloudMetadataStore) error {
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
