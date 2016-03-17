// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"io/ioutil"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/jujuclient"
)

type addCredentialCommand struct {
	cmd.CommandBase
	store           jujuclient.CredentialStore
	cloudByNameFunc func(string) (*jujucloud.Cloud, error)

	// Replace, if true, existing credential information is overwritten.
	Replace bool

	// Cloud is the name of the cloud for which we add credentials.
	Cloud string

	// CredentialsFile is the name of the credentials YAML file.
	CredentialsFile string
}

var addCredentialDoc = `
The add-credential command adds or replaces credentials for a given cloud.

The user is required to specify the name of the cloud for which credentials
will be added/replaced, and a YAML file containing credentials.
A sample YAML snippet is:

credentials:
  aws:
    me:
      auth-type: access-key
      access-key: <key>
      secret-key: <secret>


If the any of the named credentials for the cloud already exist, the --replace
option is required to overwite. Note that any default region which may have
been defined is never overwritten.

Example:
   juju add-credential aws -f my-credentials.yaml
   juju add-credential aws -f my-credentials.yaml --replace

See Also:
   juju list-credentials
   juju remove-credential
`

// NewAddCredentialCommand returns a command to add credential information.
func NewAddCredentialCommand() cmd.Command {
	return &addCredentialCommand{
		store:           jujuclient.NewFileCredentialStore(),
		cloudByNameFunc: jujucloud.CloudByName,
	}
}

func (c *addCredentialCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-credential",
		Purpose: "adds or replaces credential information for a specified cloud",
		Doc:     addCredentialDoc,
		Args:    "<cloud-name>",
	}
}

func (c *addCredentialCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Replace, "replace", false, "overwrite any existing cloud information")
	f.StringVar(&c.CredentialsFile, "f", "", "the YAML file containing credentials to add")
}

func (c *addCredentialCommand) Init(args []string) (err error) {
	if len(args) < 1 || c.CredentialsFile == "" {
		return errors.New("Usage: juju add-credential <cloud-name> -f <credentials.yaml>")
	}
	// Check that the supplied cloud is valid.
	c.Cloud = args[0]
	if _, err := c.cloudByNameFunc(c.Cloud); err != nil {
		if errors.IsNotFound(err) {
			return errors.NotValidf("cloud %v", c.Cloud)
		}
		return err
	}
	return cmd.CheckEmpty(args[1:])
}

func (c *addCredentialCommand) Run(ctxt *cmd.Context) error {
	data, err := ioutil.ReadFile(c.CredentialsFile)
	if err != nil {
		return errors.Annotate(err, "reading credentials file")
	}

	specifiedCredentials, err := jujucloud.ParseCredentials(data)
	if err != nil {
		return errors.Annotate(err, "parsing credentials file")
	}
	credentials, ok := specifiedCredentials[c.Cloud]
	if !ok {
		return errors.Errorf("no credentials for cloud %s exist in file %s", c.Cloud, c.CredentialsFile)
	}
	existingCredentials, err := c.store.CredentialForCloud(c.Cloud)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Annotate(err, "reading existing credentials for cloud")
	}
	if errors.IsNotFound(err) {
		existingCredentials = &jujucloud.CloudCredential{
			AuthCredentials: make(map[string]jujucloud.Credential),
		}
	}
	// If there are *any* credentials already for the cloud, we'll ask for the --replace flag.
	if !c.Replace && len(existingCredentials.AuthCredentials) > 0 && len(credentials.AuthCredentials) > 0 {
		return errors.Errorf("credentials for cloud %s already exist; use --replace to overwrite / merge", c.Cloud)
	}
	for name, cred := range credentials.AuthCredentials {
		existingCredentials.AuthCredentials[name] = cred
	}
	err = c.store.UpdateCredential(c.Cloud, *existingCredentials)
	if err != nil {
		return err
	}
	fmt.Fprintf(ctxt.Stdout, "credentials updated for cloud %s\n", c.Cloud)
	return nil
}
