// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"io/ioutil"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	apicloud "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
)

var usageUpdateCredentialSummary = `
Updates a controller credential for a cloud.`[1:]

var usageUpdateCredentialDetails = `
Cloud credentials for controller are used for model operations and manipulations.
Since it is common to have long-running models, it is also common to 
have these cloud credentials become invalid during models' lifetime.
When this happens, a user must update the cloud credential that 
a model was created with to the new and valid details on controller.

This command allows to update an existing, already-stored, named,
cloud-specific credential on controller or the one stored locally.

If --local is used, Juju updates credential in its internal cache directly but only on this client.
If a user will use a different client, say a different laptop, the update will not affect that 
client's copy. By extension, when using --local, remote credential copies,
on controllers, will not be affected.

Before credential is updated, the new content is validated. For some providers, 
cloud credentials are region specific. To validate the credential for a non-default region, 
use --region.

Examples:
    juju update-credential aws mysecrets
    juju update-credential -f mine.yaml
    juju update-credential aws -f mine.yaml
    juju update-credential azure --region brazilsouth -f mine.yaml

See also: 
    add-credential
    credentials`[1:]

type updateCredentialCommand struct {
	modelcmd.ControllerCommandBase

	api CredentialAPI

	cloud      string
	credential string

	// CredentialsFile is the name of the file that contains credentials to update.
	CredentialsFile string

	// Local determines if only local credentials are updated.
	Local bool

	// Region is the region that credentials will be validated for before an update.
	Region string
}

// NewUpdateCredentialCommand returns a command to update credential details.
func NewUpdateCredentialCommand() cmd.Command {
	command := updateCredentialCommand{}
	return modelcmd.WrapController(&command)
}

// Init implements Command.Init.
func (c *updateCredentialCommand) Init(args []string) error {
	argsCount := len(args)
	if argsCount == 0 {
		// We are either in the interactive mode or updating from a file.
		return nil
	}
	if argsCount > 2 {
		return errors.New("only a cloud name and / or credential name need to be provided")
	}
	if argsCount >= 1 {
		c.cloud = args[0]
	}
	if argsCount >= 2 {
		c.credential = args[1]
	}
	return nil
}

// Info implements Command.Info
func (c *updateCredentialCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "update-credential",
		Args:    "[<cloud-name> [<credential-name>]]",
		Purpose: usageUpdateCredentialSummary,
		Aliases: []string{"update-credentials"},
		Doc:     usageUpdateCredentialDetails,
	})
}

// SetFlags implements Command.SetFlags.
func (c *updateCredentialCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	f.StringVar(&c.CredentialsFile, "f", "", "The YAML file containing credential details to update")
	f.StringVar(&c.CredentialsFile, "file", "", "The YAML file containing credential details to update")
	f.BoolVar(&c.Local, "local", false, "Local operation only; controller not affected")
	f.StringVar(&c.Region, "region", "", "Cloud region that credential is valid for")
}

type CredentialAPI interface {
	Clouds() (map[names.CloudTag]jujucloud.Cloud, error)
	UpdateCloudsCredentials(cloudCredentials map[string]jujucloud.Credential) ([]params.UpdateCredentialResult, error)
	BestAPIVersion() int
	Close() error
}

func (c *updateCredentialCommand) getAPI() (CredentialAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	return apicloud.NewClient(api), nil
}

// Run implements Command.Run
func (c *updateCredentialCommand) Run(ctx *cmd.Context) error {
	// If no file and no cloud is provided, switch to interactive mode.
	if c.CredentialsFile == "" && c.cloud == "" {
		// TODO (anastasiamac 2019-03-22) interactive mode
		return errors.New("Usage: juju update-credential [options] [<cloud-name> [<credential-name>]]")
	}
	var credentials map[string]jujucloud.CloudCredential
	var err error
	if c.CredentialsFile != "" {
		credentials, err = credentialsFromFile(c.CredentialsFile, c.cloud, c.credential)
		if err != nil {
			return errors.Annotatef(err, "could not get credentials from file")
		}
	} else {
		credentials, err = credentialsFromLocalCache(c.ClientStore(), c.cloud, c.credential)
		if err != nil {
			return errors.Annotatef(err, "could not get credentials from local client cache")
		}
	}
	if c.Local {
		return c.updateLocalCredentials(ctx, credentials)
	}
	return c.updateRemoteCredentials(ctx, credentials)
}

