// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	stdcontext "context"
	"fmt"
	"net/url"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	cloudapi "github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/proxy"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

// UpdateCloudAPI - Implemented by cloudapi.Client.
type UpdateCloudAPI interface {
	Cloud(tag names.CloudTag) (jujucloud.Cloud, error)
	UpdateCloud(jujucloud.Cloud) error
	UpdateCloudsCredentials(cloudCredentials map[string]jujucloud.Credential, force bool) ([]params.UpdateCredentialResult, error)
	Close() error
}

var usageUpdateCAASSummary = `
Updates an existing k8s endpoint used by Juju.`[1:]

var usageUpdateCAASDetails = `
Update k8s cloud information on this client and/or on a controller.

The k8s cloud can be a built-in cloud like microk8s.

A k8s cloud can also be updated from a file. This requires a <cloud name> and
a yaml file containing the cloud details.

A k8s cloud on the controller can also be updated just by using a name of a k8s cloud
from this client.

Use --controller option to update a k8s cloud on a controller.

Use --client to update a k8s cloud definition on this client.

Examples:
    juju update-k8s microk8s
    juju update-k8s myk8s -f path/to/k8s.yaml
    juju update-k8s myk8s -f path/to/k8s.yaml --controller mycontroller
    juju update-k8s myk8s --controller mycontroller
    juju update-k8s myk8s --client --controller mycontroller
    juju update-k8s myk8s --client -f path/to/k8s.yaml

See also:
    add-k8s
    remove-k8s
`

// UpdateCAASCommand is the command that allows you to update a caas cloud.
type UpdateCAASCommand struct {
	modelcmd.OptionalControllerCommand

	// caasName is the name of the k8s cloud to update
	caasName string

	// CloudFile is the name of the cloud YAML file
	CloudFile string

	// builtInCloudsFunc is used to provide any built in clouds and their credential.
	builtInCloudsFunc func(string) (jujucloud.Cloud, *jujucloud.Credential, string, error)

	// updateCloudAPIFunc is used when updating a cluster on a controller.
	updateCloudAPIFunc func() (UpdateCloudAPI, error)

	// brokerGetter returns CAAS broker instance.
	brokerGetter BrokerGetter

	// cloudMetadataStore provides access to personal cloud metadata.
	cloudMetadataStore CloudMetadataStore
}

// NewUpdateCAASCommand returns a command to update CAAS information.
func NewUpdateCAASCommand(cloudMetadataStore CloudMetadataStore) cmd.Command {
	return newUpdateCAASCommand(cloudMetadataStore)
}

func newUpdateCAASCommand(cloudMetadataStore CloudMetadataStore) cmd.Command {
	store := jujuclient.NewFileClientStore()
	command := &UpdateCAASCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store: store,
		},
		cloudMetadataStore: cloudMetadataStore,
		builtInCloudsFunc:  maybeBuiltInCloud,
	}
	command.brokerGetter = command.newK8sClusterBroker
	command.updateCloudAPIFunc = func() (UpdateCloudAPI, error) {
		root, err := command.NewAPIRoot(command.Store, command.ControllerName, "")
		if err != nil {
			return nil, errors.Trace(err)
		}
		return cloudapi.NewClient(root), nil
	}
	return modelcmd.WrapBase(command)
}

// Info returns help information about the command.
func (c *UpdateCAASCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "update-k8s",
		Args:    "<k8s name>",
		Purpose: usageUpdateCAASSummary,
		Doc:     usageUpdateCAASDetails,
	})
}

// SetFlags initializes the flags supported by the command.
func (c *UpdateCAASCommand) SetFlags(f *gnuflag.FlagSet) {
	c.OptionalControllerCommand.SetFlags(f)
	f.StringVar(&c.CloudFile, "f", "", "The path to a cloud definition file")
}

// Init populates the command with the args from the command line.
func (c *UpdateCAASCommand) Init(args []string) error {
	if err := c.OptionalControllerCommand.Init(args); err != nil {
		return err
	}
	if len(args) < 1 {
		return errors.BadRequestf("k8s cloud name required")
	}

	c.caasName = args[0]
	if ok := names.IsValidCloud(c.caasName); !ok {
		return errors.NotValidf("k8s cloud name %q", c.caasName)
	}
	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return err
	}
	return nil
}

