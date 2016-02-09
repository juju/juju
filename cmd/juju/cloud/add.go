// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cloud"
)

type addCloudCommand struct {
	cmd.CommandBase
	out cmd.Output

	// Replace, if true, existing cloud information is overwritten.
	Replace bool

	// Cloud is the name fo the cloud to add.
	Cloud string

	// CloudFile is the name of the cloud YAML file.
	CloudFile string
}

var addCloudDoc = `
The add-cloud command adds a new local definition for a cloud on which Juju workloads can be deployed.
The available clouds will be the publicly available clouds like AWS, Google, Azure,
as well as any custom clouds make available by this add-cloud command.

The user is required to specify the name of the cloud to add and a YAML file containing clould definitions.
A sample YAML snippet is:

clouds:
  homestack:
    type: openstack
    auth-types: [ userpass ]
    regions:
      london:
        endpoint: https://london.homestack.com:35574/v3.0/

If the named cloud already exists, the --replace option is required to overwite it. There is no merge option.

Example:
   juju add-cloud homestack personal-clouds.yaml
   juju add-cloud homestack personal-clouds.yaml --replace
`

// NewAddCloudCommand returns a command to add cloud information.
func NewAddCloudCommand() cmd.Command {
	return &addCloudCommand{}
}

func (c *addCloudCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-cloud",
		Purpose: "adds a named cloud definition to the list of those which can run Juju workloads",
		Doc:     addCloudDoc,
	}
}

func (c *addCloudCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Replace, "replace", false, "overwrite any existing cloud information")
}

func (c *addCloudCommand) Init(args []string) (err error) {
	if len(args) < 2 {
		return errors.New("Usage: juju add-cloud <cloud-name> <cloud.yaml>")
	}
	c.Cloud = args[0]
	c.CloudFile = args[1]
	return cmd.CheckEmpty(args[2:])
}

func (c *addCloudCommand) Run(ctxt *cmd.Context) error {
	specifiedClouds, err := cloud.ParseCloudMetadataFile(c.CloudFile)
	if err != nil {
		return err
	}
	if specifiedClouds == nil {
		return errors.New("no personal clouds are defined")
	}
	newCloud, ok := specifiedClouds[c.Cloud]
	if !ok {
		return errors.Errorf("cloud %q not found in file %q", c.Cloud, c.CloudFile)
	}
	personalClouds, err := cloud.PersonalCloudMetadata()
	if err != nil {
		return err
	}
	if _, ok = personalClouds[c.Cloud]; ok && !c.Replace {
		return errors.Errorf("cloud called %q already exists; use --replace to replace this existing cloud", c.Cloud)
	}
	if personalClouds == nil {
		personalClouds = make(map[string]cloud.Cloud)
	}
	personalClouds[c.Cloud] = newCloud
	return cloud.WritePersonalCloudMetadata(personalClouds)
}
