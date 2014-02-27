// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"io/ioutil"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
)

type configSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&configSuite{})

// makeConfigMap creates a minimal map of standard configuration items,
// adds the given extra items to that and returns it.
func makeConfigMap(extra map[string]interface{}) map[string]interface{} {
	return testing.FakeConfig().Merge(testing.Attrs{
		"name": "testenv",
		"type": "azure",
	}).Merge(extra)
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

func makeAzureConfigMap(c *gc.C) map[string]interface{} {
	azureConfig := map[string]interface{}{
		"location":                   "location",
		"management-subscription-id": "subscription-id",
		"management-certificate":     testCert,
		"storage-account-name":       "account-name",
	}
	return makeConfigMap(azureConfig)
}

// createTempFile creates a temporary file.  The file will be cleaned
// up at the end of the test calling this method.
func createTempFile(c *gc.C, content []byte) string {
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, gc.IsNil)
	filename := file.Name()
	err = ioutil.WriteFile(filename, content, 0644)
	c.Assert(err, gc.IsNil)
	return filename
}

func (*configSuite) TestValidateAcceptsNilOldConfig(c *gc.C) {
	attrs := makeAzureConfigMap(c)
	provider := azureEnvironProvider{}
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	result, err := provider.Validate(config, nil)
	c.Assert(err, gc.IsNil)
	c.Check(result.Name(), gc.Equals, attrs["name"])
}

func (*configSuite) TestValidateAcceptsUnchangedConfig(c *gc.C) {
	attrs := makeAzureConfigMap(c)
	provider := azureEnvironProvider{}
	oldConfig, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	newConfig, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	result, err := provider.Validate(newConfig, oldConfig)
	c.Assert(err, gc.IsNil)
	c.Check(result.Name(), gc.Equals, attrs["name"])
}

func (*configSuite) TestValidateChecksConfigChanges(c *gc.C) {
	provider := azureEnvironProvider{}
	oldConfig, err := config.New(config.NoDefaults, makeConfigMap(nil))
	c.Assert(err, gc.IsNil)
	newAttrs := makeConfigMap(map[string]interface{}{
		"name": "different-name",
	})
	newConfig, err := config.New(config.NoDefaults, newAttrs)
	c.Assert(err, gc.IsNil)
	_, err = provider.Validate(newConfig, oldConfig)
	c.Check(err, gc.NotNil)
}

func (*configSuite) TestValidateParsesAzureConfig(c *gc.C) {
	location := "location"
	managementSubscriptionId := "subscription-id"
	certificate := "certificate content"
	storageAccountName := "account-name"
	forceImageName := "force-image-name"
	unknownFutureSetting := "preserved"
	azureConfig := map[string]interface{}{
		"location":                   location,
		"management-subscription-id": managementSubscriptionId,
		"management-certificate":     certificate,
		"storage-account-name":       storageAccountName,
		"force-image-name":           forceImageName,
		"unknown-future-setting":     unknownFutureSetting,
	}
	attrs := makeConfigMap(azureConfig)
	provider := azureEnvironProvider{}
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	azConfig, err := provider.newConfig(config)
	c.Assert(err, gc.IsNil)
	c.Check(azConfig.Name(), gc.Equals, attrs["name"])
	c.Check(azConfig.location(), gc.Equals, location)
	c.Check(azConfig.managementSubscriptionId(), gc.Equals, managementSubscriptionId)
	c.Check(azConfig.managementCertificate(), gc.Equals, certificate)
	c.Check(azConfig.storageAccountName(), gc.Equals, storageAccountName)
	c.Check(azConfig.forceImageName(), gc.Equals, forceImageName)
	c.Check(azConfig.UnknownAttrs()["unknown-future-setting"], gc.Equals, unknownFutureSetting)
}

func (*configSuite) TestValidateReadsCertFile(c *gc.C) {
	certificate := "test certificate"
	certFile := createTempFile(c, []byte(certificate))
	attrs := makeAzureConfigMap(c)
	delete(attrs, "management-certificate")
	attrs["management-certificate-path"] = certFile
	provider := azureEnvironProvider{}
	newConfig, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	azConfig, err := provider.newConfig(newConfig)
	c.Assert(err, gc.IsNil)
	c.Check(azConfig.managementCertificate(), gc.Equals, certificate)
}

func (*configSuite) TestChecksExistingCertFile(c *gc.C) {
	nonExistingCertPath := "non-existing-cert-file"
	attrs := makeAzureConfigMap(c)
	delete(attrs, "management-certificate")
	attrs["management-certificate-path"] = nonExistingCertPath
	provider := azureEnvironProvider{}
	newConfig, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	_, err = provider.Validate(newConfig, nil)
	c.Check(err, gc.ErrorMatches, ".*"+nonExistingCertPath+": no such file or directory.*")
}

func (*configSuite) TestChecksLocationIsRequired(c *gc.C) {
	attrs := makeAzureConfigMap(c)
	attrs["location"] = ""
	provider := azureEnvironProvider{}
	newConfig, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	_, err = provider.Validate(newConfig, nil)
	c.Check(err, gc.ErrorMatches, ".*environment has no location.*")
}

func (*configSuite) TestBoilerplateConfigReturnsAzureConfig(c *gc.C) {
	provider := azureEnvironProvider{}
	boilerPlateConfig := provider.BoilerplateConfig()
	c.Assert(strings.Contains(boilerPlateConfig, "type: azure"), gc.Equals, true)
}

func (*configSuite) TestSecretAttrsReturnsSensitiveAttributes(c *gc.C) {
	attrs := makeAzureConfigMap(c)
	certificate := "certificate"
	attrs["management-certificate"] = certificate
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)

	provider := azureEnvironProvider{}
	secretAttrs, err := provider.SecretAttrs(config)
	c.Assert(err, gc.IsNil)

	expectedAttrs := map[string]string{
		"management-certificate": certificate,
	}
	c.Check(secretAttrs, gc.DeepEquals, expectedAttrs)
}

func (*configSuite) TestEmptyImageStream1dot16Compat(c *gc.C) {
	attrs := makeAzureConfigMap(c)
	attrs["image-stream"] = ""
	provider := azureEnvironProvider{}
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, gc.IsNil)
	_, err = provider.Validate(cfg, nil)
	c.Assert(err, gc.IsNil)
}
