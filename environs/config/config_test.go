package config_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"
)

func Test(t *stdtesting.T) {
	TestingT(t)
}

type ConfigSuite struct {
	testing.LoggingSuite
	home string
}

var _ = Suite(&ConfigSuite{})

type attrs map[string]interface{}

var configTests = []struct {
	about string
	attrs map[string]interface{}
	err   string
}{
	{
		"The minimum good configuration",
		attrs{
			"type": "my-type",
			"name": "my-name",
		},
		"",
	}, {
		"Explicit series",
		attrs{
			"type":           "my-type",
			"name":           "my-name",
			"default-series": "my-series",
		},
		"",
	}, {
		"Implicit series with empty value",
		attrs{
			"type":           "my-type",
			"name":           "my-name",
			"default-series": "",
		},
		"",
	}, {
		"Explicit authorized-keys",
		attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": "my-keys",
		},
		"",
	}, {
		"Load authorized-keys from path",
		attrs{
			"type":                 "my-type",
			"name":                 "my-name",
			"authorized-keys-path": "~/.ssh/authorized_keys2",
		},
		"",
	}, {
		"Root cert & key from path",
		attrs{
			"type":           "my-type",
			"name":           "my-name",
			"root-cert-path": "rootcert2.pem",
		},
		"",
	}, {
		"Root cert & key from ~ path",
		attrs{
			"type":           "my-type",
			"name":           "my-name",
			"root-cert-path": "~/othercert.pem",
		},
		"",
	}, {
		"Root cert only from ~ path",
		attrs{
			"type":           "my-type",
			"name":           "my-name",
			"root-cert-path": "~/certonly.pem",
		},
		"",
	}, {
		"Root cert only as attribute",
		attrs{
			"type":      "my-type",
			"name":      "my-name",
			"root-cert": rootCert,
		},
		"",
	}, {
		"Root cert and key as attributes",
		attrs{
			"type":             "my-type",
			"name":             "my-name",
			"root-cert":        rootCert,
			"root-private-key": rootKey,
		},
		"",
	}, {
		"Mismatched root cert and key",
		attrs{
			"type":             "my-type",
			"name":             "my-name",
			"root-cert":        rootCert,
			"root-private-key": rootKey2,
		},
		"bad root certificate/key in configuration: crypto/tls: private key does not match public key",
	}, {
		"Invalid root cert",
		attrs{
			"type":      "my-type",
			"name":      "my-name",
			"root-cert": invalidRootCert,
		},
		"bad root certificate/key in configuration: ASN.1 syntax error:.*",
	}, {
		"Invalid root key",
		attrs{
			"type":             "my-type",
			"name":             "my-name",
			"root-cert":        rootCert,
			"root-private-key": invalidRootKey,
		},
		"bad root certificate/key in configuration: crypto/tls: failed to parse key:.*",
	}, {
		"Specified agent version",
		attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": "my-keys",
			"agent-version":   "1.2.3",
		},
		"",
	}, {
		"Specified development flag",
		attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": "my-keys",
			"development":     true,
		},
		"",
	}, {
		"Specified admin secret",
		attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": "my-keys",
			"development":     false,
			"admin-secret":    "pork",
		},
		"",
	}, {
		"Invalid development flag",
		attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": "my-keys",
			"development":     "true",
		},
		"development: expected bool, got \"true\"",
	}, {
		"Invalid agent version",
		attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": "my-keys",
			"agent-version":   "2",
		},
		`invalid agent version in environment configuration: "2"`,
	}, {
		"Missing type",
		attrs{
			"name": "my-name",
		},
		"type: expected string, got nothing",
	}, {
		"Empty type",
		attrs{
			"name": "my-name",
			"type": "",
		},
		"empty type in environment configuration",
	}, {
		"Missing name",
		attrs{
			"type": "my-type",
		},
		"name: expected string, got nothing",
	}, {
		"Empty name",
		attrs{
			"type": "my-type",
			"name": "",
		},
		"empty name in environment configuration",
	}, {
		"Default firewall mode",
		attrs{
			"type":          "my-type",
			"name":          "my-name",
			"firewall-mode": config.FwDefault,
		},
		"",
	}, {
		"Instance firewall mode",
		attrs{
			"type":          "my-type",
			"name":          "my-name",
			"firewall-mode": config.FwInstance,
		},
		"",
	}, {
		"Global firewall mode",
		attrs{
			"type":          "my-type",
			"name":          "my-name",
			"firewall-mode": config.FwGlobal,
		},
		"",
	}, {
		"Illegal firewall mode",
		attrs{
			"type":          "my-type",
			"name":          "my-name",
			"firewall-mode": "illegal",
		},
		"invalid firewall mode in environment configuration: .*",
	},
}

