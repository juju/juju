// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tls_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/standards/tls"
	coretesting "github.com/juju/juju/testing"
)

type RawCertSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(RawCertSuite{})

func (RawCertSuite) Test(c *gc.C) {
}

func (s *RawCertSuite) TestRawValidateFull(c *gc.C) {
	cfg := tls.RawCert{
		CACertPEM: coretesting.CACert,
		CertPEM:   coretesting.ServerCert,
		KeyPEM:    coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *RawCertSuite) TestRawValidateMissingCACert(c *gc.C) {
	cfg := tls.RawCert{
		CACertPEM: "",
		CertPEM:   validCert,
		KeyPEM:    validKey,
	}

	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `empty CACertPEM`)
}

func (s *RawCertSuite) TestRawValidateBadCACert(c *gc.C) {
	cfg := tls.RawCert{
		CACertPEM: invalidCACert,
		CertPEM:   validCert,
		KeyPEM:    validKey,
	}

	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `invalid CACertPEM: asn1: syntax error: data truncated`)
}

func (s *RawCertSuite) TestRawValidateBadCACertFormat(c *gc.C) {
	cfg := tls.RawCert{
		CACertPEM: "abc",
		CertPEM:   validCert,
		KeyPEM:    validKey,
	}

	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `invalid CACertPEM: no certificates found`)
}

func (s *RawCertSuite) TestRawValidateMissingCert(c *gc.C) {
	cfg := tls.RawCert{
		CACertPEM: validCACert,
		CertPEM:   "",
		KeyPEM:    validKey,
	}

	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `empty CertPEM`)
}

func (s *RawCertSuite) TestRawValidateBadCert(c *gc.C) {
	cfg := tls.RawCert{
		CACertPEM: validCACert,
		CertPEM:   invalidCert,
		KeyPEM:    validKey,
	}

	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `invalid CertPEM: asn1: syntax error: data truncated`)
}

func (s *RawCertSuite) TestRawValidateBadCertFormat(c *gc.C) {
	cfg := tls.RawCert{
		CACertPEM: validCACert,
		CertPEM:   "abc",
		KeyPEM:    validKey,
	}

	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `invalid CertPEM: no certificates found`)
}

func (s *RawCertSuite) TestRawValidateMissingKey(c *gc.C) {
	cfg := tls.RawCert{
		CACertPEM: validCACert,
		CertPEM:   validCert,
		KeyPEM:    "",
	}

	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `empty KeyPEM`)
}

func (s *RawCertSuite) TestRawValidateBadKey(c *gc.C) {
	cfg := tls.RawCert{
		CACertPEM: validCACert,
		CertPEM:   validCert,
		KeyPEM:    invalidKey,
	}

	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `invalid KeyPEM: bad key or key does not match certificate: crypto/tls: failed to parse private key`)
}

func (s *RawCertSuite) TestRawValidateBadKeyFormat(c *gc.C) {
	cfg := tls.RawCert{
		CACertPEM: validCACert,
		CertPEM:   validCert,
		KeyPEM:    "abc",
	}

	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `invalid KeyPEM: bad key or key does not match certificate: crypto/tls: failed to find any PEM data in key input`)
}

func (s *RawCertSuite) TestRawValidateCertKeyMismatch(c *gc.C) {
	cfg := tls.RawCert{
		CACertPEM: validCACert,
		CertPEM:   validCert,
		KeyPEM:    validKey2,
	}

	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `invalid KeyPEM: bad key or key does not match certificate: crypto/tls: private key does not match public key`)
}

func (s *RawCertSuite) TestRawValidateCACertMismatch(c *gc.C) {
	cfg := tls.RawCert{
		CACertPEM: coretesting.OtherCACert,
		CertPEM:   coretesting.ServerCert,
		KeyPEM:    coretesting.ServerKey,
	}

	err := cfg.Validate()

	c.Check(err, gc.ErrorMatches, `invalid CertPEM: cert does not match CA cert: x509: certificate signed by unknown authority \(possibly because of "crypto/rsa: verification error" while trying to verify candidate authority certificate "juju-generated CA for model \\"juju testing\\""\)`)
}

var validCACert = `
-----BEGIN CERTIFICATE-----
MIIBjTCCATmgAwIBAgIBADALBgkqhkiG9w0BAQUwHjENMAsGA1UEChMEanVqdTEN
MAsGA1UEAxMEcm9vdDAeFw0xMjExMDkxNjQxMjlaFw0yMjExMDkxNjQ2MjlaMB4x
DTALBgNVBAoTBGp1anUxDTALBgNVBAMTBHJvb3QwWjALBgkqhkiG9w0BAQEDSwAw
SAJBAIW7CbHFJivvV9V6mO8AGzJS9lqjUf6MdEPsdF6wx2Cpzr/lSFIggCwRA138
9MuFxflxb/3U8Nq+rd8rVtTgFMECAwEAAaNmMGQwDgYDVR0PAQH/BAQDAgCkMBIG
A1UdEwEB/wQIMAYBAf8CAQEwHQYDVR0OBBYEFJafrxqByMN9BwGfcmuF0Lw/1QII
MB8GA1UdIwQYMBaAFJafrxqByMN9BwGfcmuF0Lw/1QIIMAsGCSqGSIb3DQEBBQNB
AHq3vqNhxya3s33DlQfSj9whsnqM0Nm+u8mBX/T76TF5rV7+B33XmYzSyfA3yBi/
zHaUR/dbHuiNTO+KXs3/+Y4=
-----END CERTIFICATE-----
`[1:]

