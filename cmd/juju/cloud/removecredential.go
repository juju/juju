// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v3"

	apicloud "github.com/juju/juju/api/cloud"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

type removeCredentialCommand struct {
	modelcmd.OptionalControllerCommand

	cloud      string
	credential string

	cloudByNameFunc func(string) (*jujucloud.Cloud, error)

	// These attributes are used when removing a controller credential.
	controllerName    string
	remoteCloudFound  bool
	localCloudFound   bool
	credentialAPIFunc func() (RemoveCredentialAPI, error)
}

// RemoveCredentialAPI defines api Cloud facade that can remove a remote credential.
type RemoveCredentialAPI interface {
	// Clouds returns all remote clouds that the currently logged-in user can access.
	Clouds() (map[names.CloudTag]jujucloud.Cloud, error)
	// RevokeCredential removes remote credential.
	RevokeCredential(tag names.CloudCredentialTag) error
	// BestAPIVersion returns current best api version.
	BestAPIVersion() int
	// Close closes api client.
	Close() error
}

var usageRemoveCredentialSummary = `
Removes Juju credentials for a cloud.`[1:]

var usageRemoveCredentialDetails = `
The credentials to be removed are specified by a "credential name".
Credential names, and optionally the corresponding authentication
material, can be listed with `[1:] + "`juju credentials`" + `.

By default, after validating the contents, credentials are removed
from both the current controller and the current client device. 
Use --controller option to remove credentials from a different controller. 
Use --client option to remove credentials from the current client only.

Examples:
    juju remove-credential rackspace credential_name
    juju remove-credential rackspace credential_name --client
    juju remove-credential rackspace credential_name -c another_controller

See also: 
    credentials
    add-credential
    default-credential
    autoload-credentials`

// NewRemoveCredentialCommand returns a command to remove a named credential for a cloud.
func NewRemoveCredentialCommand() cmd.Command {
	store := jujuclient.NewFileClientStore()
	c := &removeCredentialCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store: store,
		},
		cloudByNameFunc: jujucloud.CloudByName,
	}
	c.credentialAPIFunc = c.credentialsAPI
	return modelcmd.WrapBase(c)
}

func (c *removeCredentialCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove-credential",
		Args:    "<cloud name> <credential name>",
		Purpose: usageRemoveCredentialSummary,
		Doc:     usageRemoveCredentialDetails,
	})
}

