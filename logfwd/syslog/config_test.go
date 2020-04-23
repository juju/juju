// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/logfwd/syslog"
	coretesting "github.com/juju/juju/testing"
)

type ConfigSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) TestRawValidateFull(c *gc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *ConfigSuite) TestRawValidateWithoutPort(c *gc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *ConfigSuite) TestRawValidateZeroValue(c *gc.C) {
	var cfg syslog.RawConfig
	err := cfg.Validate()
	c.Check(err, jc.ErrorIsNil)
}

func (s *ConfigSuite) TestRawValidateMissingHost(c *gc.C) {
	cfg := syslog.RawConfig{
		Enabled:    true,
		Host:       "",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, gc.ErrorMatches, `Host "" not valid`)
}

func (s *ConfigSuite) TestRawValidateMissingHostNotEnabled(c *gc.C) {
	cfg := syslog.RawConfig{
		Host:       "",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()
	c.Check(err, jc.ErrorIsNil)
}

func (s *ConfigSuite) TestRawValidateMissingHostname(c *gc.C) {
	cfg := syslog.RawConfig{
		Enabled:    true,
		Host:       ":9876",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, gc.ErrorMatches, `Host ":9876" not valid`)
}

func (s *ConfigSuite) TestRawValidateMissingCACert(c *gc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     "",
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, gc.ErrorMatches, `validating TLS config: parsing CA certificate: no certificates found`)
}

func (s *ConfigSuite) TestRawValidateBadCACert(c *gc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     invalidCert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, gc.ErrorMatches, `validating TLS config: parsing CA certificate: asn1: syntax error: data truncated`)
}

func (s *ConfigSuite) TestRawValidateBadCACertFormat(c *gc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     "abc",
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, gc.ErrorMatches, `validating TLS config: parsing CA certificate: no certificates found`)
}

func (s *ConfigSuite) TestRawValidateMissingCert(c *gc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     coretesting.CACert,
		ClientCert: "",
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, gc.ErrorMatches, `validating TLS config: parsing client key pair: (crypto/)?tls: failed to find any PEM data in certificate input`)
}

func (s *ConfigSuite) TestRawValidateBadCert(c *gc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     coretesting.CACert,
		ClientCert: invalidCert,
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, gc.ErrorMatches, `validating TLS config: parsing client key pair: asn1: syntax error: data truncated`)
}

func (s *ConfigSuite) TestRawValidateBadCertFormat(c *gc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     coretesting.CACert,
		ClientCert: "abc",
		ClientKey:  coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, gc.ErrorMatches, `validating TLS config: parsing client key pair: (crypto/)?tls: failed to find any PEM data in certificate input`)
}

func (s *ConfigSuite) TestRawValidateMissingKey(c *gc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  "",
	}

	err := cfg.Validate()

	c.Check(err, gc.ErrorMatches, `validating TLS config: parsing client key pair: (crypto/)?tls: failed to find any PEM data in key input`)
}

func (s *ConfigSuite) TestRawValidateBadKey(c *gc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  invalidKey,
	}

	err := cfg.Validate()

	c.Check(err, gc.ErrorMatches, `validating TLS config: parsing client key pair: (crypto/)?tls: failed to parse private key`)
}

func (s *ConfigSuite) TestRawValidateBadKeyFormat(c *gc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  "abc",
	}

	err := cfg.Validate()

	c.Check(err, gc.ErrorMatches, `validating TLS config: parsing client key pair: (crypto/)?tls: failed to find any PEM data in key input`)
}

func (s *ConfigSuite) TestRawValidateCertKeyMismatch(c *gc.C) {
	cfg := syslog.RawConfig{
		Host:       "a.b.c:9876",
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.CAKey,
	}

	err := cfg.Validate()
	c.Check(err, gc.ErrorMatches, `validating TLS config: parsing client key pair: (crypto/)?tls: private key does not match public key`)
}

var invalidCert = `
-----BEGIN CERTIFICATE-----
MIIBOgIBAAJAZabKgKInuOxj5vDWLwHHQtK3/45KB+32D15w94Nt83BmuGxo90lw
-----END CERTIFICATE-----
`[1:]

var invalidKey = `
-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJAZabKgKInuOxj5vDWLwHHQtK3/45KB+32D15w94Nt83BmuGxo90lw
-----END RSA PRIVATE KEY-----
`[1:]
