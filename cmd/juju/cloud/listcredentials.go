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
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
)

type listCredentialsCommand struct {
	cmd.CommandBase
	out         cmd.Output
	cloudName   string
	showSecrets bool

	store              jujuclient.CredentialGetter
	personalCloudsFunc func() (map[string]jujucloud.Cloud, error)
	cloudByNameFunc    func(string) (*jujucloud.Cloud, error)
}

var listCredentialsDoc = `
The list-credentials command lists the credentials for clouds on which Juju workloads
can be deployed. The credentials listed are those added with the add-credential command.

Example:
   # List all credentials.
   juju list-credentials

   # List credentials for the aws cloud only.
   juju list-credentials aws
   
   # List detailed credential information including passwords.
   juju list-credentials --format yaml --show-secrets
`

type credentialsMap struct {
	Credentials map[string]jujucloud.CloudCredential `yaml:"credentials" json:"credentials"`
}

// NewListCredentialsCommand returns a command to list cloud credentials.
func NewListCredentialsCommand() cmd.Command {
	return &listCredentialsCommand{
		store:           jujuclient.NewFileCredentialStore(),
		cloudByNameFunc: jujucloud.CloudByName,
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
	f.BoolVar(&c.showSecrets, "show-secrets", false, "show secrets for displayed credentials")
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

func (c *listCredentialsCommand) personalClouds() (map[string]jujucloud.Cloud, error) {
	if c.personalCloudsFunc == nil {
		return jujucloud.PersonalCloudMetadata()
	}
	return c.personalCloudsFunc()
}

// displayCloudName returns the provided cloud name prefixed
// with "local:" if it is a local cloud.
func displayCloudName(cloudName string, personalCloudNames []string) string {
	for _, name := range personalCloudNames {
		if cloudName == name {
			return localPrefix + cloudName
		}
	}
	return cloudName
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

	// Find local cloud names.
	personalClouds, err := c.personalClouds()
	if err != nil {
		return err
	}
	var personalCloudNames []string
	for name := range personalClouds {
		personalCloudNames = append(personalCloudNames, name)
	}

	displayCredentials := make(map[string]jujucloud.CloudCredential)
	for cloudName, cred := range credentials {
		if !c.showSecrets {
			if err := c.removeSecrets(cloudName, &cred); err != nil {
				return errors.Annotatef(err, "removing secrets from credentials for cloud %v", cloudName)
			}
		}
		displayCredentials[displayCloudName(cloudName, personalCloudNames)] = cred
	}
	return c.out.Write(ctxt, credentialsMap{displayCredentials})
}

func (c *listCredentialsCommand) removeSecrets(cloudName string, cloudCred *jujucloud.CloudCredential) error {
	cloud, err := c.cloudByNameFunc(cloudName)
	if err != nil {
		return err
	}
	provider, err := environs.Provider(cloud.Type)
	if err != nil {
		return err
	}
	schemas := provider.CredentialSchemas()
	for name, cred := range cloudCred.AuthCredentials {
		sanitisedCred, err := jujucloud.RemoveSecrets(cred, schemas)
		if err != nil {
			return err
		}
		cloudCred.AuthCredentials[name] = *sanitisedCred
	}
	return nil
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
