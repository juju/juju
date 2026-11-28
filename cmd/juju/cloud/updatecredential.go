// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"context"
	"fmt"
	"maps"
	"os"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	apicloud "github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/api/jujuclient"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/rpc/params"
)

var usageUpdateCredentialSummary = `
Updates cloud credentials on this client or on a controller.`[1:]

var usageUpdateCredentialDetails = `
Read replacement material from the client cache or a credentials YAML
file supplied with `[1:] + "`--file`" + `. Supply a cloud name when reading the
cache. A cloud name selects that cloud's entries. Supply a credential
name as well to select one entry. With a file, you can omit both names
to select entries across the file.

Use ` + "`--client`" + ` for this client, ` + "`--controller NAME`" + ` for the
signed-in account on that controller, or both flags for both stores. A
local update requires existing local credentials for the cloud. Use
` + "`juju add-credential`" + ` to add the first local entry.

Use ` + "`--region`" + ` to select the region for credential preparation.
A controller update replaces the material used by all models bound to
that credential. Verify the cloud operation those models need. To
switch one model, use ` + "`juju set-credential`" + `.

Only a controller superuser can force a controller update. After a
forced update, inspect the stored credential before relying on it.
Check the result for each credential before retrying.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/credential/


`

const usageUpdateCredentialExamples = `
Upload the cached material for one credential:

    juju update-credential aws production --controller mycontroller

Update the local AWS entry named production where AWS already has local credentials:

    juju update-credential aws production --file mine.yaml --client

Update the controller credentials for AWS from a file:

    juju update-credential aws --file mine.yaml --controller mycontroller

Update the Azure entry named production using the Brazil South region for preparation:

    juju update-credential azure production --region brazilsouth --file mine.yaml --controller mycontroller
`

type updateCredentialCommand struct {
	modelcmd.OptionalControllerCommand

	updateCredentialAPIFunc func(ctx context.Context) (CredentialAPI, error)

	cloud      string
	credential string

	// CredentialsFile is the name of the file that contains credentials to update.
	CredentialsFile string

	// Region is the region that credentials will be validated for before an update.
	Region string

	// Force determines whether the update will be forced on the controller side.
	Force bool
}

// NewUpdateCredentialCommand returns a command to update credential details.
func NewUpdateCredentialCommand() cmd.Command {
	store := jujuclient.NewFileClientStore()
	c := &updateCredentialCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store: store,
		},
	}
	c.updateCredentialAPIFunc = c.getAPI
	return modelcmd.WrapBase(c)
}

