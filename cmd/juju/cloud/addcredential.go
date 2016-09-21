// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"golang.org/x/crypto/ssh/terminal"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
)

var usageAddCredentialSummary = `
Adds or replaces credentials for a cloud.`[1:]

var usageAddCredentialDetails = `
The user is prompted to add credentials interactively if a YAML-formatted
credentials file is not specified. Here is a sample credentials file:

credentials:
  aws:
    <credential name>:
      auth-type: access-key
      access-key: <key>
      secret-key: <key>
  azure:
    <credential name>:
      auth-type: userpass
      application-id: <uuid1>
      application-password: <password>
      subscription-id: <uuid2>
      tenant-id: <uuid3>

A "credential name" is arbitrary and is used solely to represent a set of
credentials, of which there may be multiple per cloud.
The ` + "`--replace`" + ` option is required if credential information for the named
cloud already exists. All such information will be overwritten.
This command does not set default regions nor default credentials. Note
that if only one credential name exists, it will become the effective
default credential.
For credentials which are already in use by tools other than Juju, ` + "`juju \nautoload-credentials`" + ` may be used.
When Juju needs credentials for a cloud, i) if there are multiple
available; ii) there's no set default; iii) and one is not specified ('--
credential'), an error will be emitted.

Examples:
    juju add-credential google
    juju add-credential aws -f ~/credentials.yaml

See also: 
    credentials
    remove-credential
    set-default-credential
    autoload-credentials`

type addCredentialCommand struct {
	cmd.CommandBase
	store           jujuclient.CredentialStore
	cloudByNameFunc func(string) (*jujucloud.Cloud, error)

	// Replace, if true, existing credential information is overwritten.
	Replace bool

	// CloudName is the name of the cloud for which we add credentials.
	CloudName string

	// CredentialsFile is the name of the credentials YAML file.
	CredentialsFile string

	cloud *jujucloud.Cloud
}

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
		Args:    "<cloud name>",
		Purpose: usageAddCredentialSummary,
		Doc:     usageAddCredentialDetails,
	}
}

func (c *addCredentialCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.BoolVar(&c.Replace, "replace", false, "Overwrite existing credential information")
	f.StringVar(&c.CredentialsFile, "f", "", "The YAML file containing credentials to add")
}

func (c *addCredentialCommand) Init(args []string) (err error) {
	if len(args) < 1 {
		return errors.New("Usage: juju add-credential <cloud-name> [-f <credentials.yaml>]")
	}
	c.CloudName = args[0]
	return cmd.CheckEmpty(args[1:])
}

func (c *addCredentialCommand) Run(ctxt *cmd.Context) error {
	// Check that the supplied cloud is valid.
	var err error
	if c.cloud, err = common.CloudOrProvider(c.CloudName, c.cloudByNameFunc); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	}
	if len(c.cloud.AuthTypes) == 0 {
		return errors.Errorf("cloud %q does not require credentials", c.CloudName)
	}

	if c.CredentialsFile == "" {
		credentialsProvider, err := environs.Provider(c.cloud.Type)
		if err != nil {
			return errors.Annotate(err, "getting provider for cloud")
		}
		return c.interactiveAddCredential(ctxt, credentialsProvider.CredentialSchemas())
	}
	data, err := ioutil.ReadFile(c.CredentialsFile)
	if err != nil {
		return errors.Annotate(err, "reading credentials file")
	}

	specifiedCredentials, err := jujucloud.ParseCredentials(data)
	if err != nil {
		return errors.Annotate(err, "parsing credentials file")
	}
	credentials, ok := specifiedCredentials[c.CloudName]
	if !ok {
		return errors.Errorf("no credentials for cloud %s exist in file %s", c.CloudName, c.CredentialsFile)
	}
	existingCredentials, err := c.existingCredentialsForCloud()
	if err != nil {
		return errors.Trace(err)
	}
	// If there are *any* credentials already for the cloud, we'll ask for the --replace flag.
	if !c.Replace && len(existingCredentials.AuthCredentials) > 0 && len(credentials.AuthCredentials) > 0 {
		return errors.Errorf("credentials for cloud %s already exist; use --replace to overwrite / merge", c.CloudName)
	}
	for name, cred := range credentials.AuthCredentials {
		existingCredentials.AuthCredentials[name] = cred
	}
	err = c.store.UpdateCredential(c.CloudName, *existingCredentials)
	if err != nil {
		return err
	}
	fmt.Fprintf(ctxt.Stdout, "Credentials updated for cloud %q.\n", c.CloudName)
	return nil
}

