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

type configSuite struct{}

var _ = Suite(&configSuite{})

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

var testCert = `
-----BEGIN PRIVATE KEY-----
MIIBCgIBADANBgkqhkiG9w0BAQEFAASB9TCB8gIBAAIxAKQGQxP1i0VfCWn4KmMP
taUFn8sMBKjP/9vHnUYdZRvvmoJCA1C6arBUDp8s2DNX+QIDAQABAjBLRqhwN4dU
LfqHDKJ/Vg1aD8u3Buv4gYRBxdFR5PveyqHSt5eJ4g/x/4ndsvr2OqUCGQDNfNlD
zxHCiEAwZZAPaAkn8jDkFupTljcCGQDMWCujiVZ1NNuBD/N32Yt8P9JDiNzZa08C
GBW7VXLxbExpgnhb1V97vjQmTfthXQjYAwIYSTEjoFXm4+Bk5xuBh2IidgSeGZaC
FFY9AhkAsteo31cyQw2xJ80SWrmsIw+ps7Cvt5W9
-----END PRIVATE KEY-----
-----BEGIN CERTIFICATE-----
MIIBDzCByqADAgECAgkAgIBb3+lSwzEwDQYJKoZIhvcNAQEFBQAwFTETMBEGA1UE
AxQKQEhvc3ROYW1lQDAeFw0xMzA3MTkxNjA1NTRaFw0yMzA3MTcxNjA1NTRaMBUx
EzARBgNVBAMUCkBIb3N0TmFtZUAwTDANBgkqhkiG9w0BAQEFAAM7ADA4AjEApAZD
E/WLRV8JafgqYw+1pQWfywwEqM//28edRh1lG++agkIDULpqsFQOnyzYM1f5AgMB
AAGjDTALMAkGA1UdEwQCMAAwDQYJKoZIhvcNAQEFBQADMQABKfn08tKfzzqMMD2w
PI2fs3bw5bRH8tmGjrsJeEdp9crCBS8I3hKcxCkTTRTowdY=
-----END CERTIFICATE-----
`

func makeAzureConfigMap(c *C) map[string]interface{} {
	azureConfig := map[string]interface{}{
		"location":                      "location",
		"management-subscription-id":    "subscription-id",
		"management-certificate":        testCert,
		"storage-account-name":          "account-name",
		"storage-account-key":           "YWNjb3VudC1rZXkK",
		"public-storage-account-name":   "public-account-name",
		"public-storage-container-name": "public-container-name",
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

func (*configSuite) TestValidateAcceptsNilOldConfig(c *C) {
	attrs := makeAzureConfigMap(c)
	provider := azureEnvironProvider{}
	config, err := config.New(attrs)
	c.Assert(err, IsNil)
	result, err := provider.Validate(config, nil)
	c.Assert(err, IsNil)
	c.Check(result.Name(), Equals, attrs["name"])
}

func (*configSuite) TestValidateAcceptsUnchangedConfig(c *C) {
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

func (*configSuite) TestValidateChecksConfigChanges(c *C) {
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

func (*configSuite) TestValidateParsesAzureConfig(c *C) {
	location := "location"
	managementSubscriptionId := "subscription-id"
	certificate := "certificate content"
	storageAccountName := "account-name"
	storageAccountKey := "account-key"
	publicStorageAccountName := "public-account-name"
	publicStorageContainerName := "public-container-name"
	forceImageName := "force-image-name"
	unknownFutureSetting := "preserved"
	azureConfig := map[string]interface{}{
		"location":                      location,
		"management-subscription-id":    managementSubscriptionId,
		"management-certificate":        certificate,
		"storage-account-name":          storageAccountName,
		"storage-account-key":           storageAccountKey,
		"public-storage-account-name":   publicStorageAccountName,
		"public-storage-container-name": publicStorageContainerName,
		"force-image-name":              forceImageName,
		"unknown-future-setting":        unknownFutureSetting,
	}
	attrs := makeConfigMap(azureConfig)
	provider := azureEnvironProvider{}
	config, err := config.New(attrs)
	c.Assert(err, IsNil)
	azConfig, err := provider.newConfig(config)
	c.Assert(err, IsNil)
	c.Check(azConfig.Name(), Equals, attrs["name"])
	c.Check(azConfig.location(), Equals, location)
	c.Check(azConfig.managementSubscriptionId(), Equals, managementSubscriptionId)
	c.Check(azConfig.managementCertificate(), Equals, certificate)
	c.Check(azConfig.storageAccountName(), Equals, storageAccountName)
	c.Check(azConfig.storageAccountKey(), Equals, storageAccountKey)
	c.Check(azConfig.publicStorageAccountName(), Equals, publicStorageAccountName)
	c.Check(azConfig.publicStorageContainerName(), Equals, publicStorageContainerName)
	c.Check(azConfig.forceImageName(), Equals, forceImageName)
	c.Check(azConfig.UnknownAttrs()["unknown-future-setting"], Equals, unknownFutureSetting)
}

func (*configSuite) TestValidateReadsCertFile(c *C) {
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
	c.Check(azConfig.managementCertificate(), Equals, certificate)
}

func (*configSuite) TestChecksExistingCertFile(c *C) {
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

func (*configSuite) TestChecksPublicStorageAccountNameCannotBeDefinedAlone(c *C) {
	attrs := makeAzureConfigMap(c)
	attrs["public-storage-container-name"] = ""
	provider := azureEnvironProvider{}
	newConfig, err := config.New(attrs)
	c.Assert(err, IsNil)
	_, err = provider.Validate(newConfig, nil)
	c.Check(err, ErrorMatches, ".*both or none of them.*")
}

func (*configSuite) TestChecksPublicStorageContainerNameCannotBeDefinedAlone(c *C) {
	attrs := makeAzureConfigMap(c)
	attrs["public-storage-account-name"] = ""
	provider := azureEnvironProvider{}
	newConfig, err := config.New(attrs)
	c.Assert(err, IsNil)
	_, err = provider.Validate(newConfig, nil)
	c.Check(err, ErrorMatches, ".*both or none of them.*")
}

func (*configSuite) TestChecksLocationIsRequired(c *C) {
	attrs := makeAzureConfigMap(c)
	attrs["location"] = ""
	provider := azureEnvironProvider{}
	newConfig, err := config.New(attrs)
	c.Assert(err, IsNil)
	_, err = provider.Validate(newConfig, nil)
	c.Check(err, ErrorMatches, ".*environment has no location.*")
}

func (*configSuite) TestBoilerplateConfigReturnsAzureConfig(c *C) {
	provider := azureEnvironProvider{}
	boilerPlateConfig := provider.BoilerplateConfig()
	c.Assert(strings.Contains(boilerPlateConfig, "type: azure"), Equals, true)
}

func (*configSuite) TestSecretAttrsReturnsSensitiveAttributes(c *C) {
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
