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

	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	apicloud "github.com/juju/juju/api/client/cloud"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/naturalsort"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type detectCredentialsCommand struct {
	modelcmd.OptionalControllerCommand

	cloudType string

	// registeredProvidersFunc is set by tests to return all registered environ providers
	registeredProvidersFunc func() []string

	// allCloudsFunc is set by tests to return all public and personal clouds
	allCloudsFunc func(*cmd.Context) (map[string]jujucloud.Cloud, error)

	// cloudByNameFunc is set by tests to return a named cloud.
	cloudByNameFunc func(string) (*jujucloud.Cloud, error)

	// These attributes are used when adding credentials to a controller.
	credentialAPIFunc func() (CredentialAPI, error)
	remoteClouds      map[string]jujucloud.Cloud
}

const detectCredentialsSummary = `Attempts to automatically detect and add credentials for a cloud.`

var detectCredentialsDoc = `
The command searches well known, cloud-specific locations on this client.
If credential information is found, it is presented to the user
in a series of prompts to facilitated interactive addition and upload.
An alternative to this command is ` + "`juju add-credential`" + `.

After validating the contents, credentials are added to
this Juju client if --client is specified.

To upload credentials to a controller, use --controller option. 

Below are the cloud types for which credentials may be autoloaded,
including the locations searched.

EC2
  Credentials and regions:
    1. On Linux, $HOME/.aws/credentials and $HOME/.aws/config
    2. Environment variables AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY

GCE
  Credentials:
    1. A JSON file whose path is specified by the
       GOOGLE_APPLICATION_CREDENTIALS environment variable
    2. On Linux, $HOME/.config/gcloud/application_default_credentials.json
       Default region is specified by the CLOUDSDK_COMPUTE_REGION environment
       variable.
    3. On Windows, %APPDATA%\gcloud\application_default_credentials.json

OpenStack
  Credentials:
    1. On Linux, $HOME/.novarc
    2. Environment variables OS_USERNAME, OS_PASSWORD, OS_TENANT_NAME,
	   OS_DOMAIN_NAME

LXD
  Credentials:
    1. On Linux, $HOME/.config/lxc/config.yml

`[1:]

const detectCredentialsExamples = `
    juju autoload-credentials
    juju autoload-credentials --client
    juju autoload-credentials --controller mycontroller
    juju autoload-credentials --client --controller mycontroller
    juju autoload-credentials aws
`

// NewDetectCredentialsCommand returns a command to add credential information to credentials.yaml.
func NewDetectCredentialsCommand() cmd.Command {
	c := &detectCredentialsCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store: jujuclient.NewFileClientStore(),
		},
		registeredProvidersFunc: environs.RegisteredProviders,
		cloudByNameFunc:         jujucloud.CloudByName,
	}
	c.allCloudsFunc = func(ctxt *cmd.Context) (map[string]jujucloud.Cloud, error) {
		return c.allClouds(ctxt)
	}
	c.credentialAPIFunc = c.credentialsAPI
	return modelcmd.WrapBase(c)
}

func (c *detectCredentialsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "autoload-credentials",
		Purpose:  detectCredentialsSummary,
		Args:     "[<cloud-type>]",
		Doc:      detectCredentialsDoc,
		Examples: detectCredentialsExamples,
		SeeAlso: []string{
			"add-credential",
			"credentials",
			"default-credential",
			"remove-credential",
		},
	})
}

func (c *detectCredentialsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.OptionalControllerCommand.SetFlags(f)
}

func (c *detectCredentialsCommand) Init(args []string) (err error) {
	if err := c.OptionalControllerCommand.Init(args); err != nil {
		return err
	}
	if len(args) > 0 {
		c.cloudType = strings.ToLower(args[0])
		return cmd.CheckEmpty(args[1:])
	}
	return cmd.CheckEmpty(args)
}

type discoveredCredential struct {
	defaultCloudName string
	cloudType        string
	region           string
	credentialName   string
	credential       jujucloud.Credential
	isNew            bool
	isDefault        bool
}

