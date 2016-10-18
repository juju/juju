// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/jujuclient"
)

type removeCredentialCommand struct {
	cmd.CommandBase

	store      jujuclient.CredentialStore
	cloud      string
	credential string
}

var usageRemoveCredentialSummary = `
Removes credentials for a cloud.`[1:]

var usageRemoveCredentialDetails = `
The credentials to be removed are specified by a "credential name".
Credential names, and optionally the corresponding authentication
material, can be listed with `[1:] + "`juju credentials`" + `.

Examples:
    juju remove-credential rackspace credential_name

See also: 
    credentials
    add-credential
    set-default-credential
    autoload-credentials`

// NewremoveCredentialCommand returns a command to remove a named credential for a cloud.
func NewRemoveCredentialCommand() cmd.Command {
	return &removeCredentialCommand{
		store: jujuclient.NewFileCredentialStore(),
	}
}

func (c *removeCredentialCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-credential",
		Args:    "<cloud name> <credential name>",
		Purpose: usageRemoveCredentialSummary,
		Doc:     usageRemoveCredentialDetails,
	}
}

func (c *removeCredentialCommand) Init(args []string) (err error) {
	if len(args) < 2 {
		return errors.New("Usage: juju remove-credential <cloud-name> <credential-name>")
	}
	c.cloud = args[0]
	c.credential = args[1]
	return cmd.CheckEmpty(args[2:])
}

func (c *removeCredentialCommand) Run(ctxt *cmd.Context) error {
	cred, err := c.store.CredentialForCloud(c.cloud)
	if errors.IsNotFound(err) {
		ctxt.Infof("No credentials exist for cloud %q", c.cloud)
		return nil
	} else if err != nil {
		return err
	}
	if _, ok := cred.AuthCredentials[c.credential]; !ok {
		ctxt.Infof("No credential called %q exists for cloud %q", c.credential, c.cloud)
		return nil
	}
	delete(cred.AuthCredentials, c.credential)
	if err := c.store.UpdateCredential(c.cloud, *cred); err != nil {
		return err
	}
	ctxt.Infof("Credential %q for cloud %q has been deleted.", c.credential, c.cloud)
	return nil
}