var validCert = `
-----BEGIN CERTIFICATE-----
MIIBjDCCATigAwIBAgIBADALBgkqhkiG9w0BAQUwHjENMAsGA1UEChMEanVqdTEN
MAsGA1UEAxMEcm9vdDAeFw0xMjExMDkxNjQwMjhaFw0yMjExMDkxNjQ1MjhaMB4x
DTALBgNVBAoTBGp1anUxDTALBgNVBAMTBHJvb3QwWTALBgkqhkiG9w0BAQEDSgAw
RwJAduA1Gnb2VJLxNGfG4St0Qy48Y3q5Z5HheGtTGmti/FjlvQvScCFGCnJG7fKA
Knd7ia3vWg7lxYkIvMPVP88LAQIDAQABo2YwZDAOBgNVHQ8BAf8EBAMCAKQwEgYD
VR0TAQH/BAgwBgEB/wIBATAdBgNVHQ4EFgQUlvKX8vwp0o+VdhdhoA9O6KlOm00w
HwYDVR0jBBgwFoAUlvKX8vwp0o+VdhdhoA9O6KlOm00wCwYJKoZIhvcNAQEFA0EA
LlNpevtFr8gngjAFFAO/FXc7KiZcCrA5rBfb/rEy297lIqmKt5++aVbLEPyxCIFC
r71Sj63TUTFWtRZAxvn9qQ==
-----END CERTIFICATE-----
`[1:]

var validKey = `
-----BEGIN RSA PRIVATE KEY-----
MIIBOQIBAAJAduA1Gnb2VJLxNGfG4St0Qy48Y3q5Z5HheGtTGmti/FjlvQvScCFG
CnJG7fKAKnd7ia3vWg7lxYkIvMPVP88LAQIDAQABAkEAsFOdMSYn+AcF1M/iBfjo
uQWJ+Zz+CgwuvumjGNsUtmwxjA+hh0fCn0Ah2nAt4Ma81vKOKOdQ8W6bapvsVDH0
6QIhAJOkLmEKm4H5POQV7qunRbRsLbft/n/SHlOBz165WFvPAiEAzh9fMf70std1
sVCHJRQWKK+vw3oaEvPKvkPiV5ui0C8CIGNsvybuo8ald5IKCw5huRlFeIxSo36k
m3OVCXc6zfwVAiBnTUe7WcivPNZqOC6TAZ8dYvdWo4Ifz3jjpEfymjid1wIgBIJv
ERPyv2NQqIFQZIyzUP7LVRIWfpFFOo9/Ww/7s5Y=
-----END RSA PRIVATE KEY-----
`[1:]

var validCert2 = `
-----BEGIN CERTIFICATE-----
MIIBjTCCATmgAwIBAgIBADALBgkqhkiG9w0BAQUwHjENMAsGA1UEChMEanVqdTEN
MAsGA1UEAxMEcm9vdDAeFw0xMjExMDkxNjQxMDhaFw0yMjExMDkxNjQ2MDhaMB4x
DTALBgNVBAoTBGp1anUxDTALBgNVBAMTBHJvb3QwWjALBgkqhkiG9w0BAQEDSwAw
SAJBAJkSWRrr81y8pY4dbNgt+8miSKg4z6glp2KO2NnxxAhyyNtQHKvC+fJALJj+
C2NhuvOv9xImxOl3Hg8fFPCXCtcCAwEAAaNmMGQwDgYDVR0PAQH/BAQDAgCkMBIG
A1UdEwEB/wQIMAYBAf8CAQEwHQYDVR0OBBYEFOsX/ZCqKzWCAaTTVcWsWKT5Msow
MB8GA1UdIwQYMBaAFOsX/ZCqKzWCAaTTVcWsWKT5MsowMAsGCSqGSIb3DQEBBQNB
AAVV57jetEzJQnjgBzhvx/UwauFn78jGhXfV5BrQmxIb4SF4DgSCFstPwUQOAr8h
XXzJqBQH92KYmp+y3YXDoMQ=
-----END CERTIFICATE-----
`[1:]

var validKey2 = `
-----BEGIN RSA PRIVATE KEY-----
MIIBOQIBAAJBAJkSWRrr81y8pY4dbNgt+8miSKg4z6glp2KO2NnxxAhyyNtQHKvC
+fJALJj+C2NhuvOv9xImxOl3Hg8fFPCXCtcCAwEAAQJATQNzO11NQvJS5U6eraFt
FgSFQ8XZjILtVWQDbJv8AjdbEgKMHEy33icsAKIUAx8jL9kjq6K9kTdAKXZi9grF
UQIhAPD7jccIDUVm785E5eR9eisq0+xpgUIa24Jkn8cAlst5AiEAopxVFl1auer3
GP2In3pjdL4ydzU/gcRcYisoJqwHpM8CIHtqmaXBPeq5WT9ukb5/dL3+5SJCtmxA
jQMuvZWRe6khAiBvMztYtPSDKXRbCZ4xeQ+kWSDHtok8Y5zNoTeu4nvDrwIgb3Al
fikzPveC5g6S6OvEQmyDz59tYBubm2XHgvxqww0=
-----END RSA PRIVATE KEY-----
`[1:]

var invalidCACert = `
-----BEGIN CERTIFICATE-----
MIIBOgIBAAJAZabKgKInuOxj5vDWLwHHQtK3/45KB+32D15w94Nt83BmuGxo90lw
-----END CERTIFICATE-----
`[1:]

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