var configTestFiles = []struct {
	name, data string
}{
	{".ssh/id_dsa.pub", "dsa"},
	{".ssh/id_rsa.pub", "rsa\n"},
	{".ssh/identity.pub", "identity"},
	{".ssh/authorized_keys", "auth0\n# first\nauth1\n\n"},
	{".ssh/authorized_keys2", "auth2\nauth3\n"},

	{".juju/rootcert.pem", rootCert + rootKey},
	{".juju/rootcert2.pem", rootKey2 + rootCert2},
	{"othercert.pem", rootCert3 + rootKey3},
	{"certonly.pem", rootCert3},
}

func (*ConfigSuite) TestConfig(c *C) {
	homeDir := c.MkDir()
	defer os.Setenv("HOME", os.Getenv("HOME"))
	os.Setenv("HOME", homeDir)

	err := os.Mkdir(filepath.Join(homeDir, ".ssh"), 0777)
	c.Assert(err, IsNil)
	err = os.Mkdir(filepath.Join(homeDir, ".juju"), 0700)
	c.Assert(err, IsNil)

	for _, f := range configTestFiles {
		err = ioutil.WriteFile(filepath.Join(homeDir, f.name), []byte(f.data), 0666)
		c.Assert(err, IsNil)
	}

	for i, test := range configTests {
		c.Logf("test %d. %s", i, test.about)
		cfg, err := config.New(test.attrs)
		if test.err != "" {
			c.Assert(err, ErrorMatches, test.err)
			continue
		} else if err != nil {
			c.Fatalf("error with config %#v: %v", test.attrs, err)
		}

		typ, _ := test.attrs["type"].(string)
		name, _ := test.attrs["name"].(string)
		c.Assert(cfg.Type(), Equals, typ)
		c.Assert(cfg.Name(), Equals, name)
		if s := test.attrs["agent-version"]; s != nil {
			vers, err := version.Parse(s.(string))
			c.Assert(err, IsNil)
			c.Assert(cfg.AgentVersion(), Equals, vers)
		} else {
			c.Assert(cfg.AgentVersion(), Equals, version.Number{})
		}

		dev, _ := test.attrs["development"].(bool)
		c.Assert(cfg.Development(), Equals, dev)

		if series, _ := test.attrs["default-series"].(string); series != "" {
			c.Assert(cfg.DefaultSeries(), Equals, series)
		} else {
			c.Assert(cfg.DefaultSeries(), Equals, version.Current.Series)
		}

		if m, _ := test.attrs["firewall-mode"].(string); m != "" {
			c.Assert(cfg.FirewallMode(), Equals, config.FirewallMode(m))
		}

		if secret, _ := test.attrs["admin-secret"].(string); secret != "" {
			c.Assert(cfg.AdminSecret(), Equals, secret)
		}

		if path, _ := test.attrs["authorized-keys-path"].(string); path != "" {
			c.Assert(cfg.AuthorizedKeys(), Equals, fileContents(c, path))
			c.Assert(cfg.AllAttrs()["authorized-keys-path"], Equals, nil)
		} else if keys, _ := test.attrs["authorized-keys"].(string); keys != "" {
			c.Assert(cfg.AuthorizedKeys(), Equals, keys)
		} else {
			// Content of all the files that are read by default.
			want := "dsa\nrsa\nidentity\n"
			c.Assert(cfg.AuthorizedKeys(), Equals, want)
		}

		if path, _ := test.attrs["root-cert-path"].(string); path != "" {
			cert := cfg.RootCertPEM()
			key := cfg.RootPrivateKeyPEM()
			f := fileContents(c, path)
			// the certificate and the key can be in any order in the file.
			certi := strings.Index(f, "CERTIFICATE--")
			keyi := strings.Index(f, "KEY--")
			switch {
			case certi == -1:
				panic("file does not have certificate")
			case keyi == -1:
				c.Assert(cert, Equals, f)
				c.Assert(key, Equals, "")
			case certi < keyi:
				c.Assert(cert+key, Equals, f)
			default:
				c.Assert(key+cert, Equals, f)
			}
		} else if test.attrs["root-cert"] != nil {
			c.Assert(cfg.RootCertPEM(), Equals, test.attrs["root-cert"])
			key, _ := test.attrs["root-private-key"].(string)
			c.Assert(cfg.RootPrivateKeyPEM(), Equals, key)
		} else {
			c.Assert(cfg.RootCertPEM(), Equals, rootCert)
			c.Assert(cfg.RootPrivateKeyPEM(), Equals, rootKey)
		}
	}
}

