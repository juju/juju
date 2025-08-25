// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/jujuclient"
)

var usageSetDefaultCredentialSummary = `
Get, set, or unset the default credential for a cloud on this client.`[1:]

var usageSetDefaultCredentialDetails = `
This command sets a locally stored credential to be used as a default.

Default credentials avoid the need to specify a particular set of
credentials when more than one credential is available on the client for a given cloud.

`

const usageSetDefaultCredentialExamples = `
Set the default credential for the ` + "`google`" + ` cloud:

    juju default-credential google <credential>

View the default credential for the ` + "`google`" + ` cloud:

    juju default-credential google

Unset the default credential for the ` + "`google`" + ` cloud:

    juju default-credential google --reset
`

type setDefaultCredentialCommand struct {
	cmd.CommandBase

	store      jujuclient.CredentialStore
	cloud      string
	credential string
	reset      bool
}

// NewSetDefaultCredentialCommand returns a command to set the default credential for a cloud.
func NewSetDefaultCredentialCommand() cmd.Command {
	return &setDefaultCredentialCommand{
		store: jujuclient.NewFileCredentialStore(),
	}
}

func (c *setDefaultCredentialCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "default-credential",
		Aliases:  []string{"set-default-credentials"},
		Args:     "<cloud name> [<credential name>]",
		Purpose:  usageSetDefaultCredentialSummary,
		Doc:      usageSetDefaultCredentialDetails,
		Examples: usageSetDefaultCredentialExamples,
		SeeAlso: []string{
			"credentials",
			"add-credential",
			"remove-credential",
			"autoload-credentials",
		},
	})
}

func (c *setDefaultCredentialCommand) Init(args []string) (err error) {
	if len(args) < 1 {
		return errors.New("Usage: juju default-credential <cloud-name> [<credential-name>]")
	}
	c.cloud = args[0]
	end := 1
	if len(args) > 1 {
		c.credential = args[1]
		end = 2
	}
	return cmd.CheckEmpty(args[end:])
}

// SetFlags initializes the flags supported by the command.
func (c *setDefaultCredentialCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.reset, "reset", false, "Reset default credential for the cloud")
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
	if _, err := common.CloudOrProvider(c.cloud, jujucloud.CloudByName); err != nil {
		return err
	}
	cred, err := c.store.CredentialForCloud(c.cloud)
	if errors.IsNotFound(err) {
		cred = &jujucloud.CloudCredential{}
	} else if err != nil {
		return err
	}
	if !c.reset && c.credential == "" {
		// We are just reading the value.
		if cred.DefaultCredential != "" {
			ctxt.Infof("Default credential for cloud %q is %q on this client.", c.cloud, cred.DefaultCredential)
			return nil
		}
		ctxt.Infof("Default credential for cloud %q is not set on this client.", c.cloud)
		return nil
	}
	msg := fmt.Sprintf("Default credential for cloud %q is no longer set on this client.", c.cloud)
	if c.credential != "" {
		if !hasCredential(c.credential, cred.AuthCredentials) {
			return errors.NotValidf("credential %q for cloud %s", c.credential, c.cloud)
		}
		msg = fmt.Sprintf("Local credential %q is set to be default for %q for this client.", c.credential, c.cloud)
	}
	cred.DefaultCredential = c.credential
	if err := c.store.UpdateCredential(c.cloud, *cred); err != nil {
		return err
	}
	ctxt.Infof("%s", msg)
	return nil
}
