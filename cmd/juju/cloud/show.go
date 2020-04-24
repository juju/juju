// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"gopkg.in/yaml.v2"

	cloudapi "github.com/juju/juju/api/cloud"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

type showCloudCommand struct {
	modelcmd.OptionalControllerCommand
	out cmd.Output

	CloudName string

	includeConfig bool

	showCloudAPIFunc func() (showCloudAPI, error)

	configDisplayed bool
}

var showCloudDoc = `
Provided information includes 'defined' (public, built-in), 'type',
'auth-type', 'regions', 'endpoints', and cloud specific configuration
options.

If ‘--include-config’ is used, additional configuration (key, type, and
description) specific to the cloud are displayed if available.

Use --controller option to show a cloud from a controller.

Use --client option to show a cloud known on this client.

Examples:

    juju show-cloud google
    juju show-cloud azure-china --output ~/azure_cloud_details.txt
    juju show-cloud myopenstack --controller mycontroller
    juju show-cloud myopenstack --client
    juju show-cloud myopenstack --client --controller mycontroller

See also:
    clouds
    add-cloud
    update-cloud
`

type showCloudAPI interface {
	Cloud(tag names.CloudTag) (jujucloud.Cloud, error)
	Close() error
}

// NewShowCloudCommand returns a command to list cloud information.
func NewShowCloudCommand() cmd.Command {
	store := jujuclient.NewFileClientStore()
	c := &showCloudCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store:    store,
			ReadOnly: true,
		},
	}
	c.showCloudAPIFunc = c.cloudAPI
	return modelcmd.WrapBase(c)
}

func (c *showCloudCommand) cloudAPI() (showCloudAPI, error) {
	root, err := c.NewAPIRoot(c.Store, c.ControllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cloudapi.NewClient(root), nil
}

func (c *showCloudCommand) SetFlags(f *gnuflag.FlagSet) {
	c.OptionalControllerCommand.SetFlags(f)
	// We only support yaml for display purposes.
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
	})
	f.BoolVar(&c.includeConfig, "include-config", false, "Print available config option details specific to the specified cloud")
}

func (c *showCloudCommand) Init(args []string) error {
	if err := c.OptionalControllerCommand.Init(args); err != nil {
		return err
	}
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
		Purpose: "Shows detailed information for a cloud.",
		Doc:     showCloudDoc,
	})
}

func (c *showCloudCommand) Run(ctxt *cmd.Context) error {
	if err := c.MaybePrompt(ctxt, fmt.Sprintf("show cloud %q from", c.CloudName)); err != nil {
		return errors.Trace(err)
	}
	var (
		localCloud *CloudDetails
		localErr   error
	)
	if c.Client {
		localCloud, localErr = c.getLocalCloud()
	}

	var displayErr error
	if c.ControllerName != "" {
		remoteCloud, remoteErr := c.getControllerCloud()
		showRemoteConfig := c.includeConfig
		if localCloud != nil && remoteCloud != nil {
			// It's possible that a local cloud named A is different to
			// a remote cloud named A. If their types are different and we
			// need to display config, we'd need to list each cloud type config.
			// If the clouds' types are the same, we only need to list
			// config once after the local cloud information.
			showRemoteConfig = showRemoteConfig && localCloud.CloudType != remoteCloud.CloudType
		}
		if remoteCloud != nil {
			if err := c.displayCloud(ctxt, remoteCloud, fmt.Sprintf("Cloud %q from controller %q:\n", c.CloudName, c.ControllerName), showRemoteConfig, remoteErr); err != nil {
				ctxt.Infof("ERROR %v", err)
				displayErr = cmd.ErrSilent
			}
		} else {
			if remoteErr != nil {
				ctxt.Infof("ERROR %v", remoteErr)
				displayErr = cmd.ErrSilent
			} else {
				ctxt.Infof("No cloud %q exists on the controller.", c.CloudName)
			}
		}
	}
	if c.Client {
		if localCloud != nil {
			if err := c.displayCloud(ctxt, localCloud, fmt.Sprintf("\nClient cloud %q:\n", c.CloudName), c.includeConfig, localErr); err != nil {
				ctxt.Infof("ERROR %v", err)
				displayErr = cmd.ErrSilent
			}
		} else {
			if localErr != nil {
				ctxt.Infof("ERROR %v", localErr)
				displayErr = cmd.ErrSilent
			} else {
				ctxt.Infof("No cloud %q exists on this client.", c.CloudName)
			}
		}
	}

	// It's possible that a config was desired but was not display because the
	// remote cloud erred out.
	if c.includeConfig && !c.configDisplayed && localErr == nil {
		if err := c.displayConfig(ctxt, localCloud.CloudType); err != nil {
			return err
		}
	}

	return displayErr
}

