// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
)

// NewAddModelCommand returns a command to add a model.
func NewAddModelCommand() cmd.Command {
	return modelcmd.WrapController(&addModelCommand{
		newAddModelAPI: func(caller base.APICallCloser) AddModelAPI {
			return modelmanager.NewClient(caller)
		},
		newCloudAPI: func(caller base.APICallCloser) CloudAPI {
			return cloudapi.NewClient(caller)
		},
		providerRegistry: environs.GlobalProviderRegistry(),
	})
}

// addModelCommand calls the API to add a new model.
type addModelCommand struct {
	modelcmd.ControllerCommandBase
	apiRoot          api.Connection
	newAddModelAPI   func(base.APICallCloser) AddModelAPI
	newCloudAPI      func(base.APICallCloser) CloudAPI
	providerRegistry environs.ProviderRegistry

	Name           string
	Owner          string
	CredentialName string
	CloudRegion    string
	Config         common.ConfigFlag
	noSwitch       bool
}

const addModelHelpDoc = `
Adding a model is typically done in order to run a specific workload.

To add a model, you must specify a model name. Model names can be duplicated 
across controllers but must be unique per user for any given controller. 
In other words, Alice and Bob can each have their own model called "secret" but 
Alice can have only one model called "secret" in a controller.
Model names may only contain lowercase letters, digits and hyphens, and 
may not start with a hyphen.

To add a model, Juju requires a credential:

    * if you have a default (or just one) credential defined at client
     (i.e. in credentials.yaml), then juju will use that;
    * if you have no default (and multiple) credentials defined at the client, 
     then you must specify one using --credential;
    * as the admin user you can omit the credential, 
     and the credential used to bootstrap will be used.

To add a credential for add-model, use one of the "juju add-credential" or 
"juju autoload-credentials" commands. These will add credentials 
to the Juju client, which "juju add-model" will upload to the controller 
as necessary.

You may also supply model-specific configuration as well as a
cloud/region to which this model will be deployed. The cloud/region and credentials
are the ones used to create any future resources within the model.

If no cloud/region is specified, then the model will be deployed to
the same cloud/region as the controller model. If a region is specified
without a cloud qualifier, then it is assumed to be in the same cloud
as the controller model. It is not currently possible for a controller
to manage multiple clouds, so the only valid cloud is the same cloud
as the controller model is deployed to. This may change in a future
release.

Examples:

    juju add-model mymodel
    juju add-model mymodel us-east-1
    juju add-model mymodel aws/us-east-1
    juju add-model mymodel --config my-config.yaml --config image-stream=daily
    juju add-model mymodel --credential credential_name --config authorized-keys="ssh-rsa ..."
`

func (c *addModelCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-model",
		Args:    "<model name> [cloud|region|(cloud/region)]",
		Purpose: "Adds a hosted model.",
		Doc:     strings.TrimSpace(addModelHelpDoc),
	}
}

func (c *addModelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	f.StringVar(&c.Owner, "owner", "", "The owner of the new model if not the current user")
	f.StringVar(&c.CredentialName, "credential", "", "Credential used to add the model")
	f.Var(&c.Config, "config", "Path to YAML model configuration file or individual options (--config config.yaml [--config key=value ...])")
	f.BoolVar(&c.noSwitch, "no-switch", false, "Do not switch to the newly created model")
}

func (c *addModelCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("model name is required")
	}
	c.Name, args = args[0], args[1:]

	if len(args) > 0 {
		c.CloudRegion, args = args[0], args[1:]
	}

	if !names.IsValidModelName(c.Name) {
		return errors.Errorf("%q is not a valid name: model names may only contain lowercase letters, digits and hyphens", c.Name)
	}

	if c.Owner != "" && !names.IsValidUser(c.Owner) {
		return errors.Errorf("%q is not a valid user", c.Owner)
	}

	return cmd.CheckEmpty(args)
}

type AddModelAPI interface {
	CreateModel(
		name, owner, cloudName, cloudRegion string,
		cloudCredential names.CloudCredentialTag,
		config map[string]interface{},
	) (base.ModelInfo, error)
}

