// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	apicloud "github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/cloud"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/juju/interact"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
)

var usageAddCredentialSummary = `
Adds a credential for a cloud to a local client and uploads it to a controller.`[1:]

var usageAddCredentialDetails = `
The ` + "`juju add-credential`" + `command operates in two modes.

When called with only the ` + "`<cloud name>` " + `argument, ` + "`juju add-credential` " + `will
take you through an interactive prompt to add a credential specific to
the cloud provider.

Providing the ` + "`-f <credentials.yaml>` " + `option switches to the
non-interactive mode. ` + "`<credentials.yaml>` " + `must be a path to a correctly
formatted YAML-formatted file.

The following sample YAML file shows five credentials being stored against four clouds:

    credentials:
      aws:
        <credential-name>:
          auth-type: access-key
          access-key: <key>
          secret-key: <key>
      azure:
        <credential-name>:
          auth-type: service-principal-secret
          application-id: <uuid>
          application-password: <password>
          subscription-id: <uuid>
      lxd:
        <credential-a>:
          auth-type: interactive
          trust-password: <password>
        <credential-b>:
          auth-type: interactive
          trust-password: <password>
      google:
        <credential-name>:
          auth-type: oauth2
          project-id: <project-id>
          private-key: <private-key>
          client-email: <email>
          client-id: <client-id>

The ` + "`<credential-name>` " + `parameter of each credential is arbitrary, but must
be unique within each ` + "`<cloud-name>`" + `. This allows each cloud to store
multiple credentials.

The format for a credential is cloud-specific. Thus, it's best to use
the ` + "`add-credential` " + `command in an interactive mode. This will result in
adding this new credential locally and / or uploading it to a controller
in a correct format for the desired cloud.


Notes:
If you are setting up Juju for the first time, consider running
` + "`juju autoload-credentials`" + `. This may allow you to skip adding
credentials manually.

This command does not set default regions nor default credentials for the
cloud. The commands ` + "`juju default-region` " + ` and ` + "`juju default-credential`" + `
provide that functionality.

`

const usageAddCredentialExamples = `
    juju add-credential google
    juju add-credential google --client
    juju add-credential google -c mycontroller
    juju add-credential aws -f ~/credentials.yaml -c mycontroller
    juju add-credential aws -f ~/credentials.yaml
    juju add-credential aws -f ~/credentials.yaml --client
`

type addCredentialCommand struct {
	modelcmd.OptionalControllerCommand
	cloudByNameFunc func(string) (*jujucloud.Cloud, error)

	// CloudName is the name of the cloud for which we add credentials.
	CloudName string

	// CredentialsFile is the name of the credentials YAML file.
	CredentialsFile string

	cloud *jujucloud.Cloud

	// Region used to complete credentials' creation.
	Region string

	// These attributes are used when adding credentials to a controller.
	remoteCloudFound  bool
	credentialAPIFunc func() (CredentialAPI, error)
}

// NewAddCredentialCommand returns a command to add credential information.
func NewAddCredentialCommand() cmd.Command {
	store := jujuclient.NewFileClientStore()
	c := &addCredentialCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store: store,
		},
		cloudByNameFunc: jujucloud.CloudByName,
	}
	c.credentialAPIFunc = c.credentialsAPI
	return modelcmd.WrapBase(c)
}

func (c *addCredentialCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-credential",
		Args:     "<cloud name>",
		Purpose:  usageAddCredentialSummary,
		Doc:      usageAddCredentialDetails,
		Examples: usageAddCredentialExamples,
		SeeAlso: []string{
			"credentials",
			"remove-credential",
			"update-credential",
			"default-credential",
			"default-region",
			"autoload-credentials",
		},
	})
}

func (c *addCredentialCommand) SetFlags(f *gnuflag.FlagSet) {
	c.OptionalControllerCommand.SetFlags(f)
	f.StringVar(&c.CredentialsFile, "f", "", "The YAML file containing credentials to add")
	f.StringVar(&c.CredentialsFile, "file", "", "The YAML file containing credentials to add")
	f.StringVar(&c.Region, "region", "", "Cloud region that credential is valid for")
}

func (c *addCredentialCommand) Init(args []string) (err error) {
	if err := c.OptionalControllerCommand.Init(args); err != nil {
		return err
	}
	if len(args) < 1 {
		return errors.New("Usage: juju add-credential <cloud-name> [-f <credentials.yaml>]")
	}
	c.CloudName = args[0]
	return cmd.CheckEmpty(args[1:])
}

