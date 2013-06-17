// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/testing"
	"strings"
)

type ConfigSuite struct{}

var _ = Suite(new(ConfigSuite))

// makeBaseConfigMap creates a minimal map of standard configuration items.
// It's just the bare minimum to produce a configuration object.
func makeBaseConfigMap() map[string]interface{} {
	return map[string]interface{}{
		"name":           "testenv",
		"type":           "azure",
		"ca-cert":        testing.CACert,
		"ca-private-key": testing.CAKey,
	}
}

func makeConfigMap(configMap map[string]interface{}) map[string]interface{} {
	conf := makeBaseConfigMap()
	for k, v := range configMap {
		conf[k] = v
	}
	return conf
}

func makeAzureConfigMap(c *C) map[string]interface{} {
	managementCertificatePath := createTempFile(c)
	azureConfig := map[string]interface{}{
		"management-subscription-id":     "subscription-id",
		"management-certificate-path":    managementCertificatePath,
		"management-hosted-service-name": "service-name",
		"storage-account-name":           "account-name",
		"storage-account-key":            "account-key",
		"storage-container-name":         "container-name",
	}
	return makeConfigMap(azureConfig)
}

// createTempFile creates a temporary empty file.  The file will be cleaned
// up at the end of the test calling this method.
func createTempFile(c *C) string {
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, IsNil)
	filename := file.Name()
	return filename
}

func (ConfigSuite) TestValidateAcceptsNilOldConfig(c *C) {
	attrs := makeAzureConfigMap(c)
	provider := azureEnvironProvider{}
	config, err := config.New(attrs)
	c.Assert(err, IsNil)
	result, err := provider.Validate(config, nil)
	c.Assert(err, IsNil)
	c.Check(result.Name(), Equals, attrs["name"])
}

func (ConfigSuite) TestValidateAcceptsUnchangedConfig(c *C) {
	attrs := makeAzureConfigMap(c)
	provider := azureEnvironProvider{}
	oldConfig, err := config.New(attrs)
	c.Assert(err, IsNil)
	newConfig, err := config.New(attrs)
	c.Assert(err, IsNil)
	result, err := provider.Validate(newConfig, oldConfig)
	c.Assert(err, IsNil)
	c.Check(result.Name(), Equals, attrs["name"])
}

func (ConfigSuite) TestValidateChecksConfigChanges(c *C) {
	provider := azureEnvironProvider{}
	oldAttrs := makeBaseConfigMap()
	oldConfig, err := config.New(oldAttrs)
	c.Assert(err, IsNil)
	newAttrs := makeBaseConfigMap()
	newAttrs["name"] = "different-name"
	newConfig, err := config.New(newAttrs)
	c.Assert(err, IsNil)
	_, err = provider.Validate(newConfig, oldConfig)
	c.Check(err, NotNil)
}

func (ConfigSuite) TestValidateParsesAzureConfig(c *C) {
	managementSubscriptionId := "subscription-id"
	managementCertificatePath := createTempFile(c)
	managementHostedServiceName := "service-name"
	storageAccountName := "account-name"
	storageAccountKey := "account-key"
	storageContainerName := "container-name"
	azureConfig := map[string]interface{}{
		"management-subscription-id":     managementSubscriptionId,
		"management-certificate-path":    managementCertificatePath,
		"management-hosted-service-name": managementHostedServiceName,
		"storage-account-name":           storageAccountName,
		"storage-account-key":            storageAccountKey,
		"storage-container-name":         storageContainerName,
	}
	attrs := makeConfigMap(azureConfig)
	provider := azureEnvironProvider{}
	config, err := config.New(attrs)
	c.Assert(err, IsNil)
	azConfig, err := provider.newConfig(config)
	c.Assert(err, IsNil)
	c.Check(azConfig.Name(), Equals, attrs["name"])
	c.Check(azConfig.ManagementSubscriptionId(), Equals, managementSubscriptionId)
	c.Check(azConfig.ManagementCertificatePath(), Equals, managementCertificatePath)
	c.Check(azConfig.ManagementHostedServiceName(), Equals, managementHostedServiceName)
	c.Check(azConfig.StorageAccountName(), Equals, storageAccountName)
	c.Check(azConfig.StorageAccountKey(), Equals, storageAccountKey)
	c.Check(azConfig.StorageContainerName(), Equals, storageContainerName)
}

func (ConfigSuite) TestChecksExistingCertFile(c *C) {
	nonExistingCertPath := "non-existing-cert-file"
	attrs := makeAzureConfigMap(c)
	attrs["management-certificate-path"] = nonExistingCertPath
	provider := azureEnvironProvider{}
	newConfig, err := config.New(attrs)
	c.Assert(err, IsNil)
	_, err = provider.Validate(newConfig, nil)
	c.Check(err, ErrorMatches, ".*"+nonExistingCertPath+": no such file or directory.*")
}

func (ConfigSuite) TestBoilerplateConfigReturnsAzureConfig(c *C) {
	provider := azureEnvironProvider{}
	boilerPlateConfig := provider.BoilerplateConfig()
	c.Assert(strings.Contains(boilerPlateConfig, "type: azure"), Equals, true)
}
