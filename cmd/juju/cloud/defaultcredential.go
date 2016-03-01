// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/jujuclient"
)

type setDefaultCredentialCommand struct {
	cmd.CommandBase

	store      jujuclient.CredentialStore
	cloud      string
	credential string
}

var setDefaultCredentialDoc = `
The set-default-credential command sets the default credential for the specified cloud.

Example:
   juju set-default-credential aws my-sekrets
`

// NewSetDefaultCredentialCommand returns a command to set the default credential for a cloud.
func NewSetDefaultCredentialCommand() cmd.Command {
	return &setDefaultCredentialCommand{
		store: jujuclient.NewFileCredentialStore(),
	}
}

func (c *setDefaultCredentialCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set-default-credential",
		Purpose: "sets the default credential for a cloud",
		Doc:     setDefaultCredentialDoc,
		Args:    "<cloud> <credential-name>",
	}
}

func (c *setDefaultCredentialCommand) Init(args []string) (err error) {
	if len(args) < 2 {
		return errors.New("Usage: juju set-default-credential <cloud-name> <credential-name>")
	}
	c.cloud = args[0]
	c.credential = args[1]
	return cmd.CheckEmpty(args[2:])
}

func hasCredential(credential string, credentials map[string]jujucloud.Credential) bool {
	for c := range credentials {
		if c == credential {
			return true
		}
	}
	return false
}

func (c *setDefaultCredentialCommand) Run(ctxt *cmd.Context) error {
	_, err := jujucloud.CloudByName(c.cloud)
	if err != nil {
		return err
	}
	var cred *jujucloud.CloudCredential
	cred, err = c.store.CredentialForCloud(c.cloud)
	if errors.IsNotFound(err) {
		cred = &jujucloud.CloudCredential{}
	} else if err != nil {
		return err
	}
	if !hasCredential(c.credential, cred.AuthCredentials) {
		return errors.NotValidf("credential %q for cloud %s", c.credential, c.cloud)
	}

	cred.DefaultCredential = c.credential
	if err := c.store.UpdateCredential(c.cloud, *cred); err != nil {
		return err
	}
	ctxt.Infof("Default credential for %s set to %q.", c.cloud, c.credential)
	return nil
}
