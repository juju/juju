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
	"management-subscription-id":     schema.String(),
	"management-certificate-path":    schema.String(),
	"management-certificate":         schema.String(),
	"management-hosted-service-name": schema.String(),
	"storage-account-name":           schema.String(),
	"storage-account-key":            schema.String(),
	"storage-container-name":         schema.String(),
	"public-storage-account-name":    schema.String(),
	"public-storage-container-name":  schema.String(),
}
var configDefaults = schema.Defaults{
	"management-hosted-service-name": "",
	"management-certificate":         "",
	"management-certificate-path":    "",
	"storage-container-name":         "",
	"public-storage-account-name":    "",
	"public-storage-container-name":  "",
}

type azureEnvironConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (cfg *azureEnvironConfig) ManagementSubscriptionId() string {
	return cfg.attrs["management-subscription-id"].(string)
}

func (cfg *azureEnvironConfig) ManagementCertificate() string {
	return cfg.attrs["management-certificate"].(string)
}

func (cfg *azureEnvironConfig) ManagementHostedServiceName() string {
	return cfg.attrs["management-hosted-service-name"].(string)
}

func (cfg *azureEnvironConfig) StorageAccountName() string {
	return cfg.attrs["storage-account-name"].(string)
}

func (cfg *azureEnvironConfig) StorageAccountKey() string {
	return cfg.attrs["storage-account-key"].(string)
}

func (cfg *azureEnvironConfig) StorageContainerName() string {
	return cfg.attrs["storage-container-name"].(string)
}

func (cfg *azureEnvironConfig) PublicStorageContainerName() string {
	return cfg.attrs["public-storage-container-name"].(string)
}

func (cfg *azureEnvironConfig) PublicStorageAccountName() string {
	return cfg.attrs["public-storage-account-name"].(string)
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

	cert := envCfg.ManagementCertificate()
	if cert == "" {
		certPath := envCfg.attrs["management-certificate-path"].(string)
		pemData, err := ioutil.ReadFile(certPath)
		if err != nil {
			return nil, fmt.Errorf("invalid management-certificate-path: %s", err)
		}
		envCfg.attrs["management-certificate"] = string(pemData)
	}
	delete(envCfg.attrs, "management-certificate-path")
	if envCfg.ManagementHostedServiceName() == "" {
		return nil, fmt.Errorf("environment has no management-hosted-service-name; auto-creation of hosted services is not yet supported")
	}
	if envCfg.StorageContainerName() == "" {
		return nil, fmt.Errorf("environment has no storage-container-name; auto-creation of storage containers is not yet supported")
	}
	if (envCfg.PublicStorageAccountName() == "") != (envCfg.PublicStorageContainerName() == "") {
		return nil, fmt.Errorf("public-storage-account-name and public-storage-container-name must be specified both or none of them")
	}
	if oldCfg != nil {
		attrs := oldCfg.UnknownAttrs()
		if hostedServiceName, _ := attrs["management-hosted-service-name"].(string); envCfg.ManagementHostedServiceName() != hostedServiceName {
			return nil, fmt.Errorf("cannot change management-hosted-service-name from %q to %q", hostedServiceName, envCfg.ManagementHostedServiceName())
		}
		if storageContainerName, _ := attrs["storage-container-name"].(string); envCfg.StorageContainerName() != storageContainerName {
			return nil, fmt.Errorf("cannot change storage-container-name from %q to %q", storageContainerName, envCfg.StorageContainerName())
		}
	}

	return cfg.Apply(envCfg.attrs)
}

const boilerplateYAML = `azure:
  type: azure
  # http://msdn.microsoft.com/en-us/library/windowsazure
  # Windows Azure Management info.
  management-subscription-id: 886413e1-3b8a-5382-9b90-0c9aee199e5d
  management-certificate-path: /home/me/azure.pem
  management-hosted-service-name: gwaclbize3r9qh67ro6qbgvm
  # Windows Azure Storage info.
  storage-account-name: ghedlkjhw54e
  storage-account-key: fdjh4sfkg
  storage-container-name: sdg50984jmsdf
  # Public Storage info (account name and container name) denoting a public
  # container holding the juju tools.
  # public-storage-account-name: public-storage-account
  # public-storage-container-name: public-storage-container-name
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
	secretAttrs["management-certificate"] = azureCfg.ManagementCertificate()
	secretAttrs["storage-account-key"] = azureCfg.StorageAccountKey()
	return secretAttrs, nil
}
