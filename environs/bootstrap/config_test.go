// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"os"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
)

type ConfigSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

func TestConfigSuite(t *stdtesting.T) { tc.Run(t, &ConfigSuite{}) }
func (*ConfigSuite) TestDefaultConfig(c *tc.C) {
	cfg, err := bootstrap.NewConfig(nil)
	c.Assert(err, tc.ErrorIsNil)

	// These four things are generated.
	c.Assert(cfg.AdminSecret, tc.Not(tc.HasLen), 0)
	c.Assert(cfg.CACert, tc.Not(tc.HasLen), 0)
	c.Assert(cfg.CAPrivateKey, tc.Not(tc.HasLen), 0)
	c.Assert(cfg.SSHServerHostKey, tc.HasLen, 0)

	c.Assert(cfg.BootstrapTimeout, tc.Equals, time.Second*1200)
	c.Assert(cfg.BootstrapRetryDelay, tc.Equals, time.Second*5)
	c.Assert(cfg.BootstrapAddressesDelay, tc.Equals, time.Second*10)
}

func (*ConfigSuite) TestConfigValuesSpecified(c *tc.C) {
	for _, serviceType := range []string{"external", "loadbalancer"} {
		externalIps := []string{"10.0.0.1", "10.0.0.2"}
		if serviceType == "loadbalancer" {
			externalIps = externalIps[:1]
		}
		cfg, err := bootstrap.NewConfig(map[string]interface{}{
			"admin-secret":              "sekrit",
			"ca-cert":                   testing.CACert,
			"ca-private-key":            testing.CAKey,
			"ssh-server-host-key":       testing.SSHServerHostKey,
			"bootstrap-timeout":         1,
			"bootstrap-retry-delay":     2,
			"bootstrap-addresses-delay": 3,
			"controller-service-type":   serviceType,
			"controller-external-name":  "externalName",
			"controller-external-ips":   externalIps,
		})
		c.Assert(err, tc.ErrorIsNil)

		c.Assert(cfg, tc.DeepEquals, bootstrap.Config{
			AdminSecret:             "sekrit",
			CACert:                  testing.CACert,
			CAPrivateKey:            testing.CAKey,
			SSHServerHostKey:        testing.SSHServerHostKey,
			BootstrapTimeout:        time.Second * 1,
			BootstrapRetryDelay:     time.Second * 2,
			BootstrapAddressesDelay: time.Second * 3,
			ControllerServiceType:   serviceType,
			ControllerExternalName:  "externalName",
			ControllerExternalIPs:   externalIps,
		})
	}
}

func (s *ConfigSuite) addFiles(c *tc.C, files ...testhelpers.TestFile) {
	for _, f := range files {
		err := os.WriteFile(osenv.JujuXDGDataHomePath(f.Name), []byte(f.Data), 0666)
		c.Assert(err, tc.IsNil)
	}
}

func (s *ConfigSuite) TestDefaultConfigReadsDefaultCACertKeyFiles(c *tc.C) {
	s.addFiles(c, []testhelpers.TestFile{
		{"ca-cert.pem", testing.CACert},
		{"ca-private-key.pem", testing.CAKey},
	}...)

	cfg, err := bootstrap.NewConfig(nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cfg.CACert, tc.Equals, testing.CACert)
	c.Assert(cfg.CAPrivateKey, tc.Equals, testing.CAKey)
}