func (c *showCloudCommand) displayCloud(ctxt *cmd.Context, aCloud *CloudDetails, msg string, includeConfig bool, cloudErr error) error {
	if cloudErr != nil {
		return cloudErr
	}
	fmt.Fprintln(ctxt.Stdout, msg)
	aCloud.CloudType = displayCloudType(aCloud.CloudType)
	if err := c.out.Write(ctxt, aCloud); err != nil {
		return err
	}
	if includeConfig {
		return c.displayConfig(ctxt, aCloud.CloudType)
	}
	return nil
}

func (c *showCloudCommand) displayConfig(ctxt *cmd.Context, cloudType string) error {
	config := getCloudConfigDetails(cloudType)
	if len(config) > 0 {
		fmt.Fprintln(
			ctxt.Stdout,
			fmt.Sprintf("\nThe available config options specific to %s clouds are:", cloudType))
		if err := c.out.Write(ctxt, config); err != nil {
			return err
		}
		c.configDisplayed = true
	}
	return nil
}

func (c *showCloudCommand) getControllerCloud() (*CloudDetails, error) {
	api, err := c.showCloudAPIFunc()
	if err != nil {
		return nil, err
	}
	defer api.Close()
	controllerCloud, err := api.Cloud(names.NewCloudTag(c.CloudName))
	if err != nil {
		return nil, err
	}
	cloud := makeCloudDetails(c.Store, controllerCloud)
	return cloud, nil
}

func (c *showCloudCommand) getLocalCloud() (*CloudDetails, error) {
	details, err := GetAllCloudDetails(c.Store)
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
	CloudDescription string   `yaml:"description,omitempty" json:"description,omitempty"`
	AuthTypes        []string `yaml:"auth-types,omitempty,flow" json:"auth-types,omitempty"`
	Endpoint         string   `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	IdentityEndpoint string   `yaml:"identity-endpoint,omitempty" json:"identity-endpoint,omitempty"`
	StorageEndpoint  string   `yaml:"storage-endpoint,omitempty" json:"storage-endpoint,omitempty"`
	// DefaultRegion is a default region as known to this client.
	DefaultRegion string `yaml:"default-region,omitempty" json:"default-region,omitempty"`
	// CredentialCount contains the number of credentials that exist for this cloud on this client.
	CredentialCount int `yaml:"credential-count,omitempty" json:"credential-count,omitempty"`
	// Regions is for when we want to print regions in order for yaml output.
	Regions yaml.MapSlice `yaml:"regions,omitempty" json:"-"`
	// Regions map is for json marshalling where format is important but not order.
	RegionsMap    map[string]RegionDetails `yaml:"-" json:"regions,omitempty"`
	Config        map[string]interface{}   `yaml:"config,omitempty" json:"config,omitempty"`
	RegionConfig  jujucloud.RegionConfig   `yaml:"region-config,omitempty" json:"region-config,omitempty"`
	CACredentials []string                 `yaml:"ca-credentials,omitempty" json:"ca-credentials,omitempty"`
}

func makeCloudDetails(store jujuclient.CredentialGetter, cloud jujucloud.Cloud) *CloudDetails {
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
	if cred, err := store.CredentialForCloud(cloud.Name); err == nil {
		result.DefaultRegion = cred.DefaultRegion
		result.CredentialCount = len(cred.AuthCredentials)
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
func GetAllCloudDetails(store jujuclient.CredentialGetter) (map[string]*CloudDetails, error) {
	result, err := listLocalCloudDetails(store)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result.all(), nil
}