func (c *detectCredentialsCommand) credentialsAPI() (CredentialAPI, error) {
	var err error
	root, err := c.NewAPIRoot(c.Store, c.ControllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apicloud.NewClient(root), nil
}

func (c *detectCredentialsCommand) allClouds(ctxt *cmd.Context) (map[string]jujucloud.Cloud, error) {
	ctxt.Infof("\nLooking for cloud and credential information on local client...")
	clouds, _, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	if err != nil {
		return nil, err
	}
	builtinClouds, err := common.BuiltInClouds()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for k, v := range builtinClouds {
		clouds[k] = v
	}
	personalClouds, err := jujucloud.PersonalCloudMetadata()
	if err != nil {
		return nil, err
	}
	for k, v := range personalClouds {
		clouds[k] = v
	}
	if c.ControllerName != "" {
		ctxt.Infof("\nLooking for cloud information on controller %q...", c.ControllerName)
		// If there is a cloud definition for the same cloud both
		// on the controller and on the client and they conflict,
		// we want definition from the controller to take precedence.
		client, err := c.credentialAPIFunc()
		if err != nil {
			return nil, err
		}
		defer client.Close()

		remoteUserClouds, err := client.Clouds()
		if err != nil {
			return nil, err
		}
		c.remoteClouds = map[string]jujucloud.Cloud{}
		for k, v := range remoteUserClouds {
			clouds[k.Id()] = v
			c.remoteClouds[k.Id()] = v
		}
	}
	return clouds, nil
}

func (c *detectCredentialsCommand) Run(ctxt *cmd.Context) error {
	if err := c.MaybePrompt(ctxt, "add a credential to"); err != nil {
		return errors.Trace(err)
	}

	clouds, err := c.allCloudsFunc(ctxt)
	if err != nil {
		return errors.Trace(err)
	}

	// Let's ensure a consistent order.
	var sortedCloudNames []string
	for cloudName, cloud := range clouds {
		if c.cloudType != "" && cloud.Type != c.cloudType {
			continue
		}
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
		if c.cloudType != "" && providerName != c.cloudType {
			continue
		}
		provider, err := environs.Provider(providerName)
		if err != nil {
			// Should never happen but it will on go 1.2
			// because lxd provider is not built.
			logger.Errorf("provider %q not available on this platform", providerName)
			continue
		}
		if detectCredentials, ok := provider.(environs.ProviderCredentials); ok {
			detected, err := detectCredentials.DetectCredentials("")
			if err != nil && !errors.IsNotFound(err) {
				logger.Errorf("could not detect credentials for provider %q: %v", providerName, err)
				continue
			}
			if errors.IsNotFound(err) || len(detected.AuthCredentials) == 0 {
				continue
			}
			// Some providers, eg Azure can have spaces in cred names and we don't want that.
			detected.DefaultCredential = strings.ReplaceAll(detected.DefaultCredential, " ", "_")
			sortedName := []string{}
			for credName, cred := range detected.AuthCredentials {
				formattedCredName := strings.ReplaceAll(credName, " ", "_")
				sortedName = append(sortedName, formattedCredName)
				delete(detected.AuthCredentials, credName)
				detected.AuthCredentials[formattedCredName] = cred
			}
			naturalsort.Sort(sortedName)

			// For each credential, construct meta info for which cloud it may pertain to etc.
			for _, credName := range sortedName {
				newCred := detected.AuthCredentials[credName]
				if credName == "" {
					logger.Debugf("ignoring unnamed credential for provider %s", providerName)
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
					isDefault:      detected.DefaultCredential == credName,
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
	return c.interactiveCredentialsUpdate(ctxt, clouds, discovered)
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
		credentials, err := c.Store.CredentialForCloud(cloudName)
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
func (c *detectCredentialsCommand) interactiveCredentialsUpdate(ctxt *cmd.Context, clouds map[string]jujucloud.Cloud, discovered []discoveredCredential) error {
	// by cloud, by region
	loaded := map[string]map[string]map[string]jujucloud.Credential{}
	quit := func(in string) bool {
		return strings.EqualFold(in, "q")
	}
	var localErr error
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
			if quit(input) {
				goto upload
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
		cloudName, err := c.promptCloudName(ctxt.Stderr, ctxt.Stdin, cred.defaultCloudName)
		if err != nil {
			ctxt.Warningf("%v", err.Error())
			continue
		}
		if quit(cloudName) {
			goto upload
		}

		if cloudName == "" {
			fmt.Fprintln(ctxt.Stderr, "No cloud name entered.")
			continue
		}
		cloud, err := common.CloudOrProvider(cloudName, c.cloudByNameFunc)
		if err != nil {
			ctxt.Warningf("%v", err)
			continue
		}
		if cloud.Type != cred.cloudType {
			ctxt.Warningf("%v", errors.Errorf("chosen credential not compatible with %q cloud", cloud.Type))
			continue
		}

		// Reading existing info so we can apply updated values.
		existing, err := c.Store.CredentialForCloud(cloudName)
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
		if existing.DefaultCredential == "" && cred.isDefault {
			existing.DefaultCredential = cred.credentialName
		}
		existing.AuthCredentials[cred.credentialName] = cred.credential
		addLoadedCredential(loaded, cloudName, cred)
		if c.Client {
			if err := c.Store.UpdateCredential(cloudName, *existing); err != nil {
				ctxt.Warningf("error saving credential locally: %v\n", err)
				localErr = err
			} else {
				// Update so we display correctly next time list is printed.
				cred.isNew = false
				discovered[num-1] = cred
				fmt.Fprintf(ctxt.Stderr, "Saved %s to cloud %s locally\n", cred.credential.Label, cloudName)
			}
		}
	}
upload:
	if c.ControllerName != "" {
		fmt.Fprintln(ctxt.Stderr)
		return c.addRemoteCredentials(ctxt, clouds, loaded, localErr)
	}
	return nil
}

func addLoadedCredential(all map[string]map[string]map[string]jujucloud.Credential, cloudName string, cred discoveredCredential) {
	byCloud, ok := all[cloudName]
	if !ok {
		all[cloudName] = map[string]map[string]jujucloud.Credential{}
		byCloud = all[cloudName]
	}
	byRegion, ok := byCloud[cred.region]
	if !ok {
		byCloud[cred.region] = map[string]jujucloud.Credential{}
		byRegion = byCloud[cred.region]
	}
	byRegion[cred.credentialName] = cred.credential
}

func (c *detectCredentialsCommand) addRemoteCredentials(ctxt *cmd.Context, clouds map[string]jujucloud.Cloud, all map[string]map[string]map[string]jujucloud.Credential, localErr error) error {
	if len(all) == 0 {
		ctxt.Infof("No credentials loaded to controller %v.\n", c.ControllerName)
		return nil
	}
	accountDetails, err := c.Store.AccountDetails(c.ControllerName)
	if err != nil {
		return err
	}

	client, err := c.credentialAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	var results []params.UpdateCredentialResult
	moreCloudInfoNeeded := false
	for cloud, byCloud := range all {
		for region, byRegion := range byCloud {
			aCloud, ok := c.remoteClouds[cloud]
			if !ok {
				ctxt.Infof("Cloud %q does not exist on the controller: not uploading credentials for it...", cloud)
				moreCloudInfoNeeded = true
				continue
			}
			verified, erred := verifyCredentialsForUpload(ctxt, accountDetails, &aCloud, region, byRegion)
			if len(verified) == 0 {
				return erred
			}
			result, err := client.AddCloudsCredentials(verified)
			if err != nil {
				logger.Errorf("%v", err)
				ctxt.Warningf("Could not upload credentials to controller %q", c.ControllerName)
			}
			results = append(results, result...)
		}
	}
	if moreCloudInfoNeeded {
		ctxt.Infof("Use 'juju clouds' to view all available clouds and 'juju add-cloud' to add missing ones.")
	}
	return processUpdateCredentialResult(ctxt, accountDetails, "loaded", results, false, c.ControllerName, localErr)
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
	fmt.Fprint(out, "Select a credential to save by number, or type Q to quit: ")
	defer func() { _, _ = out.Write([]byte{'\n'}) }()
	input, err := readLine(in)
	if err != nil {
		return "", errors.Trace(err)
	}
	return strings.TrimSpace(input), nil
}

func (c *detectCredentialsCommand) promptCloudName(out io.Writer, in io.Reader, defaultCloudName string) (string, error) {
	text := fmt.Sprintf(`Select the cloud it belongs to, or type Q to quit [%s]: `, defaultCloudName)
	fmt.Fprint(out, text)
	defer func() { _, _ = out.Write([]byte{'\n'}) }()
	input, err := readLine(in)
	if err != nil {
		return "", errors.Trace(err)
	}
	input = strings.TrimSpace(input)
	if input == "" {
		input = defaultCloudName
	}
	return input, nil
}

func readLine(stdin io.Reader) (string, error) {
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