func (s *ConfigSuite) TestConfigReadsCACertKeyFilesFromPaths(c *tc.C) {
	s.addFiles(c, []testhelpers.TestFile{
		{"ca-cert-2.pem", testing.OtherCACert},
		{"ca-private-key-2.pem", testing.OtherCAKey},
	}...)

	cfg, err := bootstrap.NewConfig(map[string]interface{}{
		"ca-cert-path":        "ca-cert-2.pem",
		"ca-private-key-path": "ca-private-key-2.pem",
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cfg.CACert, tc.Equals, testing.OtherCACert)
	c.Assert(cfg.CAPrivateKey, tc.Equals, testing.OtherCAKey)
}

func (s *ConfigSuite) TestConfigNonExistentPath(c *tc.C) {
	s.testConfigError(c, map[string]interface{}{
		"ca-cert-path": "not/there",
	}, `reading "ca-cert" from file: "ca-cert" not set, and could not read from "not/there": .*`)
}

func (s *ConfigSuite) TestConfigInvalidCACert(c *tc.C) {
	s.testConfigError(c, map[string]interface{}{
		"ca-cert":        invalidCACert,
		"ca-private-key": testing.CAKey,
	}, "validating ca-cert and ca-private-key: x509: malformed certificate")
}

func (s *ConfigSuite) TestConfigInvalidCAKey(c *tc.C) {
	s.testConfigError(c, map[string]interface{}{
		"ca-cert":        testing.CACert,
		"ca-private-key": invalidCAKey,
	}, "validating ca-cert and ca-private-key: (crypto/)?tls: failed to parse private key")
}

func (s *ConfigSuite) TestConfigCACertKeyMismatch(c *tc.C) {
	s.testConfigError(c, map[string]interface{}{
		"ca-cert":        testing.CACert,
		"ca-private-key": testing.OtherCAKey,
	}, "validating ca-cert and ca-private-key: (crypto/)?tls: private key does not match public key")
}

func (s *ConfigSuite) TestConfigCACertWithEmptyKey(c *tc.C) {
	s.testConfigError(c, map[string]interface{}{
		"ca-cert": testing.CACert,
	}, "validating ca-cert and ca-private-key: (crypto/)?tls: failed to find any PEM data in key input")
}

func (s *ConfigSuite) TestConfigEmptyCACertWithKey(c *tc.C) {
	s.testConfigError(c, map[string]interface{}{
		"ca-private-key": testing.CAKey,
	}, "validating ca-cert and ca-private-key: (crypto/)?tls: failed to find any PEM data in certificate input")
}

func (*ConfigSuite) testConfigError(c *tc.C, attrs map[string]interface{}, expect string) {
	_, err := bootstrap.NewConfig(attrs)
	c.Assert(err, tc.ErrorMatches, expect)
}

func (*ConfigSuite) TestValidate(c *tc.C) {
	c.Assert(validConfig().Validate(), tc.ErrorIsNil)
}

func (*ConfigSuite) TestValidateAdminSecret(c *tc.C) {
	cfg := validConfig()
	cfg.AdminSecret = ""
	c.Assert(cfg.Validate(), tc.ErrorMatches, "empty admin-secret not valid")
}

func (*ConfigSuite) TestValidateBootstrapTimeout(c *tc.C) {
	cfg := validConfig()
	cfg.BootstrapTimeout = 0
	c.Assert(cfg.Validate(), tc.ErrorMatches, "bootstrap-timeout of 0s? not valid")
}

func (*ConfigSuite) TestValidateBootstrapRetryDelay(c *tc.C) {
	cfg := validConfig()
	cfg.BootstrapRetryDelay = -1 * time.Second
	c.Assert(cfg.Validate(), tc.ErrorMatches, "bootstrap-retry-delay of -1s not valid")
}

func (*ConfigSuite) TestValidateBootstrapAddressesDelay(c *tc.C) {
	cfg := validConfig()
	cfg.BootstrapAddressesDelay = -2 * time.Minute
	c.Assert(cfg.Validate(), tc.ErrorMatches, "bootstrap-addresses-delay of -2m0s not valid")
}

func (*ConfigSuite) TestValidateExternalIpsAndServiceType(c *tc.C) {
	cfg := validConfig()
	cfg.ControllerServiceType = "cluster"
	c.Assert(cfg.Validate(), tc.ErrorMatches, `external IPs require a service type of "external" or "loadbalancer"`)
}

func (*ConfigSuite) TestValidateExternalIpsAndLoadBalancer(c *tc.C) {
	cfg := validConfig()
	cfg.ControllerServiceType = "loadbalancer"
	c.Assert(cfg.Validate(), tc.ErrorMatches, `only 1 external IP is allowed with service type "loadbalancer"`)
}

func (*ConfigSuite) TestValidateSSHServerHostKey(c *tc.C) {
	cfg := validConfig()
	cfg.SSHServerHostKey = "bad key"
	c.Assert(cfg.Validate(), tc.ErrorMatches, "validating ssh-server-host-key: ssh: no key found")
}

func validConfig() bootstrap.Config {
	return bootstrap.Config{
		AdminSecret:             "sekrit",
		CACert:                  testing.CACert,
		CAPrivateKey:            testing.CAKey,
		SSHServerHostKey:        testing.SSHServerHostKey,
		BootstrapTimeout:        time.Second * 1,
		BootstrapRetryDelay:     time.Second * 2,
		BootstrapAddressesDelay: time.Second * 3,
		ControllerServiceType:   "external",
		ControllerExternalIPs:   []string{"10.0.0.1", "10.0.0.2"},
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