func (c *addCredentialCommand) existingCredentialsForCloud() (*jujucloud.CloudCredential, error) {
	existingCredentials, err := c.store.CredentialForCloud(c.CloudName)
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Annotate(err, "reading existing credentials for cloud")
	}
	if errors.IsNotFound(err) {
		existingCredentials = &jujucloud.CloudCredential{
			AuthCredentials: make(map[string]jujucloud.Credential),
		}
	}
	return existingCredentials, nil
}

func (c *addCredentialCommand) interactiveAddCredential(ctxt *cmd.Context, schemas map[jujucloud.AuthType]jujucloud.CredentialSchema) error {
	var err error
	credentialName, err := c.promptCredentialName(ctxt.Stderr, ctxt.Stdin)
	if err != nil {
		return errors.Trace(err)
	}
	if credentialName == "" {
		fmt.Fprintln(ctxt.Stderr, "Credentials entry aborted.")
		return nil
	}

	// Prompt to overwrite if needed.
	existingCredentials, err := c.existingCredentialsForCloud()
	if err != nil {
		return errors.Trace(err)
	}
	if _, ok := existingCredentials.AuthCredentials[credentialName]; ok {
		overwrite, err := c.promptReplace(ctxt.Stderr, ctxt.Stdin)
		if err != nil {
			return errors.Trace(err)
		}
		if !overwrite {
			return nil
		}
	}

	authType, err := c.promptAuthType(ctxt.Stderr, ctxt.Stdin, c.cloud.AuthTypes)
	if err != nil {
		return errors.Trace(err)
	}
	schema, ok := schemas[authType]
	if !ok {
		return errors.NotSupportedf("auth type %q for cloud %q", authType, c.CloudName)
	}

	attrs, err := c.promptCredentialAttributes(ctxt, ctxt.Stderr, ctxt.Stdin, authType, schema)
	if err != nil {
		return errors.Trace(err)
	}

	cloudEndpoint := c.cloud.Endpoint
	cloudIdentityEndpoint := c.cloud.IdentityEndpoint
	if len(c.cloud.Regions) > 0 {
		// NOTE(axw) we use the first region in the cloud,
		// because this is all we need for Azure right now.
		// Each region has the same endpoints, so it does
		// not matter which one we use. If we expand
		// credential generation to other providers, and
		// they do have region-specific endpoints, then we
		// should prompt the user for the region to use.
		// That would be better left to the provider, though.
		region := c.cloud.Regions[0]
		cloudEndpoint = region.Endpoint
		cloudIdentityEndpoint = region.IdentityEndpoint
	}

	credentialsProvider, err := environs.Provider(c.cloud.Type)
	if err != nil {
		return errors.Trace(err)
	}
	newCredential, err := credentialsProvider.FinalizeCredential(
		ctxt, environs.FinalizeCredentialParams{
			Credential:            jujucloud.NewCredential(authType, attrs),
			CloudEndpoint:         cloudEndpoint,
			CloudIdentityEndpoint: cloudIdentityEndpoint,
		},
	)
	if err != nil {
		return errors.Annotate(err, "finalizing credential")
	}

	existingCredentials.AuthCredentials[credentialName] = *newCredential
	err = c.store.UpdateCredential(c.CloudName, *existingCredentials)
	if err != nil {
		return errors.Trace(err)
	}
	fmt.Fprintf(ctxt.Stdout, "Credentials added for cloud %s.\n\n", c.CloudName)
	return nil
}

func (c *addCredentialCommand) promptCredentialName(out io.Writer, in io.Reader) (string, error) {
	fmt.Fprint(out, "Enter credential name: ")
	input, err := readLine(in)
	if err != nil {
		return "", errors.Trace(err)
	}
	return strings.TrimSpace(input), nil
}

func (c *addCredentialCommand) promptReplace(out io.Writer, in io.Reader) (bool, error) {
	fmt.Fprint(out, `
A credential with that name already exists.

Replace the existing credential? (y/N): `[1:])

	input, err := readLine(in)
	if err != nil {
		return false, errors.Trace(err)
	}
	return strings.ToLower(strings.TrimSpace(input)) == "y", nil
}

