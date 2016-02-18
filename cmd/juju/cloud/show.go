// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	jujucloud "github.com/juju/juju/cloud"
)

type showCloudCommand struct {
	cmd.CommandBase
	out cmd.Output

	CloudName string
}

var showCloudDoc = `
The show-cloud command displays information about a specified cloud.

Example:
   juju show-cloud aws
`

// NewShowCloudCommand returns a command to list cloud information.
func NewShowCloudCommand() cmd.Command {
	return &showCloudCommand{}
}

func (c *showCloudCommand) SetFlags(f *gnuflag.FlagSet) {
	// We only support yaml for display purposes.
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
	})
}

func (c *showCloudCommand) Init(args []string) error {
	switch len(args) {
	case 1:
		c.CloudName = args[0]
	default:
		return errors.New("no cloud specified")
	}
	return cmd.CheckEmpty(args[1:])
}

func (c *showCloudCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-cloud",
		Args:    "<cloudname>",
		Purpose: "show the details a specified cloud",
		Doc:     showCloudDoc,
	}
}

func (c *showCloudCommand) Run(ctxt *cmd.Context) error {
	publicClouds, _, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	if err != nil {
		return err
	}
	cloud, ok := publicClouds[c.CloudName]
	if !ok {
		return errors.NotFoundf("cloud %q", c.CloudName)
	}
	return c.out.Write(ctxt, makeCloudDetails(cloud))
}

type regionDetails struct {
	Endpoint        string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	StorageEndpoint string `yaml:"storage-endpoint,omitempty" json:"storage-endpoint,omitempty"`
}

type cloudDetails struct {
	Source          string                   `yaml:"defined,omitempty" json:"defined,omitempty"`
	CloudType       string                   `yaml:"type" json:"type"`
	AuthTypes       []string                 `yaml:"auth-types,omitempty,flow" json:"auth-types,omitempty"`
	Endpoint        string                   `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	StorageEndpoint string                   `yaml:"storage-endpoint,omitempty" json:"storage-endpoint,omitempty"`
	Regions         map[string]regionDetails `yaml:"regions,omitempty" json:"regions,omitempty"`
}

func makeCloudDetails(cloud jujucloud.Cloud) *cloudDetails {
	result := &cloudDetails{
		Source:          "public",
		CloudType:       cloud.Type,
		Endpoint:        cloud.Endpoint,
		StorageEndpoint: cloud.StorageEndpoint,
	}
	result.AuthTypes = make([]string, len(cloud.AuthTypes))
	for i, at := range cloud.AuthTypes {
		result.AuthTypes[i] = string(at)
	}
	result.Regions = make(map[string]regionDetails)
	for _, region := range cloud.Regions {
		result.Regions[region.Name] = regionDetails{
			Endpoint:        region.Endpoint,
			StorageEndpoint: region.Endpoint,
		}
	}
	return result
}