type CloudAPI interface {
	DefaultCloud() (names.CloudTag, error)
	Clouds() (map[names.CloudTag]jujucloud.Cloud, error)
	Cloud(names.CloudTag) (jujucloud.Cloud, error)
	UserCredentials(names.UserTag, names.CloudTag) ([]names.CloudCredentialTag, error)
	UpdateCredentialsCheckModels(tag names.CloudCredentialTag, credential jujucloud.Credential) ([]params.UpdateCredentialModelResult, error)
}

func (c *addModelCommand) newAPIRoot() (api.Connection, error) {
	if c.apiRoot != nil {
		return c.apiRoot, nil
	}
	return c.NewAPIRoot()
}

func (c *addModelCommand) Run(ctx *cmd.Context) error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	api, err := c.newAPIRoot()
	if err != nil {
		return errors.Annotate(err, "opening API connection")
	}
	defer api.Close()

	store := c.ClientStore()
	accountDetails, err := store.AccountDetails(controllerName)
	if err != nil {
		return errors.Trace(err)
	}

	modelOwner := accountDetails.User
	if c.Owner != "" {
		if !names.IsValidUser(c.Owner) {
			return errors.Errorf("%q is not a valid user name", c.Owner)
		}
		modelOwner = names.NewUserTag(c.Owner).Id()
	}
	forUserSuffix := fmt.Sprintf(" for user '%s'", names.NewUserTag(modelOwner).Name())

	attrs, err := c.getConfigValues(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	cloudClient := c.newCloudAPI(api)
	var cloudTag names.CloudTag
	var cloud jujucloud.Cloud
	var cloudRegion string
	if c.CloudRegion != "" {
		cloudTag, cloud, cloudRegion, err = c.getCloudRegion(cloudClient)
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		if cloudTag, cloud, err = defaultCloud(cloudClient); err != nil {
			return errors.Trace(err)
		}
	}

	// Find a credential to use with the new model.
	credential, credentialTag, cloudRegion, err := c.findCredential(ctx, cloudClient, &findCredentialParams{
		cloudTag:    cloudTag,
		cloudRegion: cloudRegion,
		cloud:       cloud,
		modelOwner:  modelOwner,
	})
	if err != nil {
		return errors.Trace(err)
	}

	// Upload the credential if it was explicitly set and we have found it locally.
	if c.CredentialName != "" && credential != nil {
		ctx.Infof("Uploading credential '%s' to controller", credentialTag.Id())
		if all, err := cloudClient.UpdateCredentialsCheckModels(credentialTag, *credential); err != nil {
			ctx.Infof("Failed to upload credential: %v", err)
			common.OutputUpdateCredentialModelResult(ctx, all, false)
			return cmd.ErrSilent
		}
	}

	addModelClient := c.newAddModelAPI(api)
	model, err := addModelClient.CreateModel(c.Name, modelOwner, cloudTag.Id(), cloudRegion, credentialTag, attrs)
	if err != nil {
		if params.IsCodeUnauthorized(err) {
			common.PermissionsMessage(ctx.Stderr, "add a model")
		}
		return errors.Trace(err)
	}

	messageFormat := "Added '%s' model"
	messageArgs := []interface{}{c.Name}

	if modelOwner == accountDetails.User {
		if err := store.UpdateModel(controllerName, c.Name, jujuclient.ModelDetails{
			ModelUUID: model.UUID,
			ModelType: model.Type,
		}); err != nil {
			return errors.Trace(err)
		}
		if !c.noSwitch {
			if err := store.SetCurrentModel(controllerName, c.Name); err != nil {
				return errors.Trace(err)
			}
		}
	}

	if c.CloudRegion != "" || model.CloudRegion != "" {
		// The user explicitly requested a cloud/region,
		// or the cloud supports multiple regions. Whichever
		// the case, tell the user which cloud/region the
		// model was deployed to.
		cloudRegion := model.Cloud
		if model.CloudRegion != "" {
			cloudRegion += "/" + model.CloudRegion
		}
		messageFormat += " on %s"
		messageArgs = append(messageArgs, cloudRegion)
	}
	if model.CloudCredential != "" {
		tag := names.NewCloudCredentialTag(model.CloudCredential)
		credentialName := tag.Name()
		if tag.Owner().Id() != modelOwner {
			credentialName = fmt.Sprintf("%s/%s", tag.Owner().Id(), credentialName)
		}
		messageFormat += " with credential '%s'"
		messageArgs = append(messageArgs, credentialName)
	}

	messageFormat += forUserSuffix

	// "Added '<model>' model [on <cloud>/<region>] [with credential '<credential>'] for user '<user namePart>'"
	ctx.Infof(messageFormat, messageArgs...)

	if _, ok := attrs[config.AuthorizedKeysKey]; !ok {
		// It is not an error to have no authorized-keys when adding a
		// model, though this should never happen since we generate
		// juju-specific SSH keys.
		ctx.Infof(`
No SSH authorized-keys were found. You must use "juju add-ssh-key"
before "juju ssh", "juju scp", or "juju debug-hooks" will work.`)
	}

	return nil
}

