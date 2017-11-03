// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	cloudapi "github.com/juju/juju/api/cloud"
	modelmanagerapi "github.com/juju/juju/api/modelmanager"
	caascfg "github.com/juju/juju/caas/clientconfig"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

var logger = loggo.GetLogger("juju.cmd.juju.caas")

type CloudMetadataStore interface {
	ParseCloudMetadataFile(path string) (map[string]cloud.Cloud, error)
	ParseOneCloud(data []byte) (cloud.Cloud, error)
	PublicCloudMetadata(searchPaths ...string) (result map[string]cloud.Cloud, fallbackUsed bool, _ error)
	PersonalCloudMetadata() (map[string]cloud.Cloud, error)
	WritePersonalCloudMetadata(cloudsMap map[string]cloud.Cloud) error
}

// Implemented by cloudapi.Client
type CloudAPI interface {
	AddCloud(cloud.Cloud) error
	AddCredential(tag string, credential cloud.Credential) error
	Close() error
}

type ModelManagerAPI interface {
	SetModelDefaults(cloud, region string, config map[string]interface{}) error
}

var usageAddCAASSummary = `
Adds a CAAS endpoint and credential to Juju from among known types.`[1:]

var usageAddCAASDetails = `

Examples:
    juju add-caas myk8s kubernetes

See also:
    caas`

// AddCAASCommand is the command that allows you to add a caas and credential
type AddCAASCommand struct {
	modelcmd.ModelCommandBase

	// caasName is the name of the caas to add.
	caasName string

	// CAASType is the type of CAAS being added
	caasType string

	// Context is the name of the context (k8s) or credential to import
	context string

	cloudMetadataStore    CloudMetadataStore
	fileCredentialStore   jujuclient.CredentialStore
	apiRoot               api.Connection
	newCloudAPI           func(base.APICallCloser) CloudAPI
	newClientConfigReader func(string) (caascfg.ClientConfigFunc, error)
	newModelManagerAPI    func(base.APICallCloser) ModelManagerAPI
}

// NewAddCAASCommand returns a command to add caas information.
func NewAddCAASCommand(cloudMetadataStore CloudMetadataStore) cmd.Command {
	cmd := &AddCAASCommand{
		cloudMetadataStore:  cloudMetadataStore,
		fileCredentialStore: jujuclient.NewFileCredentialStore(),
		newCloudAPI: func(caller base.APICallCloser) CloudAPI {
			return cloudapi.NewClient(caller)
		},
		newClientConfigReader: func(caasType string) (caascfg.ClientConfigFunc, error) {
			return caascfg.NewClientConfigReader(caasType)
		},
		newModelManagerAPI: func(caller base.APICallCloser) ModelManagerAPI {
			return modelmanagerapi.NewClient(caller)
		},
	}
	return modelcmd.Wrap(cmd)
}
func NewAddCAASCommandForTest(cloudMetadataStore CloudMetadataStore, fileCredentialStore jujuclient.CredentialStore, clientStore jujuclient.ClientStore, apiRoot api.Connection, newCloudAPIFunc func(base.APICallCloser) CloudAPI, newClientConfigReaderFunc func(string) (caascfg.ClientConfigFunc, error), newModelManagerAPIFunc func(base.APICallCloser) ModelManagerAPI) cmd.Command {
	cmd := &AddCAASCommand{
		cloudMetadataStore:    cloudMetadataStore,
		fileCredentialStore:   fileCredentialStore,
		apiRoot:               apiRoot,
		newCloudAPI:           newCloudAPIFunc,
		newClientConfigReader: newClientConfigReaderFunc,
		newModelManagerAPI:    newModelManagerAPIFunc,
	}
	cmd.SetClientStore(clientStore)
	return modelcmd.Wrap(cmd)
}

// Info returns help information about the command.
func (c *AddCAASCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-caas",
		Args:    "<caas type> <caas name>",
		Purpose: usageAddCAASSummary,
		Doc:     usageAddCAASDetails,
	}
}

// SetFlags initializes the flags supported by the command.
func (c *AddCAASCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
}

// Init populates the command with the args from the command line.
func (c *AddCAASCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.Errorf("missing CAAS type and CAAS name.")
	}
	if len(args) == 1 {
		return errors.Errorf("missing CAAS name.")
	}
	c.caasType = args[0]
	c.caasName = args[1]
	return cmd.CheckEmpty(args[2:])
}

func (c *AddCAASCommand) newAPIRoot() (api.Connection, error) {
	if c.apiRoot != nil {
		return c.apiRoot, nil
	}
	return c.NewControllerAPIRoot()
}

