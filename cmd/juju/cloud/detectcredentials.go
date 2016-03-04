// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/set"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
)

type detectCredentialsCommand struct {
	cmd.CommandBase
	out cmd.Output

	store jujuclient.CredentialStore

	// registeredProvidersFunc is set by tests to return all registered environ providers
	registeredProvidersFunc func() []string

	// allCloudsFunc is set by tests to return all public and personal clouds
	allCloudsFunc func() (map[string]jujucloud.Cloud, error)

	// cloudByNameFunc is set by tests to return a named cloud.
	cloudByNameFunc func(string) (*jujucloud.Cloud, error)
}

var detectCredentialsDoc = `
The autoload-credentials command looks for well known locations for supported clouds and
allows the user to interactively save these into the Juju credentials store to make these
available when bootstrapping new controllers and creating new models.

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
    
OpenStack
  Credentials are located in:
    1. On Linux, $HOME/.novarc
    2. Environment variables OS_USERNAME, OS_PASSWORD, OS_TENANT_NAME 
    
Example:
   juju autoload-credentials
   
See Also:
   juju list-credentials
   juju add-credential
`

// NewDetectCredentialsCommand returns a command to add credential information to credentials.yaml.
func NewDetectCredentialsCommand() cmd.Command {
	c := &detectCredentialsCommand{
		store: jujuclient.NewFileCredentialStore(),
		registeredProvidersFunc: environs.RegisteredProviders,
		cloudByNameFunc:         jujucloud.CloudByName,
	}
	c.allCloudsFunc = func() (map[string]jujucloud.Cloud, error) {
		return c.allClouds()
	}
	return c
}

func (c *detectCredentialsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "autoload-credentials",
		Purpose: "looks for cloud credentials and caches those for use by Juju when bootstrapping",
		Doc:     detectCredentialsDoc,
	}
}

type discoveredCredential struct {
	defaultCloudName string
	cloudType        string
	region           string
	credentialName   string
	credential       jujucloud.Credential
	isNew            bool
}

func (c *detectCredentialsCommand) allClouds() (map[string]jujucloud.Cloud, error) {
	clouds, _, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	if err != nil {
		return nil, err
	}
	personalClouds, err := jujucloud.PersonalCloudMetadata()
	if err != nil {
		return nil, err
	}
	for k, v := range personalClouds {
		clouds[k] = v
	}
	return clouds, nil
}

