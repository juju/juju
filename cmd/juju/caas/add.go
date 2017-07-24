// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/base"
	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

type CloudMetadataStore interface {
	ParseCloudMetadataFile(path string) (map[string]cloud.Cloud, error)
	ParseOneCloud(data []byte) (cloud.Cloud, error)
	PublicCloudMetadata(searchPaths ...string) (result map[string]cloud.Cloud, fallbackUsed bool, _ error)
	PersonalCloudMetadata() (map[string]cloud.Cloud, error)
	WritePersonalCloudMetadata(cloudsMap map[string]cloud.Cloud) error
}

// Implemented by cloudapi.Client
type AddCloudAPI interface {
	AddCloud(cloud.Cloud) error
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

	// CAASName is the name of the caas to add.
	CAASName string

	// CAASType is the type of CAAS being added
	CAASType string

	// Context is the name of the context (k8s) or credential to import
	Context string

	clientStore        jujuclient.ClientStore
	cloudMetadataStore CloudMetadataStore
	newCloudAPI        func(base.APICallCloser) AddCloudAPI
}

// NewAddCAASCommand returns a command to add caas information.
func NewAddCAASCommand(cloudMetadataStore CloudMetadataStore) *AddCAASCommand {
	return &AddCAASCommand{
		cloudMetadataStore: cloudMetadataStore,
		newCloudAPI: func(caller base.APICallCloser) AddCloudAPI {
			return cloudapi.NewClient(caller)
		},
	}
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
	if len(args) > 0 {
		c.CAASType = args[0]
	}
	if len(args) > 1 {
		c.CAASName = args[1]
	}
	if len(args) > 2 {
		return cmd.CheckEmpty(args[2:])
	}
	return nil
}

// Run executes the add caas command, adding a caas based on a passed-in yaml
// file or interactive queries.
func (c *AddCAASCommand) Run(ctxt *cmd.Context) error {
	api, err := c.NewControllerAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	newCloud := cloud.Cloud{
		Name:      c.CAASName,
		Type:      c.CAASType,
		AuthTypes: []cloud.AuthType{cloud.UserPassAuthType},
	}

	if err := addCloudToLocal(c.cloudMetadataStore, newCloud); err != nil {
		return errors.Trace(err)
	}

	cloudClient := c.newCloudAPI(api)
	if err := addCloudToController(cloudClient, newCloud); err != nil {
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

func addCloudToController(apiClient AddCloudAPI, newCloud cloud.Cloud) error {
	err := apiClient.AddCloud(newCloud)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
