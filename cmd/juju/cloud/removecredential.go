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

var removeCredentialDoc = `
The remove-credential command removes a named credential for the specified cloud.

Example:
   juju remove-credential aws my-credential

See Also:
   juju list-credentials
   juju add-credential   
`

// NewremoveCredentialCommand returns a command to remove a named credential for a cloud.
func NewRemoveCredentialCommand() cmd.Command {
	return &removeCredentialCommand{
		store: jujuclient.NewFileCredentialStore(),
	}
}

func (c *removeCredentialCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-credential",
		Purpose: "removes a credential for a cloud",
		Doc:     removeCredentialDoc,
		Args:    "<cloud> <credential-name>",
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
	ctxt.Infof("Credential %q for cloud %q has been deleted.", c.credential, c.credential)
	return nil
}
