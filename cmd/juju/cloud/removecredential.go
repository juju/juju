// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	apicloud "github.com/juju/juju/api/client/cloud"
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

	// Force determines whether the remove will be forced on the controller side.
	Force bool
}

// RemoveCredentialAPI defines api Cloud facade that can remove a remote credential.
type RemoveCredentialAPI interface {
	// Clouds returns all remote clouds that the currently logged-in user can access.
	Clouds() (map[names.CloudTag]jujucloud.Cloud, error)
	// RevokeCredential removes remote credential.
	RevokeCredential(tag names.CloudCredentialTag, force bool) error
	// Close closes api client.
	Close() error
}

var usageRemoveCredentialSummary = `
Removes Juju credentials for a cloud.`[1:]

var usageRemoveCredentialDetails = `
The credential to be removed is specified by a credential name.
Credential names, and optionally the corresponding authentication
material, can be listed with `[1:] + "`juju credentials`" + `.

Use the ` + "`--controller`" + ` option to remove credentials from a controller.

When removing cloud credential from a controller, Juju performs additional
checks to ensure that there are no models using this credential.
Occasionally, these check may not be desired by the user and can be by-passed using ` + "`--force`" + `.
If force remove was performed and some models were still using the credential, these models
will be left with unreachable machines.
Consequently, it is not recommended as a default remove action.
Models with unreachable machines are most commonly fixed by using another cloud credential,
see ` + "`juju set-credential`" + ` for more information.


Use the ` + "`--client`" + ` option to remove credentials from the current client.

`

const usageRemoveCredentialExamples = `
    juju remove-credential google credential_name
    juju remove-credential google credential_name --client
    juju remove-credential google credential_name -c mycontroller
    juju remove-credential google credential_name -c mycontroller --force

`

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
		Name:     "remove-credential",
		Args:     "<cloud name> <credential name>",
		Purpose:  usageRemoveCredentialSummary,
		Doc:      usageRemoveCredentialDetails,
		Examples: usageRemoveCredentialExamples,
		SeeAlso: []string{
			"add-credential",
			"autoload-credentials",
			"credentials",
			"default-credential",
			"set-credential",
			"update-credential",
		},
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
	f.BoolVar(&c.Force, "force", false, "Force remove controller-side credential, ignore validation errors")
}

func (c *removeCredentialCommand) Run(ctxt *cmd.Context) error {
	if err := c.MaybePrompt(ctxt, fmt.Sprintf("remove credential %q for cloud %q from", c.credential, c.cloud)); err != nil {
		return errors.Trace(err)
	}
	var client RemoveCredentialAPI
	if c.ControllerName != "" {
		var err error
		client, err = c.credentialAPIFunc()
		if err != nil {
			return err
		}
		defer client.Close()
	}

	c.checkCloud(ctxt, client)
	if !c.remoteCloudFound && !c.localCloudFound {
		ctxt.Infof("No cloud %q is found.\nTo view all available clouds, use 'juju clouds'.\nTo add new cloud, use 'juju add-cloud'.", c.cloud)
		return cmd.ErrSilent
	}
	var returnErr error
	if c.Client {
		if err := c.removeFromLocal(ctxt); err != nil {
			ctxt.Infof("ERROR %v", err)
			returnErr = cmd.ErrSilent
		}
	}
	if c.ControllerName != "" {
		if err := c.removeFromController(ctxt, client); err != nil {
			ctxt.Infof("ERROR %v", err)
			returnErr = cmd.ErrSilent
		}
	}
	return returnErr
}

func (c *removeCredentialCommand) checkCloud(ctxt *cmd.Context, client RemoveCredentialAPI) {
	if c.ControllerName != "" {
		if err := c.maybeRemoteCloud(ctxt, client); err != nil {
			if !errors.IsNotFound(err) {
				logger.Errorf("%v", err)
			}
		}
	}
	if c.Client {
		if err := c.maybeLocalCloud(ctxt); err != nil {
			if !errors.IsNotFound(err) {
				logger.Errorf("%v", err)
			}
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
	if err := client.RevokeCredential(names.NewCloudCredentialTag(id), c.Force); err != nil {
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