func credentialsFromFile(credentialsFile, cloudName, credentialName string) (map[string]jujucloud.CloudCredential, error) {
	data, err := ioutil.ReadFile(credentialsFile)
	if err != nil {
		return nil, errors.Annotate(err, "reading credentials file")
	}
	specifiedCredentials, err := jujucloud.ParseCredentials(data)
	if err != nil {
		return nil, errors.Annotate(err, "parsing credentials file")
	}

	if cloudName == "" && credentialName == "" {
		return specifiedCredentials, nil
	}

	filteredByCloud := map[string]jujucloud.CloudCredential{}
	if cloudName != "" {
		cloudCredentials, ok := specifiedCredentials[cloudName]
		if !ok {
			return nil, errors.NotFoundf("credentials for cloud %q in file %q", cloudName, credentialsFile)
		}
		filteredByCloud[cloudName] = cloudCredentials
	} else {
		filteredByCloud = specifiedCredentials
	}
	if credentialName == "" {
		return filteredByCloud, nil
	}

	filteredByName := map[string]jujucloud.CloudCredential{}
	for aCloud, cloudCredentials := range filteredByCloud {
		for name, aCredential := range cloudCredentials.AuthCredentials {
			if name == credentialName {
				filteredByName[aCloud] = jujucloud.CloudCredential{
					AuthCredentials: map[string]jujucloud.Credential{name: aCredential},
					DefaultRegion:   cloudCredentials.DefaultRegion,
				}
			}
		}
	}

	if len(filteredByName) == 0 {
		return nil, errors.NotFoundf("credential %q for cloud %q in file %s", credentialName, cloudName, credentialsFile)
	}
	return filteredByName, nil
}

func credentialsFromLocalCache(store jujuclient.ClientStore, cloudName, credentialName string) (map[string]jujucloud.CloudCredential, error) {
	all := map[string]jujucloud.CloudCredential{}
	var err error
	if cloudName == "" {
		all, err = store.AllCredentials()
		if err != nil {
			return nil, errors.Annotate(err, "loading credentials")
		}
	} else {
		var cloudCredentials *jujucloud.CloudCredential
		cloudCredentials, err = store.CredentialForCloud(cloudName)
		if err != nil {
			return nil, errors.Annotate(err, "loading credentials")
		}
		all[cloudName] = *cloudCredentials
	}
	if credentialName == "" {
		return all, nil
	}
	found := map[string]jujucloud.CloudCredential{}
	for cloudName, cloudCredentials := range all {
		for name, aCredential := range cloudCredentials.AuthCredentials {
			if name == credentialName {
				found[cloudName] = jujucloud.CloudCredential{
					AuthCredentials: map[string]jujucloud.Credential{name: aCredential},
					DefaultRegion:   cloudCredentials.DefaultRegion,
				}
				return found, nil
			}
		}
	}
	return nil, errors.NotFoundf("credential %q for cloud %q in local client cache", credentialName, cloudName)
}

func (c *updateCredentialCommand) updateLocalCredentials(ctx *cmd.Context, update map[string]jujucloud.CloudCredential) error {
	erred := false
	for cloudName, cloudCredentials := range update {
		aCloud, err := common.CloudByName(cloudName)
		if errors.IsNotFound(err) {
			ctx.Infof("Cloud %q not found.", cloudName)
			erred = true
			continue
		} else if err != nil {
			logger.Errorf("%v", err)
			ctx.Warningf("Could not verify cloud %v.", cloudName)
			erred = true
			continue
		}
		storedCredentials, err := c.ClientStore().CredentialForCloud(cloudName)
		if errors.IsNotFound(err) {
			ctx.Warningf("Could not find local credentials for cloud %v.", cloudName)
			ctx.Infof("Use `juju add-credential` to add credentials locally.")
			erred = true
			continue
		} else if err != nil {
			logger.Errorf("%v", err)
			ctx.Warningf("Could not get local credentials for cloud %v.", cloudName)
			erred = true
			continue
		}

		if c.Region != "" {
			if err := validCloudRegion(aCloud, c.Region); err != nil {
				logger.Errorf("%v", err)
				ctx.Warningf("Region %q is not valid for cloud %v.", c.Region, cloudName)
				erred = true
				continue
			}
		}
		provider, err := environs.Provider(aCloud.Type)
		if err != nil {
			return errors.Trace(err)
		}
		for credentialName, credential := range cloudCredentials.AuthCredentials {
			if shouldFinalizeCredential(provider, credential) {
				newCredential, err := finalizeProvider(ctx, aCloud, c.Region, cloudCredentials.DefaultRegion, credential.AuthType(), credential.Attributes())
				if err != nil {
					logger.Errorf("%v", err)
					logger.Warningf("Could not verify credential %v for cloud %v locally", credentialName, aCloud.Name)
					erred = true
					continue
				}
				credential = *newCredential
			}
			storedCredentials.AuthCredentials[credentialName] = credential
		}
		err = c.ClientStore().UpdateCredential(cloudName, *storedCredentials)
		if err != nil {
			logger.Errorf("%v", err)
			ctx.Warningf("Could not update local client store with credentials for cloud %v", cloudName)
			erred = true
		}
	}
	if erred {
		return cmd.ErrSilent
	}
	ctx.Infof(`Local client was updated successfully with provided credential information.`)
	return nil
}

