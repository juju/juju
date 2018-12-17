// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/juju/interact"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
)

var usageAddCredentialSummary = `
Adds or replaces credentials for a cloud, stored locally on this client.`[1:]

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
      auth-type: service-principal-secret
      application-id: <uuid1>
      application-password: <password>
      subscription-id: <uuid2>
  lxd:
    <credential name>:
      auth-type: interactive
      trust-password: <password>

A "credential name" is arbitrary and is used solely to represent a set of
credentials, of which there may be multiple per cloud.
The ` + "`--replace`" + ` option is required if credential information for the named
cloud already exists locally. All such information will be overwritten.
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
	return jujucmd.Info(&cmd.Info{
		Name:    "add-credential",
		Args:    "<cloud name>",
		Purpose: usageAddCredentialSummary,
		Doc:     usageAddCredentialDetails,
	})
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

	credentialsProvider, err := environs.Provider(c.cloud.Type)
	if err != nil {
		return errors.Annotate(err, "getting provider for cloud")
	}

	if len(c.cloud.AuthTypes) == 0 {
		return errors.Errorf("cloud %q does not require credentials", c.CloudName)
	}

	schemas := credentialsProvider.CredentialSchemas()
	if c.CredentialsFile == "" {
		return c.interactiveAddCredential(ctxt, schemas)
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
		return errors.Errorf("local credentials for cloud %q already exist; use --replace to overwrite / merge", c.CloudName)
	}

	// We could get a duplicate "interactive" entry for the validAuthType() call,
	// however it doesn't matter for the validation, so just add it.
	authTypeNames := c.cloud.AuthTypes
	if _, ok := schemas[jujucloud.InteractiveAuthType]; ok {
		authTypeNames = append(authTypeNames, jujucloud.InteractiveAuthType)
	}

	validAuthType := func(authType jujucloud.AuthType) bool {
		for _, authT := range authTypeNames {
			if authT == authType {
				return true
			}
		}
		return false
	}

	var names []string
	for name, cred := range credentials.AuthCredentials {
		if !validAuthType(cred.AuthType()) {
			return errors.Errorf("credential %q contains invalid auth type %q, valid auth types for cloud %q are %v", name, cred.AuthType(), c.CloudName, c.cloud.AuthTypes)
		}

		provider, err := environs.Provider(c.cloud.Type)
		if err != nil {
			return errors.Trace(err)
		}

		// When in non-interactive mode we still sometimes want to finalize a
		// cloud, so that we can either validate the credentials work before a
		// bootstrap happens or improve security models, where by we remove any
		// shared/secret passwords (lxd remote security).
		// This is optional and is backwards compatible with other providers.
		if shouldFinalizeCredential(provider, cred) {
			newCredential, err := c.finalizeProvider(ctxt, cred.AuthType(), cred.Attributes())
			if err != nil {
				return errors.Trace(err)
			}
			cred = *newCredential
		}
		existingCredentials.AuthCredentials[name] = cred
		names = append(names, name)
	}
	err = c.store.UpdateCredential(c.CloudName, *existingCredentials)
	if err != nil {
		return err
	}
	verb := "added"
	if c.Replace {
		verb = "updated"
	}
	fmt.Fprintf(ctxt.Stdout, "Credentials %q %v for cloud %q.\n", strings.Join(names, ", "), verb, c.CloudName)
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
	errout := interact.NewErrWriter(ctxt.Stdout)
	pollster := interact.New(ctxt.Stdin, ctxt.Stdout, errout)

	var err error
	credentialName, err := pollster.Enter("credential name")
	if err != nil {
		return errors.Trace(err)
	}

	// Prompt to overwrite if needed.
	existingCredentials, err := c.existingCredentialsForCloud()
	if err != nil {
		return errors.Trace(err)
	}
	verb := "added"
	if _, ok := existingCredentials.AuthCredentials[credentialName]; ok {
		fmt.Fprint(ctxt.Stdout, fmt.Sprintf("A credential %q already exists locally on this client.\n", credentialName))
		overwrite, err := pollster.YN("Replace local credential", false)
		if err != nil {
			return errors.Trace(err)
		}
		if !overwrite {
			return nil
		}
		verb = "updated"
	}
	authTypeNames := c.cloud.AuthTypes
	// Check the credential schema for "interactive", add to list of
	// possible authTypes for add-credential
	if _, ok := schemas[jujucloud.InteractiveAuthType]; ok {
		foundIt := false
		for _, name := range authTypeNames {
			if name == jujucloud.InteractiveAuthType {
				foundIt = true
			}
		}
		if !foundIt {
			authTypeNames = append(authTypeNames, jujucloud.InteractiveAuthType)
		}
	}
	authType, err := c.promptAuthType(pollster, authTypeNames, ctxt.Stdout)
	if err != nil {
		return errors.Trace(err)
	}
	schema, ok := schemas[authType]
	if !ok {
		return errors.NotSupportedf("auth type %q for cloud %q", authType, c.CloudName)
	}

	attrs, err := c.promptCredentialAttributes(pollster, authType, schema)
	if err != nil {
		return errors.Trace(err)
	}

	newCredential, err := c.finalizeProvider(ctxt, authType, attrs)
	if err != nil {
		return errors.Trace(err)
	}

	existingCredentials.AuthCredentials[credentialName] = *newCredential
	err = c.store.UpdateCredential(c.CloudName, *existingCredentials)
	if err != nil {
		return errors.Trace(err)
	}
	fmt.Fprintf(ctxt.Stdout, "Credential %q %v locally for cloud %q.\n\n", credentialName, verb, c.CloudName)
	return nil
}

