// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/yaml.v2"

	jujucloud "github.com/juju/juju/cloud"
)

type showCloudCommand struct {
	cmd.CommandBase
	out cmd.Output

	CloudName string
}

var showCloudDoc = `
Provided information includes 'defined' (public, built-in), 'type',
'auth-type', 'regions', and 'endpoints'.

Examples:

    juju show-cloud google
    juju show-cloud azure-china --output ~/azure_cloud_details.txt

See also:
    clouds
    update-clouds
`

// NewShowCloudCommand returns a command to list cloud information.
func NewShowCloudCommand() cmd.Command {
	return &showCloudCommand{}
}

func (c *showCloudCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
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
		Args:    "<cloud name>",
		Purpose: "Shows detailed information on a cloud.",
		Doc:     showCloudDoc,
	}
}

func (c *showCloudCommand) Run(ctxt *cmd.Context) error {
	details, err := getCloudDetails()
	if err != nil {
		return err
	}
	cloud, ok := details[c.CloudName]
	if !ok {
		return errors.NotFoundf("cloud %q", c.CloudName)
	}
	return c.out.Write(ctxt, cloud)
}

type regionDetails struct {
	Name             string `yaml:"-" json:"-"`
	Endpoint         string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	IdentityEndpoint string `yaml:"identity-endpoint,omitempty" json:"identity-endpoint,omitempty"`
	StorageEndpoint  string `yaml:"storage-endpoint,omitempty" json:"storage-endpoint,omitempty"`
}

type cloudDetails struct {
	Source           string   `yaml:"defined,omitempty" json:"defined,omitempty"`
	CloudType        string   `yaml:"type" json:"type"`
	AuthTypes        []string `yaml:"auth-types,omitempty,flow" json:"auth-types,omitempty"`
	Endpoint         string   `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	IdentityEndpoint string   `yaml:"identity-endpoint,omitempty" json:"identity-endpoint,omitempty"`
	StorageEndpoint  string   `yaml:"storage-endpoint,omitempty" json:"storage-endpoint,omitempty"`
	// Regions is for when we want to print regions in order for yaml or tabular output.
	Regions yaml.MapSlice `yaml:"regions,omitempty" json:"-"`
	// Regions map is for json marshalling where format is important but not order.
	RegionsMap   map[string]regionDetails `yaml:"-" json:"regions,omitempty"`
	Config       map[string]interface{}   `yaml:"config,omitempty" json:"config,omitempty"`
	RegionConfig jujucloud.RegionConfig   `yaml:"region-config,omitempty" json:"region-config,omitempty"`
}

func makeCloudDetails(cloud jujucloud.Cloud) *cloudDetails {
	result := &cloudDetails{
		Source:           "public",
		CloudType:        cloud.Type,
		Endpoint:         cloud.Endpoint,
		IdentityEndpoint: cloud.IdentityEndpoint,
		StorageEndpoint:  cloud.StorageEndpoint,
		Config:           cloud.Config,
		RegionConfig:     cloud.RegionConfig,
	}
	result.AuthTypes = make([]string, len(cloud.AuthTypes))
	for i, at := range cloud.AuthTypes {
		result.AuthTypes[i] = string(at)
	}
	result.RegionsMap = make(map[string]regionDetails)
	for _, region := range cloud.Regions {
		r := regionDetails{Name: region.Name}
		if region.Endpoint != result.Endpoint {
			r.Endpoint = region.Endpoint
		}
		if region.IdentityEndpoint != result.IdentityEndpoint {
			r.IdentityEndpoint = region.IdentityEndpoint
		}
		if region.StorageEndpoint != result.StorageEndpoint {
			r.StorageEndpoint = region.StorageEndpoint
		}
		result.Regions = append(result.Regions, yaml.MapItem{r.Name, r})
		result.RegionsMap[region.Name] = r
	}
	return result
}

func getCloudDetails() (map[string]*cloudDetails, error) {
	result, err := listCloudDetails()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result.all(), nil
}
