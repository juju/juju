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
	"github.com/juju/utils/set"
	"golang.org/x/crypto/ssh/terminal"
	"launchpad.net/gnuflag"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
)

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

	cloud               *jujucloud.Cloud
	credentialsProvider environs.ProviderCredentials
}

var addCredentialDoc = `
The add-credential command adds or replaces credentials for a given cloud.

The user is required to specify the name of the cloud for which credentials
will be added/replaced, and optionally a YAML file containing credentials.
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

If no YAML file is specified, the user is prompted to add credentials interactively.

Example:
   juju add-credential aws
   juju add-credential aws -f my-credentials.yaml
   juju add-credential aws -f my-credentials.yaml --replace

See Also:
   juju list-credentials
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
	if len(args) < 1 {
		return errors.New("Usage: juju add-credential <cloud-name> [-f <credentials.yaml>]")
	}
	// Check that the supplied cloud is valid.
	c.CloudName = args[0]
	if c.cloud, err = c.cloudByNameFunc(c.CloudName); err != nil {
		if errors.IsNotFound(err) {
			return errors.NotValidf("cloud %v", c.CloudName)
		}
		return err
	} else {
		if len(c.cloud.AuthTypes) == 0 {
			return errors.Errorf("cloud %q does not require credentials", c.cloud)
		}
		c.credentialsProvider, err = environs.Provider(c.cloud.Type)
		if err != nil {
			return errors.Annotate(err, "getting provider for cloud")
		}
	}
	return cmd.CheckEmpty(args[1:])
}

func (c *addCredentialCommand) Run(ctxt *cmd.Context) error {
	if c.CredentialsFile == "" {
		return c.interactiveAddCredential(ctxt)
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
	fmt.Fprintf(ctxt.Stdout, "credentials updated for cloud %s\n", c.CloudName)
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

func (c *addCredentialCommand) interactiveAddCredential(ctxt *cmd.Context) error {
	var err error
	credentialName, err := c.promptCredentialName(ctxt.Stderr, ctxt.Stdin)
	if err != nil {
		return errors.Trace(err)
	}
	if credentialName == "" {
		fmt.Fprintln(ctxt.Stderr, "credentials entry aborted")
		return nil
	}
	authType, err := c.promptAuthType(ctxt.Stderr, ctxt.Stdin, c.cloud.AuthTypes)
	if err != nil {
		return errors.Trace(err)
	}
	schema, ok := c.credentialsProvider.CredentialSchemas()[authType]
	if !ok {
		return errors.NotSupportedf("auth type %q for cloud %q", authType, c.CloudName)
	}

	attrs, err := c.promptCredentialAttributes(ctxt, ctxt.Stderr, ctxt.Stdin, authType, schema)
	if err != nil {
		return errors.Trace(err)
	}
	existingCredentials, err := c.existingCredentialsForCloud()
	if err != nil {
		return errors.Trace(err)
	}
	newCredential := jujucloud.NewCredential(authType, attrs)
	existingCredentials.AuthCredentials[credentialName] = newCredential
	err = c.store.UpdateCredential(c.CloudName, *existingCredentials)
	if err != nil {
		return errors.Trace(err)
	}
	fmt.Fprintf(ctxt.Stdout, "credentials added for cloud %s\n\n", c.CloudName)
	return nil
}

func (c *addCredentialCommand) promptCredentialName(out io.Writer, in io.Reader) (string, error) {
	fmt.Fprint(out, "  credential name: ")
	input, err := readLine(in)
	if err != nil {
		return "", errors.Trace(err)
	}
	return strings.TrimSpace(input), nil
}

func (c *addCredentialCommand) promptAuthType(out io.Writer, in io.Reader, authTypes []jujucloud.AuthType) (jujucloud.AuthType, error) {
	if len(authTypes) == 1 {
		fmt.Fprintf(out, "  auth-type: %v\n", authTypes[0])
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
		fmt.Fprintf(out, "  select auth-type [%v]: ", strings.Join(choices, ", "))
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
		fmt.Fprintf(out, "  ...invalid auth type %q\n", authType)
	}
	return jujucloud.AuthType(authType), nil
}

func (c *addCredentialCommand) promptCredentialAttributes(
	ctxt *cmd.Context, out io.Writer, in io.Reader, authType jujucloud.AuthType, schema jujucloud.CredentialSchema,
) (map[string]string, error) {

	// Record any file attributes for later validation.
	fileAttrs := set.NewStrings()
	for _, attr := range schema {
		if attr.FileAttr != "" {
			fileAttrs.Add(attr.FileAttr)
		}
	}

	attrs := make(map[string]string)
	for name, attr := range schema {
		value := ""
		for {
			// Formulate the prompt for the list of valid options.
			choicesPrompt := ""
			if len(attr.Options) > 0 {
				options := make([]string, len(attr.Options))
				for i, opt := range attr.Options {
					options[i] = fmt.Sprintf("%v", opt)
					if i == 0 {
						options[i] += "*"
					}
				}
				choicesPrompt = fmt.Sprintf(" [%v]", strings.Join(options, ","))
			}

			// Prompt for and accept input for field value.
			fmt.Fprintf(out, "  %s%s: ", name, choicesPrompt)
			var input string
			var err error
			if attr.Hidden {
				input, err = c.readHiddenField(in)
				fmt.Fprintln(out)
			} else {
				input, err = readLine(in)
			}
			if err != nil {
				return nil, errors.Trace(err)
			}
			value = strings.TrimSpace(input)

			// Validate the entered value matches any options.
			if len(attr.Options) > 0 {
				isValid := false
				for _, choice := range attr.Options {
					if choice == value || value == "" {
						isValid = true
						break
					}
				}
				if !isValid {
					fmt.Fprintf(out, "  ...invalid value %q\n", value)
					continue
				}
				if value == "" && !attr.Optional {
					value = fmt.Sprintf("%v", attr.Options[0])
				}
			}

			// Validate any file attribute is a valid file.
			if value != "" && (fileAttrs.Contains(name) || attr.FilePath) {
				filename := ctxt.AbsPath(value)
				info, err := os.Stat(filename)
				if err != nil {
					fmt.Fprintf(out, "  ...invalid file path: %s\n", filename)
					continue
				}
				if info.IsDir() {
					fmt.Fprintf(out, "  ...file path must be a file: %s\n", filename)
					continue
				}
			}

			// Stay in the loop if we need a mandatory value.
			if value != "" || attr.Optional {
				break
			}
		}
		if value != "" {
			attrs[name] = value
		}
	}
	return attrs, nil
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
