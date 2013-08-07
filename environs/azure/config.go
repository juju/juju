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
	"location":                      schema.String(),
	"management-subscription-id":    schema.String(),
	"management-certificate-path":   schema.String(),
	"management-certificate":        schema.String(),
	"storage-account-name":          schema.String(),
	"public-storage-account-name":   schema.String(),
	"public-storage-container-name": schema.String(),
	"image-stream":                  schema.String(),
	"force-image-name":              schema.String(),
}
var configDefaults = schema.Defaults{
	"location":                      "",
	"management-certificate":        "",
	"management-certificate-path":   "",
	"public-storage-account-name":   "",
	"public-storage-container-name": "",
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

func (cfg *azureEnvironConfig) publicStorageContainerName() string {
	return cfg.attrs["public-storage-container-name"].(string)
}

func (cfg *azureEnvironConfig) publicStorageAccountName() string {
	return cfg.attrs["public-storage-account-name"].(string)
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
	if (envCfg.publicStorageAccountName() == "") != (envCfg.publicStorageContainerName() == "") {
		return nil, fmt.Errorf("public-storage-account-name and public-storage-container-name must be specified both or none of them")
	}

	return cfg.Apply(envCfg.attrs)
}

// TODO: Once we have "released" images for Azure, retire the provisional
// image-stream setting.
const boilerplateYAML = `azure:
  type: azure
  admin-secret: {{rand}}
  # Location for instances, e.g. West US, North Europe.
  location: West US
  # http://msdn.microsoft.com/en-us/library/windowsazure
  # Windows Azure Management info.
  management-subscription-id: 886413e1-3b8a-5382-9b90-0c9aee199e5d
  management-certificate-path: /home/me/azure.pem
  # Windows Azure Storage info.
  storage-account-name: ghedlkjhw54e
  # Public Storage info (account name and container name) denoting a public
  # container holding the juju tools.
  # public-storage-account-name: public-storage-account
  # public-storage-container-name: public-storage-container-name
  # Override OS image selection with a fixed image for all deployments.
  # Most useful for developers.
  # force-image-name: b39f27a8b8c64d52b05eac6a62ebad85__Ubuntu-13_10-amd64-server-DEVELOPMENT-20130713-Juju_ALPHA-en-us-30GB
  # Pick a simplestreams stream to select OS images from: "daily" or "released".
  # Leaving this blank will look for any suitable image, but prefer released
  # images over daily ones.
  #image-stream: daily
`

func (prov azureEnvironProvider) BoilerplateConfig() string {
	return boilerplateYAML
}

// SecretAttrs is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) SecretAttrs(cfg *config.Config) (map[string]interface{}, error) {
	secretAttrs := make(map[string]interface{})
	azureCfg, err := prov.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	secretAttrs["management-certificate"] = azureCfg.managementCertificate()
	return secretAttrs, nil
}
