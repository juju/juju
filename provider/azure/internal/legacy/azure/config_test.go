// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/testing"
)

type configSuite struct {
	testing.BaseSuite
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
	c.Assert(err, jc.ErrorIsNil)
	filename := file.Name()
	err = ioutil.WriteFile(filename, content, 0644)
	c.Assert(err, jc.ErrorIsNil)
	return filename
}

func (*configSuite) TestValidateAcceptsNilOldConfig(c *gc.C) {
	attrs := makeAzureConfigMap(c)
	provider := azureEnvironProvider{}
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	result, err := provider.Validate(config, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Name(), gc.Equals, attrs["name"])
}

func (*configSuite) TestValidateAcceptsUnchangedConfig(c *gc.C) {
	attrs := makeAzureConfigMap(c)
	provider := azureEnvironProvider{}
	oldConfig, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	newConfig, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	result, err := provider.Validate(newConfig, oldConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Name(), gc.Equals, attrs["name"])
}

func (*configSuite) TestValidateChecksConfigChanges(c *gc.C) {
	provider := azureEnvironProvider{}
	oldConfig, err := config.New(config.NoDefaults, makeConfigMap(nil))
	c.Assert(err, jc.ErrorIsNil)
	newAttrs := makeConfigMap(map[string]interface{}{
		"name": "different-name",
	})
	newConfig, err := config.New(config.NoDefaults, newAttrs)
	c.Assert(err, jc.ErrorIsNil)
	_, err = provider.Validate(newConfig, oldConfig)
	c.Check(err, gc.NotNil)
}

func (*configSuite) TestValidateParsesAzureConfig(c *gc.C) {
	location := "location"
	managementSubscriptionId := "subscription-id"
	certificate := testCert
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
	c.Assert(err, jc.ErrorIsNil)
	azConfig, err := provider.newConfig(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(azConfig.Name(), gc.Equals, attrs["name"])
	c.Check(azConfig.location(), gc.Equals, location)
	c.Check(azConfig.managementSubscriptionId(), gc.Equals, managementSubscriptionId)
	c.Check(azConfig.managementCertificate(), gc.Equals, certificate)
	c.Check(azConfig.storageAccountName(), gc.Equals, storageAccountName)
	c.Check(azConfig.forceImageName(), gc.Equals, forceImageName)
	c.Check(azConfig.UnknownAttrs()["unknown-future-setting"], gc.Equals, unknownFutureSetting)
}

func (*configSuite) TestValidateVerifiesCertFileContents(c *gc.C) {
	certFile := createTempFile(c, []byte("definitely not PEM"))
	attrs := makeAzureConfigMap(c)
	delete(attrs, "management-certificate")
	attrs["management-certificate-path"] = certFile
	provider := azureEnvironProvider{}
	newConfig, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	_, err = provider.newConfig(newConfig)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("invalid management-certificate-path: %q is not a PEM encoded certificate file", regexp.QuoteMeta(certFile)))
}

func (*configSuite) TestValidateVerifiesCertContents(c *gc.C) {
	attrs := makeAzureConfigMap(c)
	attrs["management-certificate"] = "definitely not PEM"
	provider := azureEnvironProvider{}
	newConfig, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	_, err = provider.newConfig(newConfig)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("invalid management-certificate: not a PEM encoded certificate"))
}

func (*configSuite) TestValidateReadsCertFile(c *gc.C) {
	certFile := createTempFile(c, []byte(testCert))
	attrs := makeAzureConfigMap(c)
	delete(attrs, "management-certificate")
	attrs["management-certificate-path"] = certFile
	provider := azureEnvironProvider{}
	newConfig, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	azConfig, err := provider.newConfig(newConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(azConfig.managementCertificate(), gc.Equals, testCert)
}

func (*configSuite) TestChecksExistingCertFile(c *gc.C) {
	nonExistingCertPath := "non-existing-cert-file"
	attrs := makeAzureConfigMap(c)
	delete(attrs, "management-certificate")
	attrs["management-certificate-path"] = nonExistingCertPath
	provider := azureEnvironProvider{}
	newConfig, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	_, err = provider.Validate(newConfig, nil)
	c.Check(err, gc.ErrorMatches, ".*"+nonExistingCertPath+": "+utils.NoSuchFileErrRegexp)
}

func (*configSuite) TestChecksLocationIsRequired(c *gc.C) {
	attrs := makeAzureConfigMap(c)
	attrs["location"] = ""
	provider := azureEnvironProvider{}
	newConfig, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	_, err = provider.Validate(newConfig, nil)
	c.Check(err, gc.ErrorMatches, ".*environment has no location.*")
}

func (*configSuite) TestBoilerplateConfigReturnsAzureConfig(c *gc.C) {
	provider := azureEnvironProvider{}
	boilerPlateConfig := provider.BoilerplateConfig()
	c.Assert(strings.Contains(boilerPlateConfig, "type: azure"), jc.IsTrue)
}

func (*configSuite) TestSecretAttrsReturnsSensitiveAttributes(c *gc.C) {
	attrs := makeAzureConfigMap(c)
	attrs["management-certificate"] = testCert
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	provider := azureEnvironProvider{}
	secretAttrs, err := provider.SecretAttrs(config)
	c.Assert(err, jc.ErrorIsNil)

	expectedAttrs := map[string]string{
		"management-certificate": testCert,
	}
	c.Check(secretAttrs, gc.DeepEquals, expectedAttrs)
}

func (*configSuite) TestEmptyImageStream1dot16Compat(c *gc.C) {
	attrs := makeAzureConfigMap(c)
	attrs["image-stream"] = ""
	provider := azureEnvironProvider{}
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	_, err = provider.Validate(cfg, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *configSuite) TestAvailabilitySetsEnabledDefault(c *gc.C) {
	s.PatchValue(&verifyCredentials, func(*azureEnviron) error {
		return nil
	})
	userValues := []interface{}{nil, false, true}
	for _, userValue := range userValues {
		attrs := makeAzureConfigMap(c)
		// If availability-sets-enabled isn't specified, it's set to true.
		checker := jc.IsTrue
		if userValue, ok := userValue.(bool); ok {
			attrs["availability-sets-enabled"] = userValue
			if !userValue {
				checker = jc.IsFalse
			}
		}
		cfg, err := config.New(config.UseDefaults, attrs)
		c.Assert(err, jc.ErrorIsNil)
		env, err := azureEnvironProvider{}.PrepareForBootstrap(envtesting.BootstrapContext(c), cfg)
		c.Assert(err, jc.ErrorIsNil)
		azureEnv := env.(*azureEnviron)
		c.Assert(azureEnv.ecfg.availabilitySetsEnabled(), checker)
	}
}

func (s *configSuite) TestAvailabilitySetsEnabledImmutable(c *gc.C) {
	s.PatchValue(&verifyCredentials, func(*azureEnviron) error {
		return nil
	})
	cfg, err := config.New(config.UseDefaults, makeAzureConfigMap(c))
	c.Assert(err, jc.ErrorIsNil)
	env, err := azureEnvironProvider{}.PrepareForBootstrap(envtesting.BootstrapContext(c), cfg)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err = env.Config().Apply(map[string]interface{}{"availability-sets-enabled": false})
	c.Assert(err, jc.ErrorIsNil)
	err = env.SetConfig(cfg)
	c.Assert(err, gc.ErrorMatches, "cannot change availability-sets-enabled")
}
