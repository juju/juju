// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
)

var azureConfigChecker = schema.StrictFieldMap(
	schema.Fields{
		"management-subscription-id":     schema.String(),
		"management-certificate-path":    schema.String(),
		"management-hosted-service-name": schema.String(),
		"storage-account-name":           schema.String(),
		"storage-account-key":            schema.String(),
		"storage-container-name":         schema.String(),
	},
	schema.Defaults{
		"management-hosted-service-name": "",
		"storage-container-name":         "",
	},
)

type azureEnvironConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (cfg *azureEnvironConfig) ManagementSubscriptionId() string {
	return cfg.attrs["management-subscription-id"].(string)
}

func (cfg *azureEnvironConfig) ManagementCertificatePath() string {
	return cfg.attrs["management-certificate-path"].(string)
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

// Validate is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) Validate(cfg, oldCfg *config.Config) (*config.Config, error) {
	// Validate base configuration change before validating Azure specifics.
	err := config.Validate(cfg, oldCfg)
	if err != nil {
		return nil, err
	}

	v, err := azureConfigChecker.Coerce(cfg.UnknownAttrs(), nil)
	if err != nil {
		return nil, err
	}
	envCfg := new(azureEnvironConfig)
	envCfg.Config = cfg
	envCfg.attrs = v.(map[string]interface{})

	// Validate management-certificate-path: must be a path to an existing file.
	certPath := envCfg.ManagementCertificatePath()
	_, err = ioutil.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("invalid management-certificate-path: %s", err)
	}
	if envCfg.ManagementHostedServiceName() == "" {
		return nil, fmt.Errorf("environment has no management-hosted-service-name; auto-creation of hosted services is not yet supported")
	}
	if envCfg.StorageContainerName() == "" {
		return nil, fmt.Errorf("environment has no storage-container-name; auto-creation of storage containers is not yet supported")
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
`

func (prov azureEnvironProvider) BoilerplateConfig() string {
	return boilerplateYAML
}

// SecretAttrs is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) SecretAttrs(cfg *config.Config) (map[string]interface{}, error) {
	panic("unimplemented")
}