func (c *addModelCommand) getCloudRegion(cloudClient CloudAPI) (cloudTag names.CloudTag, cloud jujucloud.Cloud, cloudRegion string, err error) {
	var cloudName string
	sep := strings.IndexRune(c.CloudRegion, '/')
	if sep >= 0 {
		// User specified "cloud/region".
		cloudName, cloudRegion = c.CloudRegion[:sep], c.CloudRegion[sep+1:]
		if !names.IsValidCloud(cloudName) {
			return names.CloudTag{}, jujucloud.Cloud{}, "", errors.NotValidf("cloud name %q", cloudName)
		}
		cloudTag = names.NewCloudTag(cloudName)
		if cloud, err = cloudClient.Cloud(cloudTag); err != nil {
			return names.CloudTag{}, jujucloud.Cloud{}, "", errors.Trace(err)
		}
	} else {
		// User specified "cloud" or "region". We'll try first
		// for cloud (check if it's a valid cloud name, and
		// whether there is a cloud by that name), and then
		// as a region within the default cloud.
		if names.IsValidCloud(c.CloudRegion) {
			cloudName = c.CloudRegion
		} else {
			cloudRegion = c.CloudRegion
		}
		if cloudName != "" {
			cloudTag = names.NewCloudTag(cloudName)
			cloud, err = cloudClient.Cloud(cloudTag)
			if params.IsCodeNotFound(err) {
				// No such cloud with the specified name,
				// so we'll try the name as a region in
				// the default cloud.
				cloudRegion, cloudName = cloudName, ""
			} else if err != nil {
				return names.CloudTag{}, jujucloud.Cloud{}, "", errors.Trace(err)
			}
		}
		if cloudName == "" {
			cloudTag, cloud, err = defaultCloud(cloudClient)
			if err != nil && !errors.IsNotFound(err) {
				return names.CloudTag{}, jujucloud.Cloud{}, "", errors.Trace(err)
			}
		}
	}
	if cloudRegion != "" {
		// A region has been specified, make sure it exists.
		if _, err := jujucloud.RegionByName(cloud.Regions, cloudRegion); err != nil {
			if cloudRegion == c.CloudRegion {
				// The string is not in the format cloud/region,
				// so we should tell that the user that it is
				// neither a cloud nor a region in the default
				// cloud (or that there is no default cloud).
				err := c.unsupportedCloudOrRegionError(cloudClient, cloudTag)
				return names.CloudTag{}, jujucloud.Cloud{}, "", errors.Trace(err)
			}
			return names.CloudTag{}, jujucloud.Cloud{}, "", errors.Trace(err)
		}
	}
	return cloudTag, cloud, cloudRegion, nil
}

func (c *addModelCommand) unsupportedCloudOrRegionError(cloudClient CloudAPI, defaultCloudTag names.CloudTag) (err error) {
	clouds, err := cloudClient.Clouds()
	if err != nil {
		return errors.Annotate(err, "querying supported clouds")
	}
	cloudNames := make([]string, 0, len(clouds))
	for tag := range clouds {
		cloudNames = append(cloudNames, tag.Id())
	}
	sort.Strings(cloudNames)

	var buf bytes.Buffer
	tw := output.TabWriter(&buf)
	fmt.Fprintln(tw, "Cloud\tRegions")
	for _, cloudName := range cloudNames {
		cloud := clouds[names.NewCloudTag(cloudName)]
		regionNames := make([]string, len(cloud.Regions))
		for i, region := range cloud.Regions {
			regionNames[i] = region.Name
		}
		fmt.Fprintf(tw, "%s\t%s\n", cloudName, strings.Join(regionNames, ", "))
	}
	tw.Flush()

	var prefix string
	if defaultCloudTag != (names.CloudTag{}) {
		prefix = fmt.Sprintf(`
%q is neither a cloud supported by this controller,
nor a region in the controller's default cloud %q.
The clouds/regions supported by this controller are:`[1:],
			c.CloudRegion, defaultCloudTag.Id())
	} else {
		prefix = fmt.Sprintf(`
%q is not a cloud supported by this controller,
and there is no default cloud. The clouds/regions supported
by this controller are:`[1:], c.CloudRegion)
	}
	return errors.Errorf("%s\n\n%s", prefix, buf.String())
}