func (c *updateCredentialCommand) updateRemoteCredentials(ctx *cmd.Context, update map[string]jujucloud.CloudCredential) error {
	accountDetails, err := c.CurrentAccountDetails()
	if err != nil {
		return err
	}
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	// Get user clouds from the controller
	remoteUserClouds, err := client.Clouds()
	if err != nil {
		return err
	}
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}

	var erred error
	verified := map[string]jujucloud.Credential{}
	mapUnion := func(items map[string]jujucloud.Credential) {
		for k, v := range items {
			verified[k] = v
		}
	}
	for cloudName, cloudCredentials := range update {
		remoteCloud, ok := remoteUserClouds[names.NewCloudTag(cloudName)]
		if !ok {
			ctx.Warningf("No cloud %q available to user %q remotely on controller %q", cloudName, accountDetails.User, controllerName)
			erred = cmd.ErrSilent
			continue
		}
		region := cloudCredentials.DefaultRegion
		if c.Region != "" {
			region = c.Region
		}
		newlyVerified, err := verifyCredentialsForUpload(ctx, accountDetails, &remoteCloud, region, cloudCredentials.AuthCredentials)
		mapUnion(newlyVerified)
		if err != nil {
			erred = err
		}
	}

	if len(verified) == 0 {
		return erred
	}
	results, err := client.UpdateCloudsCredentials(verified)
	if err != nil {
		logger.Errorf("%v", err)
		ctx.Warningf("Could not update credentials remotely, on controller %q", controllerName)
		erred = cmd.ErrSilent
	}
	if err := processUpdateCredentialResult(ctx, accountDetails, "updated", results); err != nil {
		return err
	}
	return erred
}

func verifyCredentialsForUpload(ctx *cmd.Context, accountDetails *jujuclient.AccountDetails, aCloud *jujucloud.Cloud, region string, all map[string]jujucloud.Credential) (map[string]jujucloud.Credential, error) {
	verified := map[string]jujucloud.Credential{}
	var erred error
	for credentialName, aCredential := range all {
		id := fmt.Sprintf("%s/%s/%s", aCloud.Name, accountDetails.User, credentialName)
		if !names.IsValidCloudCredential(id) {
			ctx.Warningf("Could not update controller credential %v for user %v on cloud %v: %v", credentialName, accountDetails.User, aCloud.Name, errors.NotValidf("cloud credential ID %q", id))
			erred = cmd.ErrSilent
			continue
		}
		verifiedCredential, err := modelcmd.VerifyCredentials(ctx, aCloud, &aCredential, credentialName, region)
		if err != nil {
			logger.Errorf("%v", err)
			ctx.Warningf("Could not verify credential %v for cloud %v locally", credentialName, aCloud.Name)
			erred = cmd.ErrSilent
			continue
		}
		verified[names.NewCloudCredentialTag(id).String()] = *verifiedCredential
	}
	return verified, erred
}

func processUpdateCredentialResult(ctx *cmd.Context, accountDetails *jujuclient.AccountDetails, op string, results []params.UpdateCredentialResult) error {
	var erred error
	for _, result := range results {
		tag, err := names.ParseCloudCredentialTag(result.CredentialTag)
		if err != nil {
			logger.Errorf("%v", err)
			ctx.Warningf("Could not parse credential tag %q", result.CredentialTag)
			erred = cmd.ErrSilent
		}
		// We always want to display models information if there is any.
		common.OutputUpdateCredentialModelResult(ctx, result.Models, true)
		if result.Error != nil {
			ctx.Warningf("Controller credential %q for user %q on cloud %q not %v: %v.", tag.Name(), accountDetails.User, tag.Cloud().Id(), op, result.Error)
			if len(result.Models) != 0 {
				ctx.Infof("Failed models may require a different credential.")
				ctx.Infof("Use ‘juju set-credential’ to change credential for these models before repeating this update.")
			}
			// We do not want to return err here as we have already displayed it on the console.
			erred = cmd.ErrSilent
			continue
		}
		ctx.Infof(`
Controller credential %q for user %q on cloud %q %v.
For more information, see ‘juju show-credential %v %v’.`[1:],
			tag.Name(), accountDetails.User, tag.Cloud().Id(), op,
			tag.Cloud().Id(), tag.Name())
	}
	return erred
}
