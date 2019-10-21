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
The credential to be removed is specified by a "credential name".
Credential names, and optionally the corresponding authentication
material, can be listed with `[1:] + "`juju credentials`" + `.

By default, after validating the contents, a credential is removed
from both the current controller and the current client device. 

If a current controller can be detected, a user will be prompted to confirm 
if specified credential needs to be removed from it. 
If the prompt is not needed and the credential is always to be removed from
the current controller if that controller is detected, use --no-prompt option.

Use --controller option to remove credentials from a different controller. 

Use --controller-only option to remove credentials from a controller only. 

Use --client-only option to remove credentials from the current client only.

Examples:
    juju remove-credential rackspace credential_name
    juju remove-credential rackspace credential_name --no-prompt --controller-only
    juju remove-credential rackspace credential_name --client-only
    juju remove-credential rackspace credential_name -c another_controller

See also: 
    credentials
    add-credential
    update-credential
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
	return cmd.CheckEmpty(args[2:])
}

func (c *removeCredentialCommand) SetFlags(f *gnuflag.FlagSet) {
	c.OptionalControllerCommand.SetFlags(f)
}

func (c *removeCredentialCommand) Run(ctxt *cmd.Context) error {
	if c.BothClientAndController || c.ControllerOnly {
		if c.ControllerName == "" {
			// The user may have specified the controller via a --controller option.
			// If not, let's see if there is a current controller that can be detected.
			var err error
			c.ControllerName, err = c.MaybePromptCurrentController(ctxt, fmt.Sprintf("remove credential %q for cloud %q from", c.credential, c.cloud))
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	if c.ControllerName == "" && !c.ClientOnly {
		ctxt.Infof("To remove credential %q for cloud %q from this client, use the --client-only option.", c.credential, c.cloud)
	}
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
	var returnErr error
	if c.BothClientAndController || c.ClientOnly {
		if err := c.removeFromLocal(ctxt); err != nil {
			ctxt.Warningf("%v", err)
			returnErr = cmd.ErrSilent
		}
	}
	if c.BothClientAndController || c.ControllerOnly {
		if c.ControllerName != "" {
			if err := c.removeFromController(ctxt, client); err != nil {
				ctxt.Warningf("%v", err)
				returnErr = cmd.ErrSilent
			}
		} else {
			ctxt.Infof("Could not remove credential %q for cloud %q from any controllers: no controller specified.", c.credential, c.cloud)
		}
	}
	return returnErr
}

func (c *removeCredentialCommand) checkCloud(ctxt *cmd.Context, client RemoveCredentialAPI) {
	if c.BothClientAndController || c.ControllerOnly {
		if c.ControllerName != "" {
			if err := c.maybeRemoteCloud(ctxt, client); err != nil {
				if !errors.IsNotFound(err) {
					logger.Errorf("%v", err)
				}
				ctxt.Infof("Cloud %q is not found on controller %q.", c.cloud, c.ControllerName)
			}
		}
	}
	if c.BothClientAndController || c.ClientOnly {
		if err := c.maybeLocalCloud(ctxt); err != nil {
			if !errors.IsNotFound(err) {
				logger.Errorf("%v", err)
			}
			ctxt.Infof("Cloud %q is not found on this client.", c.cloud)
		}
	}
}

func (c *removeCredentialCommand) maybeLocalCloud(ctxt *cmd.Context) error {
	if _, err := common.CloudOrProvider(c.cloud, c.cloudByNameFunc); err != nil {
		return err
	}
	ctxt.Infof("Found local cloud %q on this client.", c.cloud)
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
		ctxt.Infof("Found remote cloud %q from the controller.", c.cloud)
		c.remoteCloudFound = true
		return nil
	}
	return errors.NotFoundf("remote cloud %q", c.cloud)
}

func (c *removeCredentialCommand) removeFromController(ctxt *cmd.Context, client RemoveCredentialAPI) error {
	if !c.remoteCloudFound {
		ctxt.Infof("No stored credentials exist since cloud %q is not found on the controller %q.", c.cloud, c.ControllerName)
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
	ctxt.Infof("Credential %q for cloud %q removed from the controller %q.", c.credential, c.cloud, c.ControllerName)
	return nil
}

func (c *removeCredentialCommand) removeFromLocal(ctxt *cmd.Context) error {
	if !c.localCloudFound {
		ctxt.Infof("No credentials exist on this client since cloud %q is not found.", c.cloud)
		return nil
	}
	cred, err := c.Store.CredentialForCloud(c.cloud)
	if errors.IsNotFound(err) {
		ctxt.Infof("No stored credentials exist for cloud %q on this client.", c.cloud)
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
	ctxt.Infof("Credential %q for cloud %q removed from this client.", c.credential, c.cloud)
	return nil
}
