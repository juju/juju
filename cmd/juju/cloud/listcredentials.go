// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/jujuclient"
)

type listCredentialsCommand struct {
	cmd.CommandBase
	out       cmd.Output
	cloudName string
	store     jujuclient.CredentialGetter
}

var listCredentialsDoc = `
The list-credentials command lists the credentials for clouds on which Juju workloads
can be deployed. The credentials listed are those added with the add-credentials
command.

Example:
   # List all credentials.
   juju list-credentials

   # List credentials for the aws cloud only.
   juju list-credentials aws
`

type credentialsMap struct {
	Credentials map[string]jujucloud.CloudCredential `yaml:"credentials" json:"credentials"`
}

// NewListCredentialsCommand returns a command to list cloud credentials.
func NewListCredentialsCommand() cmd.Command {
	return &listCredentialsCommand{
		store: jujuclient.NewFileCredentialStore(),
	}
}

func (c *listCredentialsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-credentials",
		Args:    "[<cloudname>]",
		Purpose: "list credentials available to create a Juju model",
		Doc:     listCredentialsDoc,
	}
}

func (c *listCredentialsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatCredentialsTabular,
	})
}

func (c *listCredentialsCommand) Init(args []string) error {
	cloudName, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return errors.Trace(err)
	}
	c.cloudName = cloudName
	return nil
}

func (c *listCredentialsCommand) Run(ctxt *cmd.Context) error {
	var credentials map[string]jujucloud.CloudCredential
	credentials, err := c.store.AllCredentials()
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if c.cloudName != "" {
		for cloudName := range credentials {
			if cloudName != c.cloudName {
				delete(credentials, cloudName)
			}
		}
	}
	return c.out.Write(ctxt, credentialsMap{credentials})
}

// formatCredentialsTabular returns a tabular summary of cloud information.
func formatCredentialsTabular(value interface{}) ([]byte, error) {
	credentials, ok := value.(credentialsMap)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", credentials, value)
	}

	// For tabular we'll sort alphabetically by cloud, and then by credential name.
	var cloudNames []string
	for name := range credentials.Credentials {
		cloudNames = append(cloudNames, name)
	}
	sort.Strings(cloudNames)

	var out bytes.Buffer
	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 1
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
	p := func(values ...string) {
		text := strings.Join(values, "\t")
		fmt.Fprintln(tw, text)
	}
	p("CLOUD\tCREDENTIALS")
	for _, cloudName := range cloudNames {
		var haveDefault bool
		var credentialNames []string
		credentials := credentials.Credentials[cloudName]
		for credentialName := range credentials.AuthCredentials {
			if credentialName == credentials.DefaultCredential {
				credentialNames = append([]string{credentialName + "*"}, credentialNames...)
				haveDefault = true
			} else {
				credentialNames = append(credentialNames, credentialName)
			}
		}
		if haveDefault {
			sort.Strings(credentialNames[1:])
		} else {
			sort.Strings(credentialNames)
		}
		p(cloudName, strings.Join(credentialNames, ", "))
	}
	tw.Flush()

	return out.Bytes(), nil
}