// Init implements Command.Init.
func (c *updateCredentialCommand) Init(args []string) error {
	if err := c.OptionalControllerCommand.Init(args); err != nil {
		return err
	}
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
		Name:     "update-credential",
		Args:     "[<cloud-name> [<credential-name>]]",
		Purpose:  usageUpdateCredentialSummary,
		Aliases:  []string{"update-credentials"},
		Doc:      usageUpdateCredentialDetails,
		Examples: usageUpdateCredentialExamples,
		SeeAlso: []string{
			"add-credential",
			"credentials",
			"remove-credential",
			"set-credential",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *updateCredentialCommand) SetFlags(f *gnuflag.FlagSet) {
	c.OptionalControllerCommand.SetFlags(f)
	f.StringVar(&c.CredentialsFile, "f", "", "The YAML file containing credential details to update")
	f.StringVar(&c.CredentialsFile, "file", "", "The YAML file containing credential details to update")
	f.StringVar(&c.Region, "region", "", "Cloud region to use when preparing the credential")
	f.BoolVar(&c.Force, "force", false, "Request a forced controller update (controller superuser only)")
}

type CredentialAPI interface {
	Clouds(ctx context.Context) (map[names.CloudTag]jujucloud.Cloud, error)
	AddCloudsCredentials(ctx context.Context, cloudCredentials map[string]jujucloud.Credential) ([]params.UpdateCredentialResult, error)
	UpdateCloudsCredentials(ctx context.Context, cloudCredentials map[string]jujucloud.Credential, force bool) ([]params.UpdateCredentialResult, error)
	Close() error
}

func (c *updateCredentialCommand) getAPI(ctx context.Context) (CredentialAPI, error) {
	root, err := c.NewAPIRoot(ctx, c.Store, c.ControllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apicloud.NewClient(root), nil
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
		credentials, err = credentialsFromLocalCache(c.Store, c.cloud, c.credential)
		if err != nil {
			return errors.Annotatef(err, "could not get credentials from local client")
		}
	}
	if err := c.MaybePrompt(ctx, fmt.Sprintf("update credential %q on cloud %q on", c.credential, c.cloud)); err != nil {
		return errors.Trace(err)
	}
	var returnErr error
	if c.Client {
		if err := c.updateLocalCredentials(ctx, credentials); err != nil {
			returnErr = err
		}
	}
	if c.ControllerName != "" {
		if err := c.updateRemoteCredentials(ctx, credentials); err != nil {
			returnErr = err
		}
	}
	return returnErr
}

func credentialsFromFile(credentialsFile, cloudName, credentialName string) (map[string]jujucloud.CloudCredential, error) {
	data, err := os.ReadFile(credentialsFile)
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
	return nil, errors.NotFoundf("credential %q for cloud %q in local client", credentialName, cloudName)
}

func (c *updateCredentialCommand) updateLocalCredentials(ctx *cmd.Context, update map[string]jujucloud.CloudCredential) error {
	erred := false
	for cloudName, cloudCredentials := range update {
		aCloud, err := common.CloudByName(cloudName)
		if errors.Is(err, errors.NotFound) {
			ctx.Infof("Cloud %q not found.", cloudName)
			erred = true
			continue
		} else if err != nil {
			logger.Errorf(context.TODO(), "%v", err)
			ctx.Warningf("Could not verify cloud %v.", cloudName)
			erred = true
			continue
		}
		storedCredentials, err := c.Store.CredentialForCloud(cloudName)
		if errors.Is(err, errors.NotFound) {
			ctx.Warningf("Could not find credentials for cloud %v on this client.", cloudName)
			ctx.Infof("Use `juju add-credential` to add a credential to this client.")
			erred = true
			continue
		} else if err != nil {
			logger.Errorf(context.TODO(), "%v", err)
			ctx.Warningf("Could not get credentials for cloud %v from this client.", cloudName)
			erred = true
			continue
		}

		if c.Region != "" {
			if err := validCloudRegion(aCloud, c.Region); err != nil {
				logger.Errorf(context.TODO(), "%v", err)
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
					logger.Errorf(context.TODO(), "%v", err)
					logger.Warningf(context.TODO(), "Could not verify credential %v for cloud %v on this client", credentialName, aCloud.Name)
					erred = true
					continue
				}
				credential = *newCredential
			}
			storedCredentials.AuthCredentials[credentialName] = credential
		}
		err = c.Store.UpdateCredential(cloudName, *storedCredentials)
		if err != nil {
			logger.Errorf(context.TODO(), "%v", err)
			ctx.Warningf("Could not update this client with credentials for cloud %v", cloudName)
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
	accountDetails, err := c.Store.AccountDetails(c.ControllerName)
	if err != nil {
		return err
	}
	client, err := c.updateCredentialAPIFunc(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	// Get user clouds from the controller
	remoteUserClouds, err := client.Clouds(ctx)
	if err != nil {
		return err
	}

	var erred error
	verified := map[string]jujucloud.Credential{}
	mapUnion := func(items map[string]jujucloud.Credential) {
		maps.Copy(verified, items)
	}
	for cloudName, cloudCredentials := range update {
		remoteCloud, ok := remoteUserClouds[names.NewCloudTag(cloudName)]
		if !ok {
			ctx.Warningf("No cloud %q available to user %q remotely on controller %q", cloudName, accountDetails.User, c.ControllerName)
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
	results, err := client.UpdateCloudsCredentials(ctx, verified, c.Force)
	if err != nil {
		logger.Errorf(context.TODO(), "%v", err)
		ctx.Warningf("Could not update credentials remotely, on controller %q", c.ControllerName)
		erred = cmd.ErrSilent
	}
	return processUpdateCredentialResult(ctx, accountDetails, "updated", results, c.Force, c.ControllerName, erred)
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
			logger.Errorf(context.TODO(), "%v", err)
			ctx.Warningf("Could not verify credential %v for cloud %v on this client", credentialName, aCloud.Name)
			erred = cmd.ErrSilent
			continue
		}
		verified[names.NewCloudCredentialTag(id).String()] = *verifiedCredential
	}
	return verified, erred
}

func processUpdateCredentialResult(ctx *cmd.Context, accountDetails *jujuclient.AccountDetails, op string, results []params.UpdateCredentialResult, force bool, controllerName string, localError error) error {
	for _, result := range results {
		tag, err := names.ParseCloudCredentialTag(result.CredentialTag)
		if err != nil {
			logger.Errorf(context.TODO(), "%v", err)
			ctx.Warningf("Could not parse credential tag %q", result.CredentialTag)
			localError = cmd.ErrSilent
		}
		// We always want to display models information if there is any.
		common.OutputUpdateCredentialModelResult(ctx, result.Models, true)
		haveModelErrors := false
		for _, m := range result.Models {
			haveModelErrors = len(m.Errors) > 0
			if haveModelErrors {
				break
			}
		}
		if haveModelErrors || result.Error != nil {
			if haveModelErrors {
				ctx.Infof("Failed models may require a different credential.")
				msg := "Use 'juju set-credential' to change credential for these models."
				if !force {
					msg = "Use 'juju set-credential' to change credential for these models before repeating this update."
				}
				ctx.Infof("%s", msg)
			}
			if result.Error != nil {
				ctx.Warningf("Controller credential %q for user %q for cloud %q on controller %q not %v: %v.", tag.Name(), accountDetails.User, tag.Cloud().Id(), controllerName, op, result.Error)
			}
			localError = cmd.ErrSilent
			continue
		}
		ctx.Infof(`
Controller credential %q for user %q for cloud %q on controller %q %v.
For more information, see 'juju show-credential %v %v'.`[1:],
			tag.Name(), accountDetails.User, tag.Cloud().Id(), controllerName,
			op, tag.Cloud().Id(), tag.Name())
	}
	return localError
}
