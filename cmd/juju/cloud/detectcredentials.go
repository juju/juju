// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
)

type detectCredentialsCommand struct {
	cmd.CommandBase
	out cmd.Output

	store jujuclient.CredentialUpdater

	// Replace, if true, existing credential information is overwritten.
	Replace bool

	// The cloud for which to load credentials.
	CloudName string
}

var detectCredentialsDoc = `
The autoload-credentials command looks for well known locations for supported clouds and
loads any credentials and default regions found into the Juju credentials store and makes
these available when bootstrapping new controllers.

The resulting credentials may be viewed with juju list-credentials.

The clouds for which credentials may be autoloaded are:

AWS
  Credentials and regions are located in:
    1. On Linux, $HOME/.aws/credentials and $HOME/.aws/config 
    2. Environment variables AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY
    
GCE
  Credentials are located in:
    1. A JSON file whose path is specified by the GOOGLE_APPLICATION_CREDENTIALS environment variable
    2. A JSON file in a knowm location eg on Linux $HOME/.config/gcloud/application_default_credentials.json
    
Openstack (see below)
  Credentials are located in:
    1. On Linux, $HOME/.novarc
    2. Environment variables OS_USERNAME, OS_PASSWORD, OS_TENANT_NAME 
    
Some standard credentials locations may apply for more than one cloud. Fox example, there may be more than one
Openstack cloud defined. In such cases, the cloud name may be specified and only credentials for that cloud are
searched, and when found, applied to that cloud specifically.

Example:
   juju autoload-credentials
   juju autoload-credentials rackspace
   juju autoload-credentials --replace
   
See Also:
   juju list-credentials
   juju add-credentials
`

// NewDetectCredentialsCommand returns a command to add credential information to credentials.yaml.
func NewDetectCredentialsCommand() cmd.Command {
	return &detectCredentialsCommand{
		store: jujuclient.NewFileCredentialsStore(),
	}
}

func (c *detectCredentialsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "autoload-credentials",
		Purpose: "looks for cloud credentials and caches those for use by Juju when bootstrapping",
		Doc:     detectCredentialsDoc,
	}
}

func (c *detectCredentialsCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Replace, "replace", false, "overwrite any existing credential information")
}

func (c *detectCredentialsCommand) Init(args []string) (err error) {
	if len(args) > 1 {
		return errors.New("Usage: juju autoload-credentials [--replace] [<cloud-name>]")
	}
	if len(args) == 1 {
		c.CloudName = args[0]
	}
	return nil
}

func (c *detectCredentialsCommand) Run(ctxt *cmd.Context) error {
	clouds, _, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	if err != nil {
		return err
	}
	personalClouds, err := jujucloud.PersonalCloudMetadata()
	if err != nil {
		return err
	}
	for k, v := range personalClouds {
		clouds[k] = v
	}
	for cloudName, cloud := range clouds {
		if c.CloudName != "" && c.CloudName != cloudName {
			continue
		}
		provider, err := environs.Provider(cloud.Type)
		if err != nil {
			// Should never happen but it will on go 1.2
			// because lxd provider is not built.
			logger.Warningf("cloud %q not available on this platform", cloud.Type)
			continue
		}
		// TODO(wallyworld) - use --replace option
		var creds []environs.LabeledCredential
		if detectCredentials, ok := provider.(environs.ProviderCredentials); ok {
			creds, err = detectCredentials.DetectCredentials()
			if err != nil && !errors.IsNotFound(err) {
				logger.Warningf("could not detect credentials for %q: %v", cloudName, err)
			}
			if errors.IsNotFound(err) {
				continue
			}
			// TODO(wallyworld) - add default credentials and region
			cloudCredential := jujucloud.CloudCredential{
				AuthCredentials: make(map[string]jujucloud.Credential),
			}
			for _, cred := range creds {
				cloudCredential.AuthCredentials[cred.Label] = cred.Credential
			}
			if err := c.store.UpdateCredentials(cloudName, cloudCredential); err != nil {
				return errors.Annotatef(err, "cannot add credentials for cloud %q", cloudName)
			}
		}
	}
	return nil
}
