// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
)

var usageListCredentialsSummary = `
Lists credentials for a cloud.`[1:]

var usageListCredentialsDetails = `
Credentials are used with `[1:] + "`juju bootstrap`" + `  and ` + "`juju add-model`" + `.
An arbitrary "credential name" is used to represent credentials, which are 
added either via ` + "`juju add-credential` or `juju autoload-credentials`" + `.
Note that there can be multiple sets of credentials and thus multiple 
names.
Actual authentication material is exposed with the '--show-secrets' 
option.
A controller and subsequently created models can be created with a 
different set of credentials but any action taken within the model (e.g.:
` + "`juju deploy`; `juju add-unit`" + `) applies the set used to create the model. 
Recall that when a controller is created a 'default' model is also 
created.
Credentials denoted with an asterisk '*' are currently set as the default
for the given cloud.

Examples:
    juju credentials
    juju credentials aws
    juju credentials --format yaml --show-secrets

See also: 
    add-credential
    remove-credential
    set-default-credential
    autoload-credentials`

type listCredentialsCommand struct {
	cmd.CommandBase
	out         cmd.Output
	cloudName   string
	showSecrets bool

	store              jujuclient.CredentialGetter
	personalCloudsFunc func() (map[string]jujucloud.Cloud, error)
	cloudByNameFunc    func(string) (*jujucloud.Cloud, error)
}

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
		Name:    "credentials",
		Args:    "[<cloud name>]",
		Purpose: usageListCredentialsSummary,
		Doc:     usageListCredentialsDetails,
		Aliases: []string{"list-credentials"},
	}
}

func (c *listCredentialsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.BoolVar(&c.showSecrets, "show-secrets", false, "Show secrets")
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
	cloud, err := common.CloudOrProvider(cloudName, c.cloudByNameFunc)
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

// formatCredentialsTabular writes a tabular summary of cloud information.
func formatCredentialsTabular(writer io.Writer, value interface{}) error {
	credentials, ok := value.(credentialsMap)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", credentials, value)
	}

	// For tabular we'll sort alphabetically by cloud, and then by credential name.
	var cloudNames []string
	for name := range credentials.Credentials {
		cloudNames = append(cloudNames, name)
	}
	sort.Strings(cloudNames)

	tw := output.TabWriter(writer)
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

	return nil
}
