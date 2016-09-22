// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
)

var usageAddCloudSummary = `
Adds a user-defined cloud to Juju from among known cloud types.`[1:]

var usageAddCloudDetails = `
A cloud definition file has the following YAML format:

clouds:
  mycloud:
    type: openstack
    auth-types: [ userpass ]
    regions:
      london:
        endpoint: https://london.mycloud.com:35574/v3.0/

If the named cloud already exists, the `[1:] + "`--replace`" + ` option is required to 
overwrite its configuration.
Known cloud types: azure, cloudsigma, ec2, gce, joyent, lxd, maas, manual,
openstack, rackspace

Examples:
    juju add-cloud mycloud ~/mycloud.yaml

See also: 
    clouds`

type addCloudCommand struct {
	cmd.CommandBase

	// Replace, if true, existing cloud information is overwritten.
	Replace bool

	// Cloud is the name fo the cloud to add.
	Cloud string

	// CloudFile is the name of the cloud YAML file.
	CloudFile string
}

// NewAddCloudCommand returns a command to add cloud information.
func NewAddCloudCommand() cmd.Command {
	return &addCloudCommand{}
}

func (c *addCloudCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-cloud",
		Args:    "<cloud name> <cloud definition file>",
		Purpose: usageAddCloudSummary,
		Doc:     usageAddCloudDetails,
	}
}

func (c *addCloudCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.BoolVar(&c.Replace, "replace", false, "Overwrite any existing cloud information")
}

func (c *addCloudCommand) Init(args []string) (err error) {
	if len(args) < 2 {
		return errors.New("Usage: juju add-cloud <cloud name> <cloud definition file>")
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
	publicClouds, _, err := cloud.PublicCloudMetadata()
	if err != nil {
		return err
	}
	if _, ok = publicClouds[c.Cloud]; ok && !c.Replace {
		return errors.Errorf("%q is the name of a public cloud; use --replace to use your cloud definition instead", c.Cloud)
	}
	builtinClouds := common.BuiltInClouds()
	if _, ok = builtinClouds[c.Cloud]; ok && !c.Replace {
		return errors.Errorf("%q is the name of a built-in cloud; use --replace to use your cloud definition instead", c.Cloud)
	}
	personalClouds, err := cloud.PersonalCloudMetadata()
	if err != nil {
		return err
	}
	if _, ok = personalClouds[c.Cloud]; ok && !c.Replace {
		return errors.Errorf("%q already exists; use --replace to replace this existing cloud", c.Cloud)
	}
	if personalClouds == nil {
		personalClouds = make(map[string]cloud.Cloud)
	}
	personalClouds[c.Cloud] = newCloud
	return cloud.WritePersonalCloudMetadata(personalClouds)
}