func (c *removeCredentialCommand) credentialsAPI() (RemoveCredentialAPI, error) {
	root, err := c.NewAPIRoot(c.Store, c.ControllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apicloud.NewClient(root), nil
}

func (c *removeCredentialCommand) Init(args []string) (err error) {
	if err := c.OptionalControllerCommand.Init(args); err != nil {
		return err
	}
	if len(args) < 2 {
		return errors.New("Usage: juju remove-credential <cloud-name> <credential-name>")
	}
	c.cloud = args[0]
	c.credential = args[1]
	c.ControllerName, err = c.ControllerNameFromArg()
	if err != nil && errors.Cause(err) != modelcmd.ErrNoControllersDefined {
		return errors.Trace(err)
	}
	if c.ControllerName == "" {
		// No controller was specified explicitly and we did not detect a current controller,
		// this operation should be local only.
		c.ClientOnly = true
	}
	return cmd.CheckEmpty(args[2:])
}

func (c *removeCredentialCommand) SetFlags(f *gnuflag.FlagSet) {
	c.OptionalControllerCommand.SetFlags(f)
}

func (c *removeCredentialCommand) Run(ctxt *cmd.Context) error {
	var client RemoveCredentialAPI
	if !c.ClientOnly {
		var err error
		client, err = c.credentialAPIFunc()
		if err != nil {
			return err
		}
		defer client.Close()
	}

	c.checkCloud(ctxt, client)
	if !c.remoteCloudFound && !c.localCloudFound {
		ctxt.Infof("To view all available clouds, use 'juju clouds'.\nTo add new cloud, use 'juju add-cloud'.")
		return cmd.ErrSilent
	}
	if err := c.removeFromLocal(ctxt); err != nil {
		return err
	}
	if !c.ClientOnly {
		return c.removeFromController(ctxt, client)
	}
	return nil
}

func (c *removeCredentialCommand) checkCloud(ctxt *cmd.Context, client RemoveCredentialAPI) {
	if !c.ClientOnly {
		if err := c.maybeRemoteCloud(ctxt, client); err != nil {
			if !errors.IsNotFound(err) {
				logger.Errorf("%v", err)
			}
			ctxt.Infof("Cloud %q is not found on the controller, looking for it locally on this client.", c.cloud)
		}
	}
	if err := c.maybeLocalCloud(ctxt); err != nil {
		if !errors.IsNotFound(err) {
			logger.Errorf("%v", err)
		}
		ctxt.Infof("Cloud %q is not found locally on this client.", c.cloud)
	}
}

func (c *removeCredentialCommand) maybeLocalCloud(ctxt *cmd.Context) error {
	if _, err := common.CloudOrProvider(c.cloud, c.cloudByNameFunc); err != nil {
		return err
	}
	ctxt.Infof("Found  local cloud %q on this client.", c.cloud)
	c.localCloudFound = true
	return nil
}

func (c *removeCredentialCommand) maybeRemoteCloud(ctxt *cmd.Context, client RemoveCredentialAPI) error {
	// Get user clouds from the controller
	remoteUserClouds, err := client.Clouds()
	if err != nil {
		return err
	}
	if _, ok := remoteUserClouds[names.NewCloudTag(c.cloud)]; ok {
		ctxt.Infof("Found  remote cloud %q from the controller.", c.cloud)
		c.remoteCloudFound = true
		return nil
	}
	return errors.NotFoundf("remote cloud %q", c.cloud)
}

func (c *removeCredentialCommand) removeFromController(ctxt *cmd.Context, client RemoveCredentialAPI) error {
	if !c.remoteCloudFound {
		ctxt.Infof("No stored credentials exist remotely since cloud %q is not found on the controller %q.", c.cloud, c.ControllerName)
		return cmd.ErrSilent
	}
	accountDetails, err := c.Store.AccountDetails(c.ControllerName)
	if err != nil {
		return err
	}
	id := fmt.Sprintf("%s/%s/%s", c.cloud, accountDetails.User, c.credential)
	if !names.IsValidCloudCredential(id) {
		ctxt.Warningf("Could not remove controller credential %v for user %v on cloud %v: %v", c.credential, accountDetails.User, c.cloud, errors.NotValidf("cloud credential ID %q", id))
		return cmd.ErrSilent
	}
	if err := client.RevokeCredential(names.NewCloudCredentialTag(id)); err != nil {
		return errors.Annotate(err, "could not remove remote credential")
	}
	ctxt.Infof("Credential %q removed from the controller %q.", c.credential, c.ControllerName)
	return nil
}

func (c *removeCredentialCommand) removeFromLocal(ctxt *cmd.Context) error {
	if !c.localCloudFound {
		ctxt.Infof("No credentials exist on this client since cloud %q is not found locally.", c.cloud)
		return nil
	}
	cred, err := c.Store.CredentialForCloud(c.cloud)
	if errors.IsNotFound(err) {
		ctxt.Infof("No locally stored credentials exist for cloud %q.", c.cloud)
		return nil
	} else if err != nil {
		return err
	}
	if _, ok := cred.AuthCredentials[c.credential]; !ok {
		ctxt.Infof("No credential called %q exists for cloud %q on this client", c.credential, c.cloud)
		return nil
	}
	delete(cred.AuthCredentials, c.credential)
	if err := c.Store.UpdateCredential(c.cloud, *cred); err != nil {
		return errors.Annotate(err, "could not remove credential from this client")
	}
	ctxt.Infof("Credential %q for cloud %q has been deleted from this client.", c.credential, c.cloud)
	return nil
}
