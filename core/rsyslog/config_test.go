package rsyslog_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/rsyslog"
)

var _ = gc.Suite(&configSuite{})

const (
	cert = `
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
`

	key = `
-----BEGIN RSA PRIVATE KEY-----
MIIBOQIBAAJAduA1Gnb2VJLxNGfG4St0Qy48Y3q5Z5HheGtTGmti/FjlvQvScCFG
CnJG7fKAKnd7ia3vWg7lxYkIvMPVP88LAQIDAQABAkEAsFOdMSYn+AcF1M/iBfjo
uQWJ+Zz+CgwuvumjGNsUtmwxjA+hh0fCn0Ah2nAt4Ma81vKOKOdQ8W6bapvsVDH0
6QIhAJOkLmEKm4H5POQV7qunRbRsLbft/n/SHlOBz165WFvPAiEAzh9fMf70std1
sVCHJRQWKK+vw3oaEvPKvkPiV5ui0C8CIGNsvybuo8ald5IKCw5huRlFeIxSo36k
m3OVCXc6zfwVAiBnTUe7WcivPNZqOC6TAZ8dYvdWo4Ifz3jjpEfymjid1wIgBIJv
ERPyv2NQqIFQZIyzUP7LVRIWfpFFOo9/Ww/7s5Y=
-----END RSA PRIVATE KEY-----
`

	invalidKey = `
-----BEGIN RSA PRIVATE KEY-----
MIIBOQIBAAJBAJkSWRrr81y8pY4dbNgt+8miSKg4z6glp2KO2NnxxAhyyNtQHKvC
+fJALJj+C2NhuvOv9xImxOl3Hg8fFPCXCtcCAwEAAQJATQNzO11NQvJS5U6eraFt
FgSFQ8XZjILtVWQDbJv8AjdbEgKMHEy33icsAKIUAx8jL9kjq6K9kTdAKXZi9grF
UQIhAPD7jccIDUVm785E5eR9eisq0+xpgUIa24Jkn8cAlst5AiEAopxVFl1auer3
GP2In3pjdL4ydzU/gcRcYisoJqwHpM8CIHtqmaXBPeq5WT9ukb5/dL3+5SJCtmxA
jQMuvZWRe6khAiBvMztYtPSDKXRbCZ4xeQ+kWSDHtok8Y5zNoTeu4nvDrwIgb3Al
fikzPveC5g6S6OvEQmyDz59tYBubm2XHgvxqww0=
-----END RSA PRIVATE KEY-----
`

	invalidCert = `
-----BEGIN CERTIFICATE-----
MIIBOgIBAAJAZabKgKInuOxj5vDWLwHHQtK3/45KB+32D15w94Nt83BmuGxo90lw
-----END CERTIFICATE-----
`
)

type configSuite struct{}

func (s *configSuite) TestValidation(c *gc.C) {
	tests := []struct {
		about  string
		url    string
		caCert string
		cert   string
		key    string
		err    string
	}{{
		about: "fail to parse url",
		url:   "%",
		err:   "URL not valid: parse %: invalid URL escape \"%\"",
	}, {
		about: "invalid url schema",
		url:   "ftp://test.com",
		err:   "URL not valid; https required",
	}, {
		about:  "invalid ca cert",
		caCert: invalidCert,
		err:    "CACert not valid: asn1: syntax error: data truncated",
	}, {
		about: "invalid cert",
		cert:  invalidCert,
		err:   "Cert not valid: asn1: syntax error: data truncated",
	}, {
		about: "invalid key/cert pair",
		key:   invalidKey,
		err:   "Cert/Key pair not valid: crypto/tls: private key does not match public key",
	}, {
		about: "ok",
	},
	}
	for i, test := range tests {
		c.Logf("running test %d: %v", i, test.about)
		config := rsyslog.ClientConfig{
			URL:    "https://test.com",
			CACert: cert,
			Cert:   cert,
			Key:    key,
		}
		if test.url != "" {
			config.URL = test.url
		}
		if test.caCert != "" {
			config.CACert = test.caCert
		}
		if test.cert != "" {
			config.Cert = test.cert
		}
		if test.key != "" {
			config.Key = test.key
		}
		err := config.Validate()
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
	}
}

func (s *configSuite) TestIsNotConfigured(c *gc.C) {
	tests := []struct {
		cfg rsyslog.ClientConfig
		err string
	}{{
		cfg: rsyslog.ClientConfig{
			CACert: cert,
			Cert:   cert,
			Key:    key,
		},
		err: "URL not valid",
	}, {
		cfg: rsyslog.ClientConfig{
			URL:  "https://test.com",
			Cert: cert,
			Key:  key,
		},
		err: "CACert not valid: no certificates found",
	}, {
		cfg: rsyslog.ClientConfig{
			URL:    "https://test.com",
			CACert: cert,
			Key:    key,
		},
		err: "Cert not valid: no certificates found",
	}, {
		cfg: rsyslog.ClientConfig{
			URL:    "https://test.com",
			CACert: cert,
			Cert:   cert,
		},
		err: "Cert/Key pair not valid: crypto/tls: failed to find any PEM data in key input",
	}, {
		cfg: rsyslog.ClientConfig{},
		err: rsyslog.ErrNotConfigured.Error(),
	},
	}
	for i, test := range tests {
		c.Logf("running test %d", i)
		err := test.cfg.Validate()
		c.Check(err, gc.ErrorMatches, test.err)
	}
}
