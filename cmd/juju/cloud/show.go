// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"
	"gopkg.in/yaml.v2"

	cloudapi "github.com/juju/juju/api/client/cloud"
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
Provided information includes ` + "`defined`" + ` (public, built-in), ` + "`type`" + `,
` + "`auth-type`" + `, ` + "`regions`" + `, ` + "`endpoints`" + `, and cloud specific configuration
options.

If ` + "`--include-config`" + ` is used, additional configuration (key, type, and
description) specific to the cloud are displayed if available.

Use the ` + "`--controller`" + ` option to show a cloud from a controller.

Use the ` + "`--client`" + ` option to show a cloud known on this client.
`

const showCloudExamples = `
    juju show-cloud google
    juju show-cloud azure-china --output ~/azure_cloud_details.txt
    juju show-cloud myopenstack --controller mycontroller
    juju show-cloud myopenstack --client
    juju show-cloud myopenstack --client --controller mycontroller
`

type showCloudAPI interface {
	CloudInfo(tags []names.CloudTag) ([]cloudapi.CloudInfo, error)
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
	c.out.AddFlags(f, "display", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"display": cmd.FormatYaml,
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
		Name:     "show-cloud",
		Args:     "<cloud name>",
		Purpose:  "Shows detailed information for a cloud.",
		Doc:      showCloudDoc,
		Examples: showCloudExamples,
		SeeAlso: []string{
			"clouds",
			"add-cloud",
			"update-cloud",
		},
	})
}

func (c *showCloudCommand) Run(ctxt *cmd.Context) error {
	if err := c.MaybePrompt(ctxt, fmt.Sprintf("show cloud %q from", c.CloudName)); err != nil {
		return errors.Trace(err)
	}

	var (
		localCloud *CloudDetails
		localErr   error
		displayErr error
		outputs    []CloudOutput
	)

	if c.Client {
		localCloud, localErr = c.getLocalCloud()
	}
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

		if remoteErr != nil {
			ctxt.Infof("ERROR %v", remoteErr)
			displayErr = cmd.ErrSilent
		} else if remoteCloud != nil {
			outputs = append(outputs, c.displayCloud(
				*remoteCloud,
				c.CloudName,
				fmt.Sprintf("Cloud %q from controller %q", c.CloudName, c.ControllerName),
				showRemoteConfig,
			))
		} else {
			ctxt.Infof("No cloud %q exists on the controller.", c.CloudName)
		}
	}
	if c.Client {
		if localErr != nil {
			ctxt.Infof("ERROR %v", localErr)
			displayErr = cmd.ErrSilent
		} else if localCloud != nil {
			outputs = append(outputs, c.displayCloud(
				*localCloud,
				c.CloudName,
				fmt.Sprintf("Client cloud %q", c.CloudName),
				c.includeConfig,
			))
		} else {
			ctxt.Infof("No cloud %q exists on this client.", c.CloudName)
		}
	}

	// It's possible that a config was desired but was not display because the
	// remote cloud erred out.
	if c.includeConfig && !c.configDisplayed && localCloud != nil && localErr == nil {
		outputs = append(outputs, CloudOutput{
			Name:    c.CloudName,
			Summary: fmt.Sprintf("Client cloud %q", c.CloudName),
			Config:  getCloudConfigDetails(localCloud.CloudType),
		})
	}

	if len(outputs) == 0 {
		return displayErr
	}

	switch c.out.Name() {
	case "display":
		for _, output := range outputs {
			var written bool
			if !output.CloudDetails.Empty() {
				fmt.Fprintf(ctxt.Stdout, "%s:\n\n", output.Summary)

				if err := c.out.Write(ctxt, output.CloudDetails); err != nil {
					return errors.Trace(err)
				}
				written = true
			}

			if len(output.Config) > 0 {
				fmt.Fprintln(ctxt.Stdout)
				fmt.Fprintf(ctxt.Stdout, "The available config options specific to %s clouds are:\n", output.CloudType)

				if err := c.out.Write(ctxt, output.Config); err != nil {
					return errors.Trace(err)
				}
				written = true
			}
			if written && len(outputs) > 1 {
				fmt.Fprintln(ctxt.Stdout)
			}
		}
	case "yaml":
		for _, output := range outputs {
			if err := c.out.Write(ctxt, output); err != nil {
				return errors.Trace(err)
			}
			if len(outputs) > 1 {
				fmt.Fprintln(ctxt.Stdout, "---")
			}
		}
	case "json":
		if err := c.out.Write(ctxt, outputs); err != nil {
			return errors.Trace(err)
		}
	}

	return displayErr
}

func (c *showCloudCommand) displayCloud(aCloud CloudDetails, name, summary string, includeConfig bool) CloudOutput {
	aCloud.CloudType = displayCloudType(aCloud.CloudType)
	var config map[string]any
	if includeConfig {
		config = getCloudConfigDetails(aCloud.CloudType)
		c.configDisplayed = true
	}
	return CloudOutput{
		Name:         name,
		Summary:      summary,
		CloudDetails: aCloud,
		Config:       config,
	}
}

func (c *showCloudCommand) getControllerCloud() (*CloudDetails, error) {
	api, err := c.showCloudAPIFunc()
	if err != nil {
		return nil, err
	}
	defer api.Close()
	controllerCloud, err := api.CloudInfo([]names.CloudTag{names.NewCloudTag(c.CloudName)})
	if err != nil {
		return nil, err
	}
	cloud := makeCloudDetailsForUser(c.Store, controllerCloud[0])
	return cloud, nil
}

func (c *showCloudCommand) getLocalCloud() (*CloudDetails, error) {
	details, err := GetAllCloudDetails(c.Store)
	if err != nil {
		return nil, err
	}
	cloud, ok := details[c.CloudName]
	if !ok {
		alternatives := make([]string, 0, len(details))
		for name := range details {
			alternatives = append(alternatives, name)
		}
		if len(alternatives) == 0 {
			return nil, errors.NotFoundf("cloud %s", c.CloudName)
		}
		sort.Strings(alternatives)
		names := strings.Join(alternatives, "\n  - ")
		return nil, errors.NewNotFound(nil, fmt.Sprintf("cloud %s not found, possible alternative clouds:\n\n  - %s", c.CloudName, names))
	}
	return cloud, nil
}

type CloudOutput struct {
	Name         string `yaml:"name,omitempty" json:"name,omitempty"`
	Summary      string `yaml:"summary,omitempty" json:"summary,omitempty"`
	CloudDetails `yaml:",inline" json:",inline"`
	Config       map[string]any `yaml:"cloud-config,omitempty" json:"cloud-config,omitempty"`
}

// RegionDetails holds region details.
type RegionDetails struct {
	Name             string `yaml:"-" json:"-"`
	Endpoint         string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	IdentityEndpoint string `yaml:"identity-endpoint,omitempty" json:"identity-endpoint,omitempty"`
	StorageEndpoint  string `yaml:"storage-endpoint,omitempty" json:"storage-endpoint,omitempty"`
}

// CloudUserInfo holds user access info for a cloud.
type CloudUserInfo struct {
	DisplayName string `yaml:"display-name,omitempty" json:"display-name,omitempty"`
	Access      string `yaml:"access" json:"access"`
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
	Config        map[string]any           `yaml:"config,omitempty" json:"config,omitempty"`
	RegionConfig  jujucloud.RegionConfig   `yaml:"region-config,omitempty" json:"region-config,omitempty"`
	CACredentials []string                 `yaml:"ca-credentials,omitempty" json:"ca-credentials,omitempty"`
	Users         map[string]CloudUserInfo `json:"users,omitempty" yaml:"users,omitempty"`
	SkipTLSVerify bool                     `yaml:"skip-tls-verify,omitempty" json:"skip-tls-verify,omitempty"`
}

func (d *CloudDetails) Empty() bool {
	return d.Source == "" &&
		d.CloudType == "" &&
		d.CloudDescription == "" &&
		len(d.AuthTypes) == 0 &&
		d.Endpoint == "" &&
		d.IdentityEndpoint == "" &&
		d.StorageEndpoint == "" &&
		d.DefaultRegion == "" &&
		d.CredentialCount == 0 &&
		len(d.Regions) == 0 &&
		len(d.RegionsMap) == 0 &&
		len(d.Config) == 0 &&
		len(d.RegionConfig) == 0 &&
		len(d.CACredentials) == 0 &&
		len(d.Users) == 0 &&
		!d.SkipTLSVerify
}

func makeCloudDetails(store jujuclient.CredentialGetter, cloud jujucloud.Cloud) *CloudDetails {
	return makeCloudDetailsForUser(store, cloudapi.CloudInfo{Cloud: cloud})
}

func makeCloudDetailsForUser(store jujuclient.CredentialGetter, cloud cloudapi.CloudInfo) *CloudDetails {
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
		Users:            make(map[string]CloudUserInfo),
		SkipTLSVerify:    cloud.SkipTLSVerify,
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
		result.Regions = append(result.Regions, yaml.MapItem{Key: r.Name, Value: r})
		result.RegionsMap[region.Name] = r
	}
	if cred, err := store.CredentialForCloud(cloud.Name); err == nil {
		result.DefaultRegion = cred.DefaultRegion
		result.CredentialCount = len(cred.AuthCredentials)
	}
	for name, user := range cloud.Users {
		result.Users[name] = CloudUserInfo{
			DisplayName: user.DisplayName,
			Access:      user.Access,
		}
	}
	return result
}

func getCloudConfigDetails(cloudType string) map[string]any {
	// providerSchema has all config options, including their descriptions
	// and types.
	providerSchema, err := common.CloudSchemaByType(cloudType)
	if err != nil {
		// Some providers do not implement the ProviderSchema interface.
		return nil
	}
	specifics := make(map[string]any)
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
			Type:        string(providerSchema[attr].Type),
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