func (c *addCredentialCommand) Run(ctxt *cmd.Context) error {
	if err := c.MaybePrompt(ctxt, "add a credential to"); err != nil {
		return errors.Trace(err)
	}

	// Check that the supplied cloud is valid.
	if c.ControllerName != "" {
		if err := c.maybeRemoteCloud(ctxt); err != nil {
			if !errors.IsNotFound(err) {
				logger.Errorf("%v", err)
			}
			ctxt.Infof("Cloud %q is not found on the controller, looking for a locally stored cloud.", c.CloudName)
		}
	}
	if c.cloud == nil {
		var err error
		if c.cloud, err = common.CloudOrProvider(c.CloudName, c.cloudByNameFunc); err != nil {
			logger.Errorf("%v", err)
			ctxt.Infof("To view all available clouds, use 'juju clouds'.\nTo add new cloud, use 'juju add-cloud'.")
			return cmd.ErrSilent
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
	existingCredentials, err := c.existingCredentialsForCloud()
	if err != nil {
		return errors.Trace(err)
	}
	if c.Region != "" {
		if err := validCloudRegion(c.cloud, c.Region); err != nil {
			return err
		}
	}

	if c.CredentialsFile == "" {
		return c.interactiveAddCredential(ctxt, schemas, existingCredentials)
	}

	if c.Region == "" {
		c.Region = existingCredentials.DefaultRegion
	}
	data, err := os.ReadFile(c.CredentialsFile)
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

	provider, err := environs.Provider(c.cloud.Type)
	if err != nil {
		return errors.Trace(err)
	}
	var allNames []string
	added := map[string]jujucloud.Credential{}
	var returnErr error
	for name, cred := range credentials.AuthCredentials {
		if !names.IsValidCloudCredentialName(name) {
			return errors.Errorf("%q is not a valid credential name", name)
		}
		if !validAuthType(cred.AuthType()) {
			return errors.Errorf("credential %q contains invalid auth type %q, valid auth types for cloud %q are %v", name, cred.AuthType(), c.CloudName, c.cloud.AuthTypes)
		}

		cred, err := cloud.ExpandFilePathsOfCredential(cred, schemas)
		if err != nil {
			return fmt.Errorf("expanding file paths for credential: %w", err)
		}

		// When in non-interactive mode we still sometimes want to finalize a
		// cloud, so that we can either validate the credentials work before a
		// bootstrap happens or improve security models, where by we remove any
		// shared/secret passwords (lxd remote security).
		// This is optional and is backwards compatible with other providers.
		if shouldFinalizeCredential(provider, cred) {
			newCredential, err := finalizeProvider(ctxt, c.cloud, c.Region, existingCredentials.DefaultRegion, cred.AuthType(), cred.Attributes())
			if err != nil {
				return errors.Errorf("Could not verify credential %v for cloud %v locally: %v", name, c.CloudName, err)
			}
			cred = *newCredential
		}

		added[name] = cred
		if _, ok := existingCredentials.AuthCredentials[name]; ok {
			// We want to flag to the user as an error but we do not actually want to err here
			// but continue processing the rest of cloud credentials.
			ctxt.Infof("ERROR credential %q for cloud %q already exists locally, use 'juju update-credential %v %v -f %v' to update this local client copy", name, c.CloudName, c.CloudName, name, c.CredentialsFile)
			returnErr = cmd.ErrSilent
			continue
		}

		existingCredentials.AuthCredentials[name] = cred
		allNames = append(allNames, name)
	}
	return c.internalAddCredential(ctxt, "added", *existingCredentials, added, allNames, returnErr)
}

func (c *addCredentialCommand) internalAddCredential(ctxt *cmd.Context, verb string, existingCredentials jujucloud.CloudCredential, added map[string]jujucloud.Credential, allNames []string, returnErr error) error {
	if c.Client {
		// Local processing.
		if len(allNames) == 0 {
			fmt.Fprintf(ctxt.Stdout, "No local credentials for cloud %q changed.\n", c.CloudName)
		} else {
			var msg string
			if len(allNames) == 1 {
				msg = fmt.Sprintf(" %q", allNames[0])
			} else {
				msg = fmt.Sprintf("s %q", strings.Join(allNames, ", "))
			}
			err := c.Store.UpdateCredential(c.CloudName, existingCredentials)
			if err == nil {
				fmt.Fprintf(ctxt.Stdout, "Credential%s %s locally for cloud %q.\n\n", msg, verb, c.CloudName)
			} else {
				fmt.Fprintf(ctxt.Stdout, "Credential%s not %v locally for cloud %q: %v\n\n", msg, verb, c.CloudName, err)
				returnErr = cmd.ErrSilent
			}
		}
	}
	if c.ControllerName != "" {
		// Remote processing.
		return c.addRemoteCredentials(ctxt, added, returnErr)
	}
	return returnErr
}

func (c *addCredentialCommand) existingCredentialsForCloud() (*jujucloud.CloudCredential, error) {
	existingCredentials, err := c.Store.CredentialForCloud(c.CloudName)
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

func (c *addCredentialCommand) interactiveAddCredential(ctxt *cmd.Context, schemas map[jujucloud.AuthType]jujucloud.CredentialSchema, existingCredentials *jujucloud.CloudCredential) error {
	errout := interact.NewErrWriter(ctxt.Stdout)
	pollster := interact.New(ctxt.Stdin, ctxt.Stdout, errout)

	credentialName, err := c.promptCredentialName(pollster, ctxt.Stdout)
	if err != nil {
		return err
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

	err = c.promptCloudRegion(pollster, existingCredentials, ctxt.Stdout)
	if err != nil {
		return errors.Trace(err)
	}
	authType, err := c.promptAuthType(pollster, authTypeNames, ctxt.Stdout)
	if err != nil {
		return errors.Trace(err)
	}
	schema, ok := schemas[authType]
	if !ok {
		return errors.NotSupportedf("auth type %q for cloud %q", authType, c.CloudName)
	}

	attrs, err := c.promptCredentialAttributes(pollster, schema)
	if err != nil {
		return errors.Trace(err)
	}

	newCredential, err := finalizeProvider(ctxt, c.cloud, c.Region, existingCredentials.DefaultRegion, authType, attrs)
	if err != nil {
		return errors.Trace(err)
	}

	existingCredentials.AuthCredentials[credentialName] = *newCredential
	return c.internalAddCredential(ctxt, verb, *existingCredentials, map[string]jujucloud.Credential{credentialName: *newCredential}, []string{credentialName}, nil)
}

func finalizeProvider(ctxt *cmd.Context, cloud *jujucloud.Cloud, regionName, defaultRegion string, authType jujucloud.AuthType, attrs map[string]string) (*jujucloud.Credential, error) {
	cloudEndpoint := cloud.Endpoint
	cloudStorageEndpoint := cloud.StorageEndpoint
	cloudIdentityEndpoint := cloud.IdentityEndpoint
	if len(cloud.Regions) > 0 {
		// For some providers we must have a region to construct a valid credential, for e.g. azure.
		// If a region was specified by the user, we'd use it;
		// otherwise, we'd use default region if one is set or, if not, the first region.
		if regionName == "" {
			regionName = defaultRegion
		}
		region := cloud.Regions[0]
		if regionName != "" {
			for _, r := range cloud.Regions {
				if r.Name == regionName {
					region = r
				}
			}
		}
		cloudEndpoint = region.Endpoint
		cloudStorageEndpoint = region.StorageEndpoint
		cloudIdentityEndpoint = region.IdentityEndpoint
	}

	credentialsProvider, err := environs.Provider(cloud.Type)
	if err != nil {
		return nil, errors.Trace(err)
	}
	newCredential, err := credentialsProvider.FinalizeCredential(
		ctxt, environs.FinalizeCredentialParams{
			Credential:            jujucloud.NewCredential(authType, attrs),
			CloudName:             cloud.Name,
			CloudEndpoint:         cloudEndpoint,
			CloudStorageEndpoint:  cloudStorageEndpoint,
			CloudIdentityEndpoint: cloudIdentityEndpoint,
		},
	)
	return newCredential, errors.Annotate(err, "finalizing credential")
}

func (c *addCredentialCommand) promptCredentialName(p *interact.Pollster, out io.Writer) (string, error) {
	credentialName, err := p.EnterVerify("credential name", func(value string) (ok bool, errmsg string, err error) {
		if !names.IsValidCloudCredentialName(value) {
			return false, fmt.Sprintf("Invalid credential name: %q", value), nil
		}

		return true, "", nil
	})
	if err != nil {
		return "", errors.Trace(err)
	}

	return credentialName, nil
}

func (c *addCredentialCommand) promptCloudRegion(p *interact.Pollster, existingCredentials *jujucloud.CloudCredential, out io.Writer) error {
	regions := c.cloud.Regions
	if len(regions) == 0 {
		return nil
	}
	if c.Region != "" {
		fmt.Fprintf(out, "User specified region %q, using it.\n\n", c.Region)
		return nil
	}
	choices := make([]string, len(regions))
	for i, r := range regions {
		choices[i] = r.Name
	}

	def := "any region, credential is not region specific"
	var err error
	c.Region, err = p.SelectVerify(interact.List{
		Singular: "region",
		Plural:   "regions",
		Options:  choices,
		Default:  def,
	}, func(value string) (ok bool, errmsg string, err error) {
		if value == "" {
			return true, "", nil
		}
		if regionErr := validCloudRegion(c.cloud, value); regionErr != nil {
			return false, regionErr.Error(), nil
		}
		return true, "", nil
	})
	if c.Region == def {
		c.Region = ""
	}
	return errors.Trace(err)
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

func (c *addCredentialCommand) promptCredentialAttributes(p *interact.Pollster, schema jujucloud.CredentialSchema) (attributes map[string]string, err error) {
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
	// Adds support for lp1988239. Order is important here!
	case attr.Hidden && attr.ExpandFilePath:
		return enterFile(name, attr.Description, p, true, attr.Optional)
	case attr.Hidden && attr.Optional && attr.ShortSuffix != "":
		return p.EnterPasswordWithSuffix(name, attr.ShortSuffix)
	case attr.Hidden:
		return p.EnterPassword(name)
	case attr.ExpandFilePath:
		return enterFile(name, attr.Description, p, true, attr.Optional)
	case attr.FilePath:
		return enterFile(name, attr.Description, p, false, attr.Optional)
	case attr.Optional && attr.ShortSuffix != "":
		return p.EnterWithSuffix(name, attr.ShortSuffix)
	case attr.Optional:
		return p.EnterOptional(name)
	default:
		return p.Enter(name)
	}
}

func (c *addCredentialCommand) credentialsAPI() (CredentialAPI, error) {
	root, err := c.NewAPIRoot(c.Store, c.ControllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apicloud.NewClient(root), nil
}

func (c *addCredentialCommand) maybeRemoteCloud(ctxt *cmd.Context) error {
	client, err := c.credentialAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()
	// Get user clouds from the controller
	remoteUserClouds, err := client.Clouds()
	if err != nil {
		return err
	}
	if remoteCloud, ok := remoteUserClouds[names.NewCloudTag(c.CloudName)]; ok {
		ctxt.Infof("Using cloud %q from the controller to verify credentials.", c.CloudName)
		c.cloud = &remoteCloud
		c.remoteCloudFound = true
	}
	return nil
}

func (c *addCredentialCommand) addRemoteCredentials(ctxt *cmd.Context, all map[string]jujucloud.Credential, localError error) error {
	if len(all) == 0 {
		fmt.Fprintf(ctxt.Stdout, "No credentials for cloud %q uploaded to controller %q.\n", c.CloudName, c.ControllerName)
		return localError
	}
	if !c.remoteCloudFound {
		fmt.Fprintf(ctxt.Stdout, "No cloud %q found on the controller %q: credentials are not uploaded.\n"+
			"Use 'juju clouds -c %v' to see what clouds are available on the controller.\n"+
			"User 'juju add-cloud %v -c %v' to add your cloud to the controller.\n",
			c.CloudName, c.ControllerName, c.ControllerName, c.CloudName, c.ControllerName)
		return localError
	}

	accountDetails, err := c.Store.AccountDetails(c.ControllerName)
	if err != nil {
		return err
	}
	verified, erred := verifyCredentialsForUpload(ctxt, accountDetails, c.cloud, c.Region, all)
	if len(verified) == 0 {
		return erred
	}
	client, err := c.credentialAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()
	results, err := client.AddCloudsCredentials(verified)
	if err != nil {
		logger.Errorf("%v", err)
		ctxt.Warningf("Could not upload credentials to controller %q", c.ControllerName)
	}
	return processUpdateCredentialResult(ctxt, accountDetails, "added", results, false, c.ControllerName, localError)
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
	contents, err := os.ReadFile(abs)
	return string(contents), errors.Trace(err)
}

func shouldFinalizeCredential(provider environs.EnvironProvider, cred jujucloud.Credential) bool {
	if finalizer, ok := provider.(environs.RequestFinalizeCredential); ok {
		return finalizer.ShouldFinalizeCredential(cred)
	}
	return false
}

func validCloudRegion(aCloud *jujucloud.Cloud, region string) error {
	for _, r := range aCloud.Regions {
		if r.Name == region {
			return nil
		}
	}
	return errors.NotValidf("provided region %q for cloud %q", region, aCloud.Name)
}