func (c *addCredentialCommand) promptAuthType(out io.Writer, in io.Reader, authTypes []jujucloud.AuthType) (jujucloud.AuthType, error) {
	if len(authTypes) == 1 {
		fmt.Fprintf(out, "Using auth-type %q.\n", authTypes[0])
		return authTypes[0], nil
	}
	authType := ""
	choices := make([]string, len(authTypes))
	for i, a := range authTypes {
		choices[i] = string(a)
		if i == 0 {
			choices[i] += "*"
		}
	}
	for {
		fmt.Fprintf(out, "Auth Types\n%s\n\nSelect auth-type: ",
			strings.Join(choices, "\n"))
		input, err := readLine(in)
		if err != nil {
			return "", errors.Trace(err)
		}
		authType = strings.ToLower(strings.TrimSpace(input))
		if authType == "" {
			authType = string(authTypes[0])
		}
		isValid := false
		for _, a := range authTypes {
			if string(a) == authType {
				isValid = true
				break
			}
		}
		if isValid {
			break
		}
		fmt.Fprintf(out, "Invalid auth type %q.\n", authType)
	}
	return jujucloud.AuthType(authType), nil
}

func (c *addCredentialCommand) promptCredentialAttributes(
	ctxt *cmd.Context, out io.Writer, in io.Reader, authType jujucloud.AuthType, schema jujucloud.CredentialSchema,
) (map[string]string, error) {

	attrs := make(map[string]string)
	for _, attr := range schema {
		currentAttr := attr
		value := ""
		for {
			var err error
			// Interactive add does not support adding multi-line values, which
			// is what we typically get when the attribute can come from a file.
			// For now we'll skip, and just get the user to enter the file path.
			// TODO(wallyworld) - add support for multi-line entry
			if currentAttr.FileAttr == "" {
				value, err = c.promptFieldValue(out, in, currentAttr)
				if err != nil {
					return nil, err
				}
			}
			// Validate the entered value matches any options.
			// If the user just hits Enter, the first option is used.
			if len(currentAttr.Options) > 0 {
				isValid := false
				for _, choice := range currentAttr.Options {
					if choice == value || value == "" {
						isValid = true
						break
					}
				}
				if !isValid {
					fmt.Fprintf(out, "Invalid value %q.\n", value)
					continue
				}
				if value == "" && !currentAttr.Optional {
					value = fmt.Sprintf("%v", currentAttr.Options[0])
				}
			}

			// If the entered value is empty and the attribute can come
			// from a file, prompt for that.
			if value == "" && currentAttr.FileAttr != "" {
				fileAttr := currentAttr
				fileAttr.Name = currentAttr.FileAttr
				fileAttr.Hidden = false
				fileAttr.FilePath = true
				currentAttr = fileAttr
				value, err = c.promptFieldValue(out, in, currentAttr)
				if err != nil {
					return nil, err
				}
			}

			// Validate any file attribute is a valid file.
			if value != "" && currentAttr.FilePath {
				value, err = jujucloud.ValidateFileAttrValue(value)
				if err != nil {
					fmt.Fprintf(out, "Invalid file attribute %q.\n", err.Error())
					continue
				}
			}

			// Stay in the loop if we need a mandatory value.
			if value != "" || currentAttr.Optional {
				break
			}
		}
		if value != "" {
			attrs[currentAttr.Name] = value
		}
	}
	return attrs, nil
}

func (c *addCredentialCommand) promptFieldValue(
	out io.Writer, in io.Reader, attr jujucloud.NamedCredentialAttr,
) (string, error) {

	name := attr.Name
	// Formulate the prompt for the list of valid options.
	optionsPrompt := ""
	if len(attr.Options) > 0 {
		options := make([]string, len(attr.Options))
		for i, opt := range attr.Options {
			options[i] = fmt.Sprintf("%v", opt)
			if i == 0 {
				options[i] += "*"
			}
		}
		optionsPrompt = fmt.Sprintf(" [%v]", strings.Join(options, ","))
	}

	// Prompt for and accept input for field value.
	fmt.Fprintf(out, "Enter %s%s: ", name, optionsPrompt)
	var input string
	var err error
	if attr.Hidden {
		input, err = c.readHiddenField(in)
		fmt.Fprintln(out)
	} else {
		input, err = readLine(in)
	}
	if err != nil {
		return "", errors.Trace(err)
	}
	value := strings.TrimSpace(input)
	return value, nil
}

func (c *addCredentialCommand) readHiddenField(in io.Reader) (string, error) {
	if f, ok := in.(*os.File); ok && terminal.IsTerminal(int(f.Fd())) {
		value, err := terminal.ReadPassword(int(f.Fd()))
		if err != nil {
			return "", errors.Trace(err)
		}
		return string(value), nil
	}
	return readLine(in)
}
