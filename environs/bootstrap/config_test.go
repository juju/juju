// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"io/ioutil"
	"time"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type ConfigSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&ConfigSuite{})

func (*ConfigSuite) TestDefaultConfig(c *gc.C) {
	cfg, err := bootstrap.NewConfig(nil)
	c.Assert(err, jc.ErrorIsNil)

	// These three things are generated.
	c.Assert(cfg.AdminSecret, gc.Not(gc.HasLen), 0)
	c.Assert(cfg.CACert, gc.Not(gc.HasLen), 0)
	c.Assert(cfg.CAPrivateKey, gc.Not(gc.HasLen), 0)

	c.Assert(cfg.BootstrapTimeout, gc.Equals, time.Second*1200)
	c.Assert(cfg.BootstrapRetryDelay, gc.Equals, time.Second*5)
	c.Assert(cfg.BootstrapAddressesDelay, gc.Equals, time.Second*10)
}

func (*ConfigSuite) TestConfigValuesSpecified(c *gc.C) {
	cfg, err := bootstrap.NewConfig(map[string]interface{}{
		"admin-secret":              "sekrit",
		"ca-cert":                   testing.CACert,
		"ca-private-key":            testing.CAKey,
		"bootstrap-timeout":         1,
		"bootstrap-retry-delay":     2,
		"bootstrap-addresses-delay": 3,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cfg, jc.DeepEquals, bootstrap.Config{
		AdminSecret:             "sekrit",
		CACert:                  testing.CACert,
		CAPrivateKey:            testing.CAKey,
		BootstrapTimeout:        time.Second * 1,
		BootstrapRetryDelay:     time.Second * 2,
		BootstrapAddressesDelay: time.Second * 3,
	})
}

func (s *ConfigSuite) addFiles(c *gc.C, files ...gitjujutesting.TestFile) {
	for _, f := range files {
		err := ioutil.WriteFile(osenv.JujuXDGDataHomePath(f.Name), []byte(f.Data), 0666)
		c.Assert(err, gc.IsNil)
	}
}

func (s *ConfigSuite) TestDefaultConfigReadsDefaultCACertKeyFiles(c *gc.C) {
	s.addFiles(c, []gitjujutesting.TestFile{
		{"ca-cert.pem", testing.CACert},
		{"ca-private-key.pem", testing.CAKey},
	}...)

	cfg, err := bootstrap.NewConfig(nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cfg.CACert, gc.Equals, testing.CACert)
	c.Assert(cfg.CAPrivateKey, gc.Equals, testing.CAKey)
}

func (s *ConfigSuite) TestConfigReadsCACertKeyFilesFromPaths(c *gc.C) {
	s.addFiles(c, []gitjujutesting.TestFile{
		{"ca-cert-2.pem", testing.OtherCACert},
		{"ca-private-key-2.pem", testing.OtherCAKey},
	}...)

	cfg, err := bootstrap.NewConfig(map[string]interface{}{
		"ca-cert-path":        "ca-cert-2.pem",
		"ca-private-key-path": "ca-private-key-2.pem",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cfg.CACert, gc.Equals, testing.OtherCACert)
	c.Assert(cfg.CAPrivateKey, gc.Equals, testing.OtherCAKey)
}

func (s *ConfigSuite) TestConfigNonExistentPath(c *gc.C) {
	s.testConfigError(c, map[string]interface{}{
		"ca-cert-path": "not/there",
	}, `reading "ca-cert" from file: "ca-cert" not set, and could not read from "not/there": .*`)
}

func (s *ConfigSuite) TestConfigInvalidCACert(c *gc.C) {
	s.testConfigError(c, map[string]interface{}{
		"ca-cert":        invalidCACert,
		"ca-private-key": testing.CAKey,
	}, "validating ca-cert and ca-private-key: asn1: syntax error: data truncated")
}

func (s *ConfigSuite) TestConfigInvalidCAKey(c *gc.C) {
	s.testConfigError(c, map[string]interface{}{
		"ca-cert":        testing.CACert,
		"ca-private-key": invalidCAKey,
	}, "validating ca-cert and ca-private-key: (crypto/)?tls: failed to parse private key")
}

func (s *ConfigSuite) TestConfigCACertKeyMismatch(c *gc.C) {
	s.testConfigError(c, map[string]interface{}{
		"ca-cert":        testing.CACert,
		"ca-private-key": testing.OtherCAKey,
	}, "validating ca-cert and ca-private-key: (crypto/)?tls: private key does not match public key")
}

func (s *ConfigSuite) TestConfigCACertWithEmptyKey(c *gc.C) {
	s.testConfigError(c, map[string]interface{}{
		"ca-cert": testing.CACert,
	}, "validating ca-cert and ca-private-key: (crypto/)?tls: failed to find any PEM data in key input")
}

func (s *ConfigSuite) TestConfigEmptyCACertWithKey(c *gc.C) {
	s.testConfigError(c, map[string]interface{}{
		"ca-private-key": testing.CAKey,
	}, "validating ca-cert and ca-private-key: (crypto/)?tls: failed to find any PEM data in certificate input")
}

func (*ConfigSuite) testConfigError(c *gc.C, attrs map[string]interface{}, expect string) {
	_, err := bootstrap.NewConfig(attrs)
	c.Assert(err, gc.ErrorMatches, expect)
}

func (*ConfigSuite) TestValidate(c *gc.C) {
	c.Assert(validConfig().Validate(), jc.ErrorIsNil)
}

func (*ConfigSuite) TestValidateAdminSecret(c *gc.C) {
	cfg := validConfig()
	cfg.AdminSecret = ""
	c.Assert(cfg.Validate(), gc.ErrorMatches, "empty admin-secret not valid")
}

func (*ConfigSuite) TestValidateBootstrapTimeout(c *gc.C) {
	cfg := validConfig()
	cfg.BootstrapTimeout = 0
	c.Assert(cfg.Validate(), gc.ErrorMatches, "bootstrap-timeout of 0s? not valid")
}

func (*ConfigSuite) TestValidateBootstrapRetryDelay(c *gc.C) {
	cfg := validConfig()
	cfg.BootstrapRetryDelay = -1 * time.Second
	c.Assert(cfg.Validate(), gc.ErrorMatches, "bootstrap-retry-delay of -1s not valid")
}

func (*ConfigSuite) TestValidateBootstrapAddressesDelay(c *gc.C) {
	cfg := validConfig()
	cfg.BootstrapAddressesDelay = -2 * time.Minute
	c.Assert(cfg.Validate(), gc.ErrorMatches, "bootstrap-addresses-delay of -2m0s not valid")
}

func validConfig() bootstrap.Config {
	return bootstrap.Config{
		AdminSecret:             "sekrit",
		CACert:                  testing.CACert,
		CAPrivateKey:            testing.CAKey,
		BootstrapTimeout:        time.Second * 1,
		BootstrapRetryDelay:     time.Second * 2,
		BootstrapAddressesDelay: time.Second * 3,
	}
}

var invalidCAKey = `
-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJAZabKgKInuOxj5vDWLwHHQtK3/45KB+32D15w94Nt83BmuGxo90lw
-----END RSA PRIVATE KEY-----
`[1:]

var invalidCACert = `
-----BEGIN CERTIFICATE-----
MIIBOgIBAAJAZabKgKInuOxj5vDWLwHHQtK3/45KB+32D15w94Nt83BmuGxo90lw
-----END CERTIFICATE-----
`[1:]