func (c *addCredentialCommand) finalizeProvider(ctxt *cmd.Context, authType jujucloud.AuthType, attrs map[string]string) (*jujucloud.Credential, error) {
	cloudEndpoint := c.cloud.Endpoint
	cloudStorageEndpoint := c.cloud.StorageEndpoint
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
		cloudStorageEndpoint = region.StorageEndpoint
		cloudIdentityEndpoint = region.IdentityEndpoint
	}

	credentialsProvider, err := environs.Provider(c.cloud.Type)
	if err != nil {
		return nil, errors.Trace(err)
	}
	newCredential, err := credentialsProvider.FinalizeCredential(
		ctxt, environs.FinalizeCredentialParams{
			Credential:            jujucloud.NewCredential(authType, attrs),
			CloudEndpoint:         cloudEndpoint,
			CloudStorageEndpoint:  cloudStorageEndpoint,
			CloudIdentityEndpoint: cloudIdentityEndpoint,
		},
	)
	return newCredential, errors.Annotate(err, "finalizing credential")
}

func (c *addCredentialCommand) promptAuthType(p *interact.Pollster, authTypes []jujucloud.AuthType, out io.Writer) (jujucloud.AuthType, error) {
	if len(authTypes) == 1 {
		fmt.Fprintf(out, "Using auth-type %q.\n\n", authTypes[0])
		return authTypes[0], nil
	}
	choices := make([]string, len(authTypes))
	for i, a := range authTypes {
		choices[i] = string(a)
	}
	// If "interactive" is a valid credential type, choose by default
	// o.w. take the top of the slice
	def := string(jujucloud.InteractiveAuthType)
	if !strings.Contains(strings.Join(choices, " "), def) {
		def = choices[0]
	}
	authType, err := p.Select(interact.List{
		Singular: "auth type",
		Plural:   "auth types",
		Options:  choices,
		Default:  def,
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return jujucloud.AuthType(authType), nil
}

func (c *addCredentialCommand) promptCredentialAttributes(p *interact.Pollster, authType jujucloud.AuthType, schema jujucloud.CredentialSchema) (attributes map[string]string, err error) {
	// Interactive add does not support adding multi-line values, which
	// is what we typically get when the attribute can come from a file.
	// For now we'll skip, and just get the user to enter the file path.
	// TODO(wallyworld) - add support for multi-line entry

	attrs := make(map[string]string)
	for _, attr := range schema {
		currentAttr := attr
		value := ""
		var err error

		if currentAttr.FileAttr == "" {
			value, err = c.promptFieldValue(p, currentAttr)
			if err != nil {
				return nil, err
			}
		} else {
			currentAttr.Name = currentAttr.FileAttr
			currentAttr.Hidden = false
			currentAttr.FilePath = true
			value, err = c.promptFieldValue(p, currentAttr)
			if err != nil {
				return nil, err
			}
		}
		if value != "" {
			attrs[currentAttr.Name] = value
		}
	}
	return attrs, nil
}

func (c *addCredentialCommand) promptFieldValue(p *interact.Pollster, attr jujucloud.NamedCredentialAttr) (string, error) {
	name := attr.Name

	if len(attr.Options) > 0 {
		options := make([]string, len(attr.Options))
		for i, opt := range attr.Options {
			options[i] = fmt.Sprintf("%v", opt)
		}
		return p.Select(interact.List{
			Singular: name,
			Plural:   name,
			Options:  options,
			Default:  options[0],
		})
	}

	// We assume that Hidden, ExpandFilePath and FilePath are mutually
	// exclusive here.
	switch {
	case attr.Hidden:
		return p.EnterPassword(name)
	case attr.ExpandFilePath:
		return enterFile(name, attr.Description, p, true, attr.Optional)
	case attr.FilePath:
		return enterFile(name, attr.Description, p, false, attr.Optional)
	case attr.Optional:
		return p.EnterOptional(name)
	default:
		return p.Enter(name)
	}
}

func enterFile(name, descr string, p *interact.Pollster, expanded, optional bool) (string, error) {
	inputSuffix := ""
	if optional {
		inputSuffix += " (optional)"
	}
	input, err := p.EnterVerify(fmt.Sprintf("%s%s", descr, inputSuffix), func(s string) (ok bool, msg string, err error) {
		if optional && s == "" {
			return true, "", nil
		}
		_, err = jujucloud.ValidateFileAttrValue(s)
		if err != nil {
			return false, err.Error(), nil
		}

		return true, "", nil
	})
	if err != nil {
		return "", errors.Trace(err)
	}

	// If it's optional and the input is empty, then return back out.
	if optional && input == "" {
		return "", nil
	}

	// We have to run this twice, since it has glommed together
	// validation and normalization, and Pollster doesn't deal with the
	// verification function modifying the value.
	abs, err := jujucloud.ValidateFileAttrValue(input)
	if err != nil {
		return "", errors.Trace(err)
	}

	// If we don't need to expand the file path, exit out early.
	if !expanded {
		return abs, err
	}

	// Expand the file path to consume the contents
	contents, err := ioutil.ReadFile(abs)
	return string(contents), errors.Trace(err)
}

func shouldFinalizeCredential(provider environs.EnvironProvider, cred jujucloud.Credential) bool {
	if finalizer, ok := provider.(environs.RequestFinalizeCredential); ok {
		return finalizer.ShouldFinalizeCredential(cred)
	}
	return false
}