func (c *detectCredentialsCommand) Run(ctxt *cmd.Context) error {
	fmt.Fprintln(ctxt.Stderr, "\nLooking for cloud and credential information locally...")

	clouds, err := c.allCloudsFunc()
	if err != nil {
		return errors.Trace(err)
	}

	// Let's ensure a consistent order.
	var sortedCloudNames []string
	for cloudName := range clouds {
		sortedCloudNames = append(sortedCloudNames, cloudName)
	}
	sort.Strings(sortedCloudNames)

	// The default cloud name for each provider type is the
	// first cloud in the sorted list.
	defaultCloudNames := make(map[string]string)
	for _, cloudName := range sortedCloudNames {
		cloud := clouds[cloudName]
		if _, ok := defaultCloudNames[cloud.Type]; ok {
			continue
		}
		defaultCloudNames[cloud.Type] = cloudName
	}

	providerNames := c.registeredProvidersFunc()
	sort.Strings(providerNames)

	var discovered []discoveredCredential
	discoveredLabels := set.NewStrings()
	for _, providerName := range providerNames {
		provider, err := environs.Provider(providerName)
		if err != nil {
			// Should never happen but it will on go 1.2
			// because lxd provider is not built.
			logger.Warningf("provider %q not available on this platform", providerName)
			continue
		}
		if detectCredentials, ok := provider.(environs.ProviderCredentials); ok {
			detected, err := detectCredentials.DetectCredentials()
			if err != nil && !errors.IsNotFound(err) {
				logger.Warningf("could not detect credentials for provider %q: %v", providerName, err)
				continue
			}
			if errors.IsNotFound(err) || len(detected.AuthCredentials) == 0 {
				continue
			}

			// For each credential, construct meta info for which cloud it may pertain to etc.
			for credName, newCred := range detected.AuthCredentials {
				if credName == "" {
					logger.Warningf("ignoring unnamed credential for provider %s", providerName)
					continue
				}
				// Ignore empty credentials.
				if newCred.AuthType() == jujucloud.EmptyAuthType {
					continue
				}
				// Check that another provider hasn't loaded the same credential.
				if discoveredLabels.Contains(newCred.Label) {
					continue
				}
				discoveredLabels.Add(newCred.Label)

				credInfo := discoveredCredential{
					cloudType:      providerName,
					credentialName: credName,
					credential:     newCred,
				}

				// Fill in the default cloud and other meta information.
				defaultCloud, existingDefaultRegion, isNew, err := c.guessCloudInfo(sortedCloudNames, clouds, providerName, credName)
				if err != nil {
					return errors.Trace(err)
				}
				if defaultCloud == "" {
					defaultCloud = defaultCloudNames[providerName]
				}
				credInfo.defaultCloudName = defaultCloud
				if isNew {
					credInfo.defaultCloudName = defaultCloudNames[providerName]
				}
				if (isNew || existingDefaultRegion == "") && detected.DefaultRegion != "" {
					credInfo.region = detected.DefaultRegion
				}
				credInfo.isNew = isNew
				discovered = append(discovered, credInfo)
			}
		}
	}
	if len(discovered) == 0 {
		fmt.Fprintln(ctxt.Stderr, "No cloud credentials found.")
		return nil
	}
	return c.interactiveCredentialsUpdate(ctxt, discovered)
}

// guessCloudInfo looks at all the compatible clouds for the provider name and
// looks to see whether the credential name exists already.
// The first match allows the default cloud and region to be set. The default
// cloud is used when prompting to save a credential. The sorted cloud names
// ensures that "aws" is preferred over "aws-china".
func (c *detectCredentialsCommand) guessCloudInfo(
	sortedCloudNames []string,
	clouds map[string]jujucloud.Cloud,
	providerName, credName string,
) (defaultCloud, defaultRegion string, isNew bool, _ error) {
	isNew = true
	for _, cloudName := range sortedCloudNames {
		cloud := clouds[cloudName]
		if cloud.Type != providerName {
			continue
		}
		credentials, err := c.store.CredentialForCloud(cloudName)
		if err != nil && !errors.IsNotFound(err) {
			return "", "", false, errors.Trace(err)
		}
		if err != nil {
			// None found.
			continue
		}
		existingCredNames := set.NewStrings()
		for name := range credentials.AuthCredentials {
			existingCredNames.Add(name)
		}
		isNew = !existingCredNames.Contains(credName)
		if defaultRegion == "" && credentials.DefaultRegion != "" {
			defaultRegion = credentials.DefaultRegion
		}
		if defaultCloud == "" {
			defaultCloud = cloudName
		}
	}
	return defaultCloud, defaultRegion, isNew, nil
}

