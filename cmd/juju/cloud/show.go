// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/yaml.v2"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
)

type showCloudCommand struct {
	cmd.CommandBase
	out cmd.Output

	CloudName string

	includeConfig bool
}

var showCloudDoc = `
Provided information includes 'defined' (public, built-in), 'type',
'auth-type', 'regions', 'endpoints', and cloud specific configuration
options.

If ‘--include-config’ is used, additional configuration (key, type, and
description) specific to the cloud are displayed if available.

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
	f.BoolVar(&c.includeConfig, "include-config", false, "Print available config option details specific to the specified cloud")
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
	if err = c.out.Write(ctxt, cloud); err != nil {
		return err
	}
	if c.includeConfig {
		config := getCloudConfigDetails(cloud.CloudType)
		if len(config) > 0 {
			fmt.Fprintln(ctxt.Stdout, fmt.Sprintf("\nThe available config options specific to %s clouds are:", cloud.CloudType))
			return c.out.Write(ctxt, config)
		}
	}
	return nil
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
	CloudDescription string   `yaml:"description" json:"description"`
	AuthTypes        []string `yaml:"auth-types,omitempty,flow" json:"auth-types,omitempty"`
	Endpoint         string   `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	IdentityEndpoint string   `yaml:"identity-endpoint,omitempty" json:"identity-endpoint,omitempty"`
	StorageEndpoint  string   `yaml:"storage-endpoint,omitempty" json:"storage-endpoint,omitempty"`
	// Regions is for when we want to print regions in order for yaml output.
	Regions yaml.MapSlice `yaml:"regions,omitempty" json:"-"`
	// Regions map is for json marshalling where format is important but not order.
	RegionsMap   map[string]regionDetails `yaml:"-" json:"regions,omitempty"`
	Config       map[string]interface{}   `yaml:"config,omitempty" json:"config,omitempty"`
	RegionConfig jujucloud.RegionConfig   `yaml:"region-config,omitempty" json:"region-config,omitempty"`
}

type cloudConfig struct {
	Type        string `yaml:"type,omitempty" json:"type,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
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
		CloudDescription: cloud.Description,
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

func getCloudConfigDetails(cloudType string) map[string]interface{} {
	// providerSchema has all config options, including their descriptions
	// and types.
	providerSchema, err := common.CloudSchemaByType(cloudType)
	if err != nil {
		// Some providers do not implement the ProviderSchema interface.
		return nil
	}
	specifics := make(map[string]interface{})
	ps, err := common.ProviderConfigSchemaSourceByType(cloudType)
	if err != nil {
		// Some providers do not implement the ConfigSchema interface.
		return nil
	}
	// ps.ConfigSchema() returns the provider specific config option names, but no
	// description etc.
	for attr := range ps.ConfigSchema() {
		if providerSchema[attr].Secret {
			continue
		}
		specifics[attr] = cloudConfig{
			Description: providerSchema[attr].Description,
			Type:        fmt.Sprintf("%s", providerSchema[attr].Type),
		}
	}
	return specifics
}

func getCloudDetails() (map[string]*cloudDetails, error) {
	result, err := listCloudDetails()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result.all(), nil
}