func (c *AddCAASCommand) Run(ctxt *cmd.Context) error {
	api, err := c.newAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	if err := c.verifyName(c.caasName); err != nil {
		return errors.Trace(err)
	}

	clientConfigFunc, err := c.newClientConfigReader(c.caasType)
	if err != nil {
		return errors.Trace(err)
	}

	caasConfig, err := clientConfigFunc()
	if err != nil {
		return errors.Trace(err)
	}

	if len(caasConfig.Contexts) == 0 {
		return errors.Errorf("No CAAS cluster definitions found in config")
	}
	defaultContext := caasConfig.Contexts[caasConfig.CurrentContext]

	defaultCredential := caasConfig.Credentials[defaultContext.CredentialName]
	defaultCloud := caasConfig.Clouds[defaultContext.CloudName]

	cloudConfig := map[string]interface{}{
		"CAData": defaultCloud.Attributes["CAData"],
	}

	defaultRegion := cloud.Region{
		Name:     "default",
		Endpoint: defaultCloud.Endpoint,
	}

	newCloud := cloud.Cloud{
		Name:      c.caasName,
		Type:      c.caasType,
		Endpoint:  defaultCloud.Endpoint,
		Config:    cloudConfig,
		AuthTypes: []cloud.AuthType{defaultCredential.AuthType()},
		Regions:   []cloud.Region{defaultRegion},
	}

	if err := addCloudToLocal(c.cloudMetadataStore, newCloud); err != nil {
		return errors.Trace(err)
	}

	cloudClient := c.newCloudAPI(api)
	modelManagerClient := c.newModelManagerAPI(api)

	if err := addCloudToController(cloudClient, modelManagerClient, newCloud); err != nil {
		return errors.Trace(err)
	}

	if err := c.addCredentialToLocal(c.caasName, defaultCredential, defaultContext.CredentialName); err != nil {
		return errors.Trace(err)
	}

	if err := c.addCredentialToController(cloudClient, defaultCredential, defaultContext.CredentialName); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (c *AddCAASCommand) verifyName(name string) error {
	public, _, err := c.cloudMetadataStore.PublicCloudMetadata()
	if err != nil {
		return err
	}
	msg, err := nameExists(name, public)
	if err != nil {
		return errors.Trace(err)
	}
	if msg != "" {
		return errors.Errorf(msg)
	}
	return nil
}

// nameExists returns either an empty string if the name does not exist, or a
// non-empty string with an error message if it does exist.
func nameExists(name string, public map[string]cloud.Cloud) (string, error) {
	if _, ok := public[name]; ok {
		return fmt.Sprintf("%q is the name of a public cloud", name), nil
	}
	builtin, err := common.BuiltInClouds()
	if err != nil {
		return "", errors.Trace(err)
	}
	if _, ok := builtin[name]; ok {
		return fmt.Sprintf("%q is the name of a built-in cloud", name), nil
	}
	return "", nil
}

func addCloudToLocal(cloudMetadataStore CloudMetadataStore, newCloud cloud.Cloud) error {
	personalClouds, err := cloudMetadataStore.PersonalCloudMetadata()
	if err != nil {
		return err
	}
	if personalClouds == nil {
		personalClouds = make(map[string]cloud.Cloud)
	}
	personalClouds[newCloud.Name] = newCloud
	return cloudMetadataStore.WritePersonalCloudMetadata(personalClouds)
}

func addCloudToController(cloudAPIClient CloudAPI, modelManagerAPIClient ModelManagerAPI, newCloud cloud.Cloud) error {

	err := cloudAPIClient.AddCloud(newCloud)
	if err != nil {
		return errors.Trace(err)
	}

	if len(newCloud.Regions) == 0 {
		return errors.Errorf("CAAS clouds should always have at least one default region")
	} else {
		for _, region := range newCloud.Regions {
			err = modelManagerAPIClient.SetModelDefaults(newCloud.Name, region.Name, newCloud.Config)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func (c *AddCAASCommand) addCredentialToLocal(cloudName string, newCredential cloud.Credential, credentialName string) error {
	newCredentials := &cloud.CloudCredential{
		AuthCredentials: make(map[string]cloud.Credential),
	}
	newCredentials.AuthCredentials[credentialName] = newCredential
	err := c.fileCredentialStore.UpdateCredential(cloudName, *newCredentials)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *AddCAASCommand) addCredentialToController(apiClient CloudAPI, newCredential cloud.Credential, credentialName string) error {
	currentAccountDetails, err := c.CurrentAccountDetails()
	if err != nil {
		return errors.Trace(err)
	}

	cloudCredTag := names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s",
		c.caasName, currentAccountDetails.User, credentialName))

	if err := apiClient.AddCredential(cloudCredTag.String(), newCredential); err != nil {
		return errors.Trace(err)
	}
	return nil
}