func fileContents(c *C, path string) string {
	for _, f := range configTestFiles {
		if filepath.Base(f.name) == filepath.Base(path) {
			return f.data
		}
	}
	c.Fatalf("path attribute holds unknown test file: %q", path)
	panic("unreachable")
}

func (*ConfigSuite) TestConfigAttrs(c *C) {
	attrs := map[string]interface{}{
		"type":             "my-type",
		"name":             "my-name",
		"authorized-keys":  "my-keys",
		"firewall-mode":    string(config.FwDefault),
		"default-series":   version.Current.Series,
		"admin-secret":     "foo",
		"unknown":          "my-unknown",
		"root-private-key": "",
		"root-cert":        rootCert,
	}
	cfg, err := config.New(attrs)
	c.Assert(err, IsNil)

	attrs["development"] = false // This attribute is added if not set.
	c.Assert(cfg.AllAttrs(), DeepEquals, attrs)
	c.Assert(cfg.UnknownAttrs(), DeepEquals, map[string]interface{}{"unknown": "my-unknown"})

	newcfg, err := cfg.Apply(map[string]interface{}{
		"name":        "new-name",
		"new-unknown": "my-new-unknown",
	})

	attrs["name"] = "new-name"
	attrs["new-unknown"] = "my-new-unknown"
	c.Assert(newcfg.AllAttrs(), DeepEquals, attrs)
}

var rootCert = `
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

var rootKey = `
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

var rootCert2 = `
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

var rootKey2 = `
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

var rootCert3 = `
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

var rootKey3 = `
-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJBAIW7CbHFJivvV9V6mO8AGzJS9lqjUf6MdEPsdF6wx2Cpzr/lSFIg
gCwRA1389MuFxflxb/3U8Nq+rd8rVtTgFMECAwEAAQJAaivPi4qJPrJb2onl50H/
VZnWKqmljGF4YQDWduMEt7GTPk+76x9SpO7W4gfY490Ivd9DEXfbr/KZqhwWikNw
LQIhALlLfRXLF2ZfToMfB1v1v+jith5onAu24O68mkdRc5PLAiEAuMJ/6U07hggr
Ckf9OT93wh84DK66h780HJ/FUHKcoCMCIDsPZaJBpoa50BOZG0ZjcTTwti3BGCPf
uZg+w0oCGz27AiEAsUCYKqEXy/ymHhT2kSecozYENdajyXvcaOG3EPkD3nUCICOP
zatzs7c/4mx4a0JBG6Za0oEPUcm2I34is50KSohz
-----END RSA PRIVATE KEY-----
`[1:]

var invalidRootKey = `
-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJAZabKgKInuOxj5vDWLwHHQtK3/45KB+32D15w94Nt83BmuGxo90lw
-----END RSA PRIVATE KEY-----
`[1:]

var invalidRootCert = `
-----BEGIN CERTIFICATE-----
MIIBOgIBAAJAZabKgKInuOxj5vDWLwHHQtK3/45KB+32D15w94Nt83BmuGxo90lw
-----END CERTIFICATE-----
`[1:]
