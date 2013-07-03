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
	azureConfig := map[string]interface{}{
		"management-subscription-id":     "subscription-id",
		"management-certificate":         "cert",
		"management-hosted-service-name": "service-name",
		"storage-account-name":           "account-name",
		"storage-account-key":            "account-key",
		"storage-container-name":         "container-name",
		"public-storage-account-name":    "public-account-name",
		"public-storage-container-name":  "public-container-name",
	}
	return makeConfigMap(azureConfig)
}

// createTempFile creates a temporary file.  The file will be cleaned
// up at the end of the test calling this method.
func createTempFile(c *C, content []byte) string {
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, IsNil)
	filename := file.Name()
	err = ioutil.WriteFile(filename, content, 0644)
	c.Assert(err, IsNil)
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

func (ConfigSuite) TestValidateRejectsChangingHostedServiceName(c *C) {
	attrs := makeAzureConfigMap(c)
	newConfig, err := config.New(attrs)
	c.Assert(err, IsNil)
	provider := azureEnvironProvider{}
	attrs["management-hosted-service-name"] = "another name"
	oldConfig, err := config.New(attrs)
	c.Assert(err, IsNil)
	_, err = provider.Validate(newConfig, oldConfig)
	c.Check(err, ErrorMatches, ".*cannot change management-hosted-service-name.*")
}

func (ConfigSuite) TestValidateRejectsChangingStorageContainer(c *C) {
	attrs := makeAzureConfigMap(c)
	newConfig, err := config.New(attrs)
	c.Assert(err, IsNil)
	provider := azureEnvironProvider{}
	attrs["storage-container-name"] = "another name"
	oldConfig, err := config.New(attrs)
	c.Assert(err, IsNil)
	_, err = provider.Validate(newConfig, oldConfig)
	c.Check(err, ErrorMatches, ".*cannot change storage-container-name.*")
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
	certificate := "certificate content"
	managementHostedServiceName := "service-name"
	storageAccountName := "account-name"
	storageAccountKey := "account-key"
	storageContainerName := "container-name"
	publicStorageAccountName := "public-account-name"
	publicStorageContainerName := "public-container-name"
	unknownFutureSetting := "preserved"
	azureConfig := map[string]interface{}{
		"management-subscription-id":     managementSubscriptionId,
		"management-certificate":         certificate,
		"management-hosted-service-name": managementHostedServiceName,
		"storage-account-name":           storageAccountName,
		"storage-account-key":            storageAccountKey,
		"storage-container-name":         storageContainerName,
		"public-storage-account-name":    publicStorageAccountName,
		"public-storage-container-name":  publicStorageContainerName,
		"unknown-future-setting":         unknownFutureSetting,
	}
	attrs := makeConfigMap(azureConfig)
	provider := azureEnvironProvider{}
	config, err := config.New(attrs)
	c.Assert(err, IsNil)
	azConfig, err := provider.newConfig(config)
	c.Assert(err, IsNil)
	c.Check(azConfig.Name(), Equals, attrs["name"])
	c.Check(azConfig.ManagementSubscriptionId(), Equals, managementSubscriptionId)
	c.Check(azConfig.ManagementCertificate(), Equals, certificate)
	c.Check(azConfig.ManagementHostedServiceName(), Equals, managementHostedServiceName)
	c.Check(azConfig.StorageAccountName(), Equals, storageAccountName)
	c.Check(azConfig.StorageAccountKey(), Equals, storageAccountKey)
	c.Check(azConfig.StorageContainerName(), Equals, storageContainerName)
	c.Check(azConfig.PublicStorageAccountName(), Equals, publicStorageAccountName)
	c.Check(azConfig.PublicStorageContainerName(), Equals, publicStorageContainerName)
	c.Check(azConfig.UnknownAttrs()["unknown-future-setting"], Equals, unknownFutureSetting)
}

func (ConfigSuite) TestValidateReadsCertFile(c *C) {
	certificate := "test certificate"
	certFile := createTempFile(c, []byte(certificate))
	attrs := makeAzureConfigMap(c)
	delete(attrs, "management-certificate")
	attrs["management-certificate-path"] = certFile
	provider := azureEnvironProvider{}
	newConfig, err := config.New(attrs)
	c.Assert(err, IsNil)
	azConfig, err := provider.newConfig(newConfig)
	c.Assert(err, IsNil)
	c.Check(azConfig.ManagementCertificate(), Equals, certificate)
}

func (ConfigSuite) TestChecksExistingCertFile(c *C) {
	nonExistingCertPath := "non-existing-cert-file"
	attrs := makeAzureConfigMap(c)
	delete(attrs, "management-certificate")
	attrs["management-certificate-path"] = nonExistingCertPath
	provider := azureEnvironProvider{}
	newConfig, err := config.New(attrs)
	c.Assert(err, IsNil)
	_, err = provider.Validate(newConfig, nil)
	c.Check(err, ErrorMatches, ".*"+nonExistingCertPath+": no such file or directory.*")
}

func (ConfigSuite) TestChecksPublicStorageAccountNameCannotBeDefinedAlone(c *C) {
	attrs := makeAzureConfigMap(c)
	attrs["public-storage-container-name"] = ""
	provider := azureEnvironProvider{}
	newConfig, err := config.New(attrs)
	c.Assert(err, IsNil)
	_, err = provider.Validate(newConfig, nil)
	c.Check(err, ErrorMatches, ".*both or none of them.*")
}

func (ConfigSuite) TestChecksPublicStorageContainerNameCannotBeDefinedAlone(c *C) {
	attrs := makeAzureConfigMap(c)
	attrs["public-storage-account-name"] = ""
	provider := azureEnvironProvider{}
	newConfig, err := config.New(attrs)
	c.Assert(err, IsNil)
	_, err = provider.Validate(newConfig, nil)
	c.Check(err, ErrorMatches, ".*both or none of them.*")
}

func (ConfigSuite) TestBoilerplateConfigReturnsAzureConfig(c *C) {
	provider := azureEnvironProvider{}
	boilerPlateConfig := provider.BoilerplateConfig()
	c.Assert(strings.Contains(boilerPlateConfig, "type: azure"), Equals, true)
}

func (ConfigSuite) TestSecretAttrsReturnsSensitiveAttributes(c *C) {
	attrs := makeAzureConfigMap(c)
	certificate := "certificate"
	attrs["management-certificate"] = certificate
	storageAccountKey := "key"
	attrs["storage-account-key"] = storageAccountKey
	config, err := config.New(attrs)
	c.Assert(err, IsNil)

	provider := azureEnvironProvider{}
	secretAttrs, err := provider.SecretAttrs(config)
	c.Assert(err, IsNil)

	expectedAttrs := map[string]interface{}{
		"management-certificate": certificate,
		"storage-account-key":    storageAccountKey,
	}
	c.Check(secretAttrs, DeepEquals, expectedAttrs)
}
