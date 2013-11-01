// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"io/ioutil"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
)

var configFields = schema.Fields{
	"location":                    schema.String(),
	"management-subscription-id":  schema.String(),
	"management-certificate-path": schema.String(),
	"management-certificate":      schema.String(),
	"storage-account-name":        schema.String(),
	"image-stream":                schema.String(),
	"force-image-name":            schema.String(),
}
var configDefaults = schema.Defaults{
	"location":                    "",
	"management-certificate":      "",
	"management-certificate-path": "",
	// The default is blank, which means "use the first of the base URLs
	// that has a matching image."  The first base URL is for "released",
	// which is what we want, but also a blank default will be easier on
	// the user if we later make the list of base URLs configurable.
	"image-stream":     "",
	"force-image-name": "",
}

type azureEnvironConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (cfg *azureEnvironConfig) location() string {
	return cfg.attrs["location"].(string)
}

func (cfg *azureEnvironConfig) managementSubscriptionId() string {
	return cfg.attrs["management-subscription-id"].(string)
}

func (cfg *azureEnvironConfig) managementCertificate() string {
	return cfg.attrs["management-certificate"].(string)
}

func (cfg *azureEnvironConfig) storageAccountName() string {
	return cfg.attrs["storage-account-name"].(string)
}

func (cfg *azureEnvironConfig) imageStream() string {
	return cfg.attrs["image-stream"].(string)
}

func (cfg *azureEnvironConfig) forceImageName() string {
	return cfg.attrs["force-image-name"].(string)
}

func (prov azureEnvironProvider) newConfig(cfg *config.Config) (*azureEnvironConfig, error) {
	validCfg, err := prov.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	result := new(azureEnvironConfig)
	result.Config = validCfg
	result.attrs = validCfg.UnknownAttrs()
	return result, nil
}

// Validate ensures that config is a valid configuration for this
// provider like specified in the EnvironProvider interface.
func (prov azureEnvironProvider) Validate(cfg, oldCfg *config.Config) (*config.Config, error) {
	// Validate base configuration change before validating Azure specifics.
	err := config.Validate(cfg, oldCfg)
	if err != nil {
		return nil, err
	}

	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}
	envCfg := new(azureEnvironConfig)
	envCfg.Config = cfg
	envCfg.attrs = validated

	cert := envCfg.managementCertificate()
	if cert == "" {
		certPath := envCfg.attrs["management-certificate-path"].(string)
		pemData, err := ioutil.ReadFile(certPath)
		if err != nil {
			return nil, fmt.Errorf("invalid management-certificate-path: %s", err)
		}
		envCfg.attrs["management-certificate"] = string(pemData)
	}
	delete(envCfg.attrs, "management-certificate-path")
	if envCfg.location() == "" {
		return nil, fmt.Errorf("environment has no location; you need to set one.  E.g. 'West US'")
	}
	return cfg.Apply(envCfg.attrs)
}

// TODO(jtv): Once we have "released" images for Azure, update the provisional
// image-stream and default-series settings.
const boilerplateYAML = `
# https://juju.ubuntu.com/docs/config-azure.html
azure:
    type: azure

    # location specifies the place where instances will be started, for
    # example: West US, North Europe.
    location: West US
    
    # The following attributes specify Windows Azure Management information.
    # See  http://msdn.microsoft.com/en-us/library/windowsazure
    # for details.
    management-subscription-id: <00000000-0000-0000-0000-000000000000>
    management-certificate-path: /home/me/azure.pem

    # storage-account-name holds Windows Azure Storage info.
    storage-account-name: abcdefghijkl

    # force-image-name overrides the OS image selection to use
    # a fixed image for all deployments. Most useful for developers.
    # force-image-name: b39f27a8b8c64d52b05eac6a62ebad85__Ubuntu-13_10-amd64-server-DEVELOPMENT-20130713-Juju_ALPHA-en-us-30GB
    
    # image-stream chooses a simplestreams stream to select OS images from,
    # for example daily or released images (or any other stream available on simplestreams).
    # Leave blank for released images.
    # image-stream: ""
`

func (prov azureEnvironProvider) BoilerplateConfig() string {
	return boilerplateYAML
}

// SecretAttrs is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	secretAttrs := make(map[string]string)
	azureCfg, err := prov.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	secretAttrs["management-certificate"] = azureCfg.managementCertificate()
	return secretAttrs, nil
}