// interactiveCredentialsUpdate prints a list of the discovered credentials
// and prompts the user to update their local credentials.
func (c *detectCredentialsCommand) interactiveCredentialsUpdate(ctxt *cmd.Context, discovered []discoveredCredential) error {
	for {
		// Prompt for a credential to save.
		c.printCredentialOptions(ctxt, discovered)
		var input string
		for {
			var err error
			input, err = c.promptCredentialNumber(ctxt.Stderr, ctxt.Stdin)
			if err != nil {
				return errors.Trace(err)
			}
			if strings.ToLower(input) == "q" {
				return nil
			}
			if input != "" {
				break
			}
		}

		// Check the entered number.
		num, err := strconv.Atoi(input)
		if err != nil || num < 1 || num > len(discovered) {
			fmt.Fprintf(ctxt.Stderr, "Invalid choice, enter a number between 1 and %v\n", len(discovered))
			continue
		}
		cred := discovered[num-1]
		// Prompt for the cloud for which to save the credential.
		cloudName, err := c.promptCloudName(ctxt.Stderr, ctxt.Stdin, cred.defaultCloudName, cred.cloudType)
		if err != nil {
			fmt.Fprintln(ctxt.Stderr, err.Error())
			continue
		}
		if cloudName == "" {
			fmt.Fprintln(ctxt.Stderr, "No cloud name entered.")
			continue
		}

		// Reading existing info so we can apply updated values.
		existing, err := c.store.CredentialForCloud(cloudName)
		if err != nil && !errors.IsNotFound(err) {
			fmt.Fprintf(ctxt.Stderr, "error reading credential file: %v\n", err)
			continue
		}
		if errors.IsNotFound(err) {
			existing = &jujucloud.CloudCredential{
				AuthCredentials: make(map[string]jujucloud.Credential),
			}
		}
		if cred.region != "" {
			existing.DefaultRegion = cred.region
		}
		existing.AuthCredentials[cred.credentialName] = cred.credential
		if err := c.store.UpdateCredential(cloudName, *existing); err != nil {
			fmt.Fprintf(ctxt.Stderr, "error saving credential: %v\n", err)
		} else {
			// Update so we display correctly next time list is printed.
			cred.isNew = false
			discovered[num-1] = cred
			fmt.Fprintf(ctxt.Stderr, "Saved %s to cloud %s\n", cred.credential.Label, cloudName)
		}
	}
}

func (c *detectCredentialsCommand) printCredentialOptions(ctxt *cmd.Context, discovered []discoveredCredential) {
	fmt.Fprintln(ctxt.Stderr)
	for i, cred := range discovered {
		suffixText := " (existing, will overwrite)"
		if cred.isNew {
			suffixText = " (new)"
		}
		fmt.Fprintf(ctxt.Stderr, "%d. %s%s\n", i+1, cred.credential.Label, suffixText)
	}
}

func (c *detectCredentialsCommand) promptCredentialNumber(out io.Writer, in io.Reader) (string, error) {
	fmt.Fprint(out, "Save any? Type number, or Q to quit, then enter. ")
	defer out.Write([]byte{'\n'})
	input, err := c.readLine(in)
	if err != nil {
		return "", errors.Trace(err)
	}
	return strings.TrimSpace(input), nil
}

func (c *detectCredentialsCommand) promptCloudName(out io.Writer, in io.Reader, defaultCloudName, cloudType string) (string, error) {
	text := fmt.Sprintf(`Enter cloud to which the credential belongs, or Q to quit [%s] `, defaultCloudName)
	fmt.Fprint(out, text)
	defer out.Write([]byte{'\n'})
	input, err := c.readLine(in)
	if err != nil {
		return "", errors.Trace(err)
	}
	cloudName := strings.TrimSpace(input)
	if strings.ToLower(cloudName) == "q" {
		return "", nil
	}
	if cloudName == "" {
		return defaultCloudName, nil
	}
	cloud, err := c.cloudByNameFunc(cloudName)
	if err != nil {
		return "", err
	}
	if cloud.Type != cloudType {
		return "", errors.Errorf("chosen credentials not compatible with a %s cloud", cloud.Type)
	}
	return cloudName, nil
}

func (c *detectCredentialsCommand) readLine(stdin io.Reader) (string, error) {
	// Read one byte at a time to avoid reading beyond the delimiter.
	line, err := bufio.NewReader(byteAtATimeReader{stdin}).ReadString('\n')
	if err != nil {
		return "", errors.Trace(err)
	}
	return line[:len(line)-1], nil
}

type byteAtATimeReader struct {
	io.Reader
}

func (r byteAtATimeReader) Read(out []byte) (int, error) {
	return r.Reader.Read(out[:1])
}