func defaultCloud(cloudClient CloudAPI) (names.CloudTag, jujucloud.Cloud, error) {
	cloudTag, err := cloudClient.DefaultCloud()
	if err != nil {
		if params.IsCodeNotFound(err) {
			return names.CloudTag{}, jujucloud.Cloud{}, errors.NewNotFound(nil, `
there is no default cloud defined, please specify one using:

    juju add-model [flags] <model-name> cloud[/region]`[1:])
		}
		return names.CloudTag{}, jujucloud.Cloud{}, errors.Trace(err)
	}
	cloud, err := cloudClient.Cloud(cloudTag)
	if err != nil {
		return names.CloudTag{}, jujucloud.Cloud{}, errors.Trace(err)
	}
	return cloudTag, cloud, nil
}

var ambiguousDetectedCredentialError = errors.New(`
more than one credential detected. Add all detected credentials
to the client with:

    juju autoload-credentials

and then run the add-model command again with the --credential flag.`[1:],
)

var ambiguousCredentialError = errors.New(`
more than one credential is available. List credentials with:

    juju credentials

and then run the add-model command again with the --credential flag.`[1:],
)

type findCredentialParams struct {
	cloudTag    names.CloudTag
	cloud       jujucloud.Cloud
	cloudRegion string
	modelOwner  string
}

// findCredential finds a suitable credential to use for the new model.
// The credential will first be searched for locally and then on the
// controller. If a credential is found locally then it's value will be
// returned as the first return value. If it is found on the controller
// this will be nil as there is no need to upload it in that case.
func (c *addModelCommand) findCredential(ctx *cmd.Context, cloudClient CloudAPI, p *findCredentialParams) (_ *jujucloud.Credential, _ names.CloudCredentialTag, cloudRegion string, _ error) {
	if c.CredentialName == "" {
		return c.findUnspecifiedCredential(ctx, cloudClient, p)
	}
	return c.findSpecifiedCredential(ctx, cloudClient, p)
}

func (c *addModelCommand) findUnspecifiedCredential(ctx *cmd.Context, cloudClient CloudAPI, p *findCredentialParams) (_ *jujucloud.Credential, _ names.CloudCredentialTag, cloudRegion string, _ error) {
	fail := func(err error) (*jujucloud.Credential, names.CloudCredentialTag, string, error) {
		return nil, names.CloudCredentialTag{}, "", err
	}
	// If the user has not specified a credential, and the cloud advertises
	// itself as supporting the "empty" auth-type, then return immediately.
	for _, authType := range p.cloud.AuthTypes {
		if authType == jujucloud.EmptyAuthType {
			return nil, names.CloudCredentialTag{}, p.cloudRegion, nil
		}
	}

	// No credential has been specified, so see if there is one already on the controller we can use.
	modelOwnerTag := names.NewUserTag(p.modelOwner)
	credentialTags, err := cloudClient.UserCredentials(modelOwnerTag, p.cloudTag)
	if err != nil {
		return fail(errors.Trace(err))
	}
	var credentialTag names.CloudCredentialTag
	if len(credentialTags) == 1 {
		credentialTag = credentialTags[0]
	}

	if (credentialTag != names.CloudCredentialTag{}) {
		// If the controller already has a credential, see if
		// there is a local version that has an associated
		// region.
		credential, _, cloudRegion, err := c.findLocalCredential(ctx, p, credentialTag.Name())
		if errors.IsNotFound(err) {
			// No local credential; use the region
			// specified by the user, if any.
			cloudRegion = p.cloudRegion
		} else if err != nil {
			return fail(errors.Trace(err))
		}
		// If there is a credential in the controller use it even if we don't have a local version.
		return credential, credentialTag, cloudRegion, nil
	}
	// There is not a default credential on the controller (either
	// there are no credentials, or there is more than one). Look for
	// a local credential we might use.
	credential, credentialName, cloudRegion, err := c.findLocalCredential(ctx, p, "")
	if err != nil {
		return fail(errors.Trace(err))
	}
	// We've got a local credential to use.
	credentialTag, err = common.ResolveCloudCredentialTag(
		modelOwnerTag, p.cloudTag, credentialName,
	)
	if err != nil {
		return fail(errors.Trace(err))
	}
	return credential, credentialTag, cloudRegion, nil
}