func (c *UpdateCAASCommand) newK8sClusterBroker(cloud jujucloud.Cloud, credential jujucloud.Credential) (caas.ClusterMetadataChecker, error) {
	openParams, err := provider.BaseKubeCloudOpenParams(cloud, credential)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if c.ControllerName != "" {
		ctrlUUID, err := c.ControllerUUID(c.Store, c.ControllerName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		openParams.ControllerUUID = ctrlUUID
	}
	return caas.New(stdcontext.TODO(), openParams)
}

// maybeBuiltInCloud returns a built in cloud (eg microk8s) and the relevant credential
// if it exists, else returns a not found error.
func maybeBuiltInCloud(cloudName string) (jujucloud.Cloud, *jujucloud.Credential, string, error) {
	fail := func(err error) (jujucloud.Cloud, *jujucloud.Credential, string, error) {
		return jujucloud.Cloud{}, nil, "", errors.Trace(err)
	}
	builtIn, err := common.BuiltInClouds()
	if err != nil {
		return fail(err)
	}
	cloud, ok := builtIn[cloudName]
	if !ok {
		return fail(errors.NotFoundf("built in cloud %q", cloudName))
	}
	p, err := environs.Provider(cloud.Type)
	if err != nil {
		return fail(err)
	}

	creds, err := modelcmd.RegisterCredentials(p, modelcmd.RegisterCredentialsParams{Cloud: cloud})
	if err != nil {
		return fail(err)
	}
	cred, ok := creds[cloudName]
	if !ok {
		return cloud, nil, "", nil
	}
	// Return the first named credential.
	// There will only be one for microk8s.
	for name, cred := range cred.AuthCredentials {
		return cloud, &cred, name, nil
	}
	return cloud, nil, "", nil
}

// Run is defined on the Command interface.
func (c *UpdateCAASCommand) Run(ctx *cmd.Context) (err error) {
	var newCloud *jujucloud.Cloud
	havePersonalCloud := false
	haveBuiltinCloud := false
	// First, see if we're updating a built-in cloud.
	builtinCloud, credential, credentialName, err := c.builtInCloudsFunc(c.caasName)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if err == nil {
		if c.CloudFile != "" {
			ctx.Infof("%q is a built-in cloud and does not support specifying a cloud YAML file.", c.caasName)
			return cmd.ErrSilent
		}
		newCloud = &builtinCloud
		haveBuiltinCloud = true
	} else if c.CloudFile != "" {
		r := &cloud.CloudFileReader{
			CloudMetadataStore: c.cloudMetadataStore,
			CloudName:          c.caasName,
		}
		var err error
		if newCloud, err = r.ReadCloudFromFile(c.CloudFile, ctx); err != nil {
			return errors.Annotatef(err, "could not read cloud definition from provided file")
		}
		c.caasName = r.CloudName
	} else {
		personalClouds, err := c.cloudMetadataStore.PersonalCloudMetadata()
		if err != nil {
			return errors.Trace(err)
		}
		if localCloud, ok := personalClouds[c.caasName]; !ok {
			return errors.NotFoundf("cloud %s", c.caasName)
		} else {
			newCloud = &localCloud
			havePersonalCloud = true
		}
	}

	if newCloud.Type != k8sconstants.CAASProviderType {
		ctx.Infof("The %q cloud is a %q cloud and not a %q cloud.", c.caasName, newCloud.Type, k8sconstants.CAASProviderType)
		return cmd.ErrSilent
	}

	if err := c.MaybePrompt(ctx, fmt.Sprintf("update k8s cloud %q on", c.caasName)); err != nil {
		return errors.Trace(err)
	}

	// Check the cluster only if we have a credential to use.
	if credential != nil {
		broker, err := c.brokerGetter(*newCloud, *credential)
		if err != nil {
			return errors.Trace(err)
		}

		if _, err := broker.GetClusterMetadata(""); err != nil {
			return errors.Annotate(err, "unable to update k8s cluster because the cluster is not accessible")
		}
	}

	var returnErr error
	processErr := func(err error, successMsg string) {
		if err != nil {
			if err != cmd.ErrSilent {
				ctx.Infof("%v", err)
			}
			returnErr = cmd.ErrSilent
			return
		}
		ctx.Infof("%s", successMsg)
	}
	if c.Client {
		if !haveBuiltinCloud && !havePersonalCloud {
			if err := updateCloudOnLocal(c.cloudMetadataStore, *newCloud); err != nil {
				ctx.Infof("%v", err)
				return cmd.ErrSilent
			}
		}
		err = updateControllerProxyOnLocal(c.Store, *newCloud)
		processErr(err, fmt.Sprintf("k8s cloud %q updated on this client.", c.caasName))
	}
	if c.ControllerName != "" {
		if err := jujuclient.ValidateControllerName(c.ControllerName); err != nil {
			return errors.Trace(err)
		}
		cloudClient, err := c.updateCloudAPIFunc()
		if err != nil {
			return errors.Trace(err)
		}
		defer cloudClient.Close()

		existing, err := cloudClient.Cloud(names.NewCloudTag(newCloud.Name))
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		if existing.Type != k8sconstants.CAASProviderType {
			ctx.Infof("The %q cloud on the controller is a %q cloud and not a %q cloud.", c.caasName, existing.Type, k8sconstants.CAASProviderType)
			return cmd.ErrSilent
		}

		err = cloudClient.UpdateCloud(*newCloud)
		processErr(err, fmt.Sprintf("k8s cloud %q updated on controller %q.", c.caasName, c.ControllerName))
		if credential != nil {
			err = c.updateCredentialOnController(ctx, cloudClient, *credential, c.caasName, credentialName)
			processErr(err, fmt.Sprintf("k8s cloud credential %q updated on controller %q.", credentialName, c.ControllerName))
		}
	}
	return returnErr
}

func updateCloudOnLocal(cloudMetadataStore CloudMetadataStore, updatedCloud jujucloud.Cloud) error {
	personalClouds, err := cloudMetadataStore.PersonalCloudMetadata()
	if err != nil {
		return errors.Trace(err)
	}
	if personalClouds == nil {
		personalClouds = make(map[string]jujucloud.Cloud)
	}
	personalClouds[updatedCloud.Name] = updatedCloud
	return cloudMetadataStore.WritePersonalCloudMetadata(personalClouds)
}

// updateControllerProxyOnLocal ensures that any local controller proxy config
// is updated to have the endpoint connectivity required by the updatedCloud.
func updateControllerProxyOnLocal(store jujuclient.ControllerStore, updatedCloud jujucloud.Cloud) error {
	all, err := store.AllControllers()
	if err != nil {
		return errors.Trace(err)
	}
	for name, details := range all {
		if details.Cloud != updatedCloud.Name || details.Proxy == nil {
			continue
		}
		k8sProxier, ok := details.Proxy.Proxier.(*proxy.Proxier)
		if !ok {
			continue
		}
		host := updatedCloud.Endpoint
		if apiURL, err := url.Parse(updatedCloud.Endpoint); err == nil {
			host = apiURL.Host
		}
		k8sProxier.SetAPIHost(host)
		details.Proxy.Proxier = k8sProxier
		err = store.UpdateController(name, details)
		if err != nil {
			return errors.Annotate(err, "saving controller details for updated k8s cluster")
		}
	}
	return nil
}

func (c *UpdateCAASCommand) updateCredentialOnController(ctx *cmd.Context, apiClient UpdateCloudAPI, newCredential jujucloud.Credential, cloudName, credentialName string) error {
	_, err := c.Store.ControllerByName(c.ControllerName)
	if err != nil {
		return errors.Trace(err)
	}

	currentAccountDetails, err := c.Store.AccountDetails(c.ControllerName)
	if err != nil {
		return errors.Trace(err)
	}

	id := fmt.Sprintf("%s/%s/%s", cloudName, currentAccountDetails.User, credentialName)
	if !names.IsValidCloudCredential(id) {
		return errors.NotValidf("cloud credential ID %q", id)
	}
	cloudCredTag := names.NewCloudCredentialTag(id)

	toUpdate := map[string]jujucloud.Credential{}
	toUpdate[cloudCredTag.String()] = newCredential
	results, err := apiClient.UpdateCloudsCredentials(toUpdate, false)
	if err != nil {
		return errors.Trace(err)
	}
	var resultError error
	for _, result := range results {
		tag, err := names.ParseCloudCredentialTag(result.CredentialTag)
		if err != nil {
			logger.Errorf("%v", err)
			ctx.Warningf("Could not parse credential tag %q", result.CredentialTag)
			resultError = cmd.ErrSilent
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
				ctx.Infof("Use ‘juju set-credential’ to change credential for these models before repeating this update.")
			}
			if result.Error != nil {
				ctx.Warningf("Controller credential %q for user %q for cloud %q on controller %q not updated: %v.",
					tag.Name(), currentAccountDetails.User, tag.Cloud().Id(), c.ControllerName, result.Error)
			}
			// We do not want to return err here as we have already displayed it on the console.
			resultError = cmd.ErrSilent
			continue
		}
	}
	return resultError
}
