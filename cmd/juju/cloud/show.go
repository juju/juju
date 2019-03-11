// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"
	"gopkg.in/yaml.v2"

	cloudapi "github.com/juju/juju/api/cloud"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

type showCloudCommand struct {
	modelcmd.CommandBase
	out cmd.Output

	CloudName string

	includeConfig bool

	// Used when querying a controller for its cloud details
	controllerName   string
	store            jujuclient.ClientStore
	showCloudAPIFunc func(controllerName string) (showCloudAPI, error)
}

var showCloudDoc = `
Provided information includes 'defined' (public, built-in), 'type',
'auth-type', 'regions', 'endpoints', and cloud specific configuration
options.

If ‘--include-config’ is used, additional configuration (key, type, and
description) specific to the cloud are displayed if available.

If you supply a controller name the cloud information will be queried from the
controller.

Examples:

    juju show-cloud google
    juju show-cloud azure-china --output ~/azure_cloud_details.txt
    juju show-cloud myopenstack --controller mycontroller

See also:
    clouds
    update-clouds
`

type showCloudAPI interface {
	Cloud(tag names.CloudTag) (jujucloud.Cloud, error)
	Close() error
}

// NewShowCloudCommand returns a command to list cloud information.
func NewShowCloudCommand() cmd.Command {
	c := &showCloudCommand{
		store: jujuclient.NewFileClientStore(),
	}
	c.showCloudAPIFunc = c.cloudAPI
	return modelcmd.WrapBase(c)
}

func (c *showCloudCommand) cloudAPI(controllerName string) (showCloudAPI, error) {
	root, err := c.NewAPIRoot(c.store, controllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cloudapi.NewClient(root), nil
}

func (c *showCloudCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	// We only support yaml for display purposes.
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
	})
	f.BoolVar(&c.includeConfig, "include-config", false, "Print available config option details specific to the specified cloud")
	f.StringVar(&c.controllerName, "c", "", "Controller to operate in")
	f.StringVar(&c.controllerName, "controller", "", "")
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
	return jujucmd.Info(&cmd.Info{
		Name:    "show-cloud",
		Args:    "<cloud name>",
		Purpose: "Shows detailed information on a cloud.",
		Doc:     showCloudDoc,
	})
}

func (c *showCloudCommand) Run(ctxt *cmd.Context) error {
	var (
		cloud *CloudDetails
		err   error
	)
	if c.controllerName == "" {
		cloud, err = c.getLocalCloud()
	} else {
		cloud, err = c.getControllerCloud()
	}
	if err != nil {
		return err
	}

	if err := c.out.Write(ctxt, cloud); err != nil {
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

func (c *showCloudCommand) getControllerCloud() (*CloudDetails, error) {
	api, err := c.showCloudAPIFunc(c.controllerName)
	if err != nil {
		return nil, err
	}
	defer api.Close()
	controllerCloud, err := api.Cloud(names.NewCloudTag(c.CloudName))
	if err != nil {
		return nil, err
	}
	cloud := makeCloudDetails(controllerCloud)
	return cloud, nil
}

func (c *showCloudCommand) getLocalCloud() (*CloudDetails, error) {
	details, err := GetAllCloudDetails()
	if err != nil {
		return nil, err
	}
	cloud, ok := details[c.CloudName]
	if !ok {
		return nil, errors.NotFoundf("cloud %q", c.CloudName)
	}
	return cloud, nil
}

// RegionDetails holds region details.
type RegionDetails struct {
	Name             string `yaml:"-" json:"-"`
	Endpoint         string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	IdentityEndpoint string `yaml:"identity-endpoint,omitempty" json:"identity-endpoint,omitempty"`
	StorageEndpoint  string `yaml:"storage-endpoint,omitempty" json:"storage-endpoint,omitempty"`
}

// CloudDetails holds cloud details.
type CloudDetails struct {
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
	RegionsMap    map[string]RegionDetails `yaml:"-" json:"regions,omitempty"`
	Config        map[string]interface{}   `yaml:"config,omitempty" json:"config,omitempty"`
	RegionConfig  jujucloud.RegionConfig   `yaml:"region-config,omitempty" json:"region-config,omitempty"`
	CACredentials []string                 `yaml:"ca-credentials,omitempty" json:"ca-credentials,omitempty"`
}

func makeCloudDetails(cloud jujucloud.Cloud) *CloudDetails {
	result := &CloudDetails{
		Source:           "public",
		CloudType:        cloud.Type,
		Endpoint:         cloud.Endpoint,
		IdentityEndpoint: cloud.IdentityEndpoint,
		StorageEndpoint:  cloud.StorageEndpoint,
		Config:           cloud.Config,
		RegionConfig:     cloud.RegionConfig,
		CloudDescription: cloud.Description,
		CACredentials:    cloud.CACertificates,
	}
	result.AuthTypes = make([]string, len(cloud.AuthTypes))
	for i, at := range cloud.AuthTypes {
		result.AuthTypes[i] = string(at)
	}
	result.RegionsMap = make(map[string]RegionDetails)
	for _, region := range cloud.Regions {
		r := RegionDetails{Name: region.Name}
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
		specifics[attr] = common.PrintConfigSchema{
			Description: providerSchema[attr].Description,
			Type:        fmt.Sprintf("%s", providerSchema[attr].Type),
		}
	}
	return specifics
}

// GetAllCloudDetails returns a list of all cloud details.
func GetAllCloudDetails() (map[string]*CloudDetails, error) {
	result, err := listCloudDetails()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result.all(), nil
}