func (c *addModelCommand) findSpecifiedCredential(ctx *cmd.Context, cloudClient CloudAPI, p *findCredentialParams) (_ *jujucloud.Credential, _ names.CloudCredentialTag, cloudRegion string, _ error) {
	fail := func(err error) (*jujucloud.Credential, names.CloudCredentialTag, string, error) {
		return nil, names.CloudCredentialTag{}, "", err
	}
	// Look for a local credential with the specified name
	credential, credentialName, cloudRegion, err := c.findLocalCredential(ctx, p, c.CredentialName)
	if err != nil && !errors.IsNotFound(err) {
		return fail(errors.Trace(err))
	}
	if credential != nil {
		// We found a local credential with the specified name.
		modelOwnerTag := names.NewUserTag(p.modelOwner)
		credentialTag, err := common.ResolveCloudCredentialTag(
			modelOwnerTag, p.cloudTag, credentialName,
		)
		if err != nil {
			return fail(errors.Trace(err))
		}
		return credential, credentialTag, cloudRegion, nil
	}

	// There was no local credential with that name, check the controller
	modelOwnerTag := names.NewUserTag(p.modelOwner)
	credentialTags, err := cloudClient.UserCredentials(modelOwnerTag, p.cloudTag)
	if err != nil {
		return fail(errors.Trace(err))
	}
	credentialTag, err := common.ResolveCloudCredentialTag(
		modelOwnerTag, p.cloudTag, c.CredentialName,
	)
	if err != nil {
		return fail(errors.Trace(err))
	}
	credentialId := credentialTag.Id()
	for _, tag := range credentialTags {
		if tag.Id() != credentialId {
			continue
		}
		ctx.Infof("Using credential '%s' cached in controller", c.CredentialName)
		return nil, credentialTag, "", nil
	}
	// Cannot find a credential with the correct name
	return fail(errors.NotFoundf("credential '%s'", c.CredentialName))
}

func (c *addModelCommand) findLocalCredential(ctx *cmd.Context, p *findCredentialParams, name string) (_ *jujucloud.Credential, credentialName, cloudRegion string, _ error) {
	fail := func(err error) (*jujucloud.Credential, string, string, error) {
		return nil, "", "", err
	}
	provider, err := c.providerRegistry.Provider(p.cloud.Type)
	if err != nil {
		return fail(errors.Trace(err))
	}
	credential, credentialName, cloudRegion, _, err := common.GetOrDetectCredential(
		ctx, c.ClientStore(), provider, modelcmd.GetCredentialsParams{
			Cloud:          p.cloud,
			CloudRegion:    p.cloudRegion,
			CredentialName: name,
		},
	)
	if err == nil {
		return credential, credentialName, cloudRegion, nil
	}
	switch errors.Cause(err) {
	case modelcmd.ErrMultipleCredentials:
		return fail(ambiguousCredentialError)
	case common.ErrMultipleDetectedCredentials:
		return fail(ambiguousDetectedCredentialError)
	}
	return fail(errors.Trace(err))
}

func (c *addModelCommand) getConfigValues(ctx *cmd.Context) (map[string]interface{}, error) {
	configValues, err := c.Config.ReadAttrs(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "unable to parse config")
	}
	coercedValues, err := common.ConformYAML(configValues)
	if err != nil {
		return nil, errors.Annotatef(err, "unable to parse config")
	}
	attrs, ok := coercedValues.(map[string]interface{})
	if !ok {
		return nil, errors.New("params must contain a YAML map with string keys")
	}
	if err := common.FinalizeAuthorizedKeys(ctx, attrs); err != nil {
		if errors.Cause(err) != common.ErrNoAuthorizedKeys {
			return nil, errors.Trace(err)
		}
	}
	return attrs, nil
}
