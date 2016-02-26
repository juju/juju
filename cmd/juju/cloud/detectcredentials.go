// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"fmt"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/utils/set"
)

type detectCredentialsCommand struct {
	cmd.CommandBase
	out cmd.Output

	store jujuclient.CredentialStore

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
  Default region is specified by the CLOUDSDK_COMPUTE_REGION environment variable.  
    
OpenStack (see below)
  Credentials are located in:
    1. On Linux, $HOME/.novarc
    2. Environment variables OS_USERNAME, OS_PASSWORD, OS_TENANT_NAME 
    
Some standard credentials locations may apply for more than one cloud. For example, there may be more than one
OpenStack cloud defined. In such cases, the cloud name may be specified and only credentials for that cloud are
searched, and when found, applied to that cloud specifically.

If the detected credentials contain labeled credential values which already exist, the --replace option
may be used to force the overwrite of any existing values. The --replace option is also used to specify
that any default region value is overwritten.

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
		store: jujuclient.NewFileCredentialStore(),
	}
}

func (c *detectCredentialsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "autoload-credentials",
		Purpose: "looks for cloud credentials and caches those for use by Juju when bootstrapping",
		Args:    "[<cloud-name>]",
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

// TODO(wallyworld) - add prompting as per spec
// unambiguousClouds represents those clouds for which we will detect credentials
// without user disambiguation.
var unambiguousClouds = []string{"aws", "azure", "google", "joyent", "cloudsigma"}

func (c *detectCredentialsCommand) Run(ctxt *cmd.Context) error {
	fmt.Fprintf(ctxt.Stdout, "Looking for cloud and credential information locally...\n")
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
	okClouds := set.NewStrings(unambiguousClouds...)
	for cloudName, cloud := range clouds {
		if c.CloudName != "" && c.CloudName != cloudName {
			continue
		} else if c.CloudName == "" && !okClouds.Contains(cloudName) {
			continue
		}
		provider, err := environs.Provider(cloud.Type)
		if err != nil {
			// Should never happen but it will on go 1.2
			// because lxd provider is not built.
			logger.Warningf("cloud %q not available on this platform", cloud.Type)
			continue
		}
		if detectCredentials, ok := provider.(environs.ProviderCredentials); ok {
			detected, err := detectCredentials.DetectCredentials()
			if err != nil && !errors.IsNotFound(err) {
				logger.Warningf("could not detect credentials for %q: %v", cloudName, err)
				continue
			}
			if errors.IsNotFound(err) || len(detected.AuthCredentials) == 0 {
				continue
			}
			credentials, err := c.store.CredentialForCloud(cloudName)
			if err != nil && !errors.IsNotFound(err) {
				return errors.Trace(err)
			}
			if !c.Replace && err == nil {
				existingCredNames := set.NewStrings()
				for n := range credentials.AuthCredentials {
					existingCredNames.Add(n)
				}
				newCredNames := set.NewStrings()
				for n, cred := range detected.AuthCredentials {
					if cred.AuthType() == jujucloud.EmptyAuthType {
						continue
					}
					newCredNames.Add(credentialLabel(n))
				}
				if !existingCredNames.Intersection(newCredNames).IsEmpty() {
					fmt.Fprintf(ctxt.Stdout, "Detected credentials would overwrite existing credentials.\nUse the --replace option.\n")
					return nil
				}
			}
			for name := range detected.AuthCredentials {
				credName := credentialLabel(name)
				fmt.Fprintf(ctxt.Stdout, "%s cloud credential %q found\n", cloudName, credName)
			}
			if credentials == nil {
				credentials = detected
			} else {
				for name, cred := range detected.AuthCredentials {
					credName := credentialLabel(name)
					credentials.AuthCredentials[credName] = cred
				}
			}
			if c.Replace && detected.DefaultRegion != "" {
				credentials.DefaultRegion = detected.DefaultRegion
			}
			if err := c.store.UpdateCredential(cloudName, *credentials); err != nil {
				return errors.Annotatef(err, "cannot add credentials for cloud %q", cloudName)
			}
		}
	}
	return nil
}

func credentialLabel(name string) string {
	if name != "" {
		return name
	}
	return "default"
}
