package config_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/version"
	"os"
	"path/filepath"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type ConfigSuite struct {
	home string
}

var _ = Suite(&ConfigSuite{})

type attrs map[string]interface{}

func (s *ConfigSuite) SetUpSuite(c *C) {
}

var configTests = []struct {
	attrs map[string]interface{}
	err   string
}{
	{
		// The minimum good configuration.
		attrs{
			"type": "my-type",
			"name": "my-name",
		},
		"",
	}, {
		// Explicit series.
		attrs{
			"type":           "my-type",
			"name":           "my-name",
			"default-series": "my-series",
		},
		"",
	}, {
		// Implicit series with empty value.
		attrs{
			"type":           "my-type",
			"name":           "my-name",
			"default-series": "",
		},
		"",
	}, {
		// Explicit authorized-keys.
		attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": "my-keys",
		},
		"",
	}, {
		// Load authorized-keys from path.
		attrs{
			"type":                 "my-type",
			"name":                 "my-name",
			"authorized-keys-path": "~/.ssh/authorized_keys2",
		},
		"",
	}, {
		attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": "my-keys",
			"agent-version":   "1.2.3",
		},
		"",
	}, {
		attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": "my-keys",
			"development":     true,
		},
		"",
	}, {
		attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": "my-keys",
			"development":     false,
		},
		"",
	}, {
		attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": "my-keys",
			"development":     "true",
		},
		"development: expected bool, got \"true\"",
	}, {
		attrs{
			"type":            "my-type",
			"name":            "my-name",
			"authorized-keys": "my-keys",
			"agent-version":   "2",
		},
		`invalid agent version in environment configuration: "2"`,
	}, {
		attrs{
			"name": "my-name",
		},
		"type: expected string, got nothing",
	}, {
		attrs{
			"name": "my-name",
			"type": "",
		},
		"empty type in environment configuration",
	}, {
		attrs{
			"type": "my-type",
		},
		"name: expected string, got nothing",
	}, {
		attrs{
			"type": "my-type",
			"name": "",
		},
		"empty name in environment configuration",
	}, {
		// Default firewall mode.
		attrs{
			"type":          "my-type",
			"name":          "my-name",
			"firewall-mode": config.FwDefault,
		},
		"",
	}, {
		// Global firewall mode.
		attrs{
			"type":          "my-type",
			"name":          "my-name",
			"firewall-mode": config.FwGlobal,
		},
		"",
	}, {
		// Illegal firewall mode.
		attrs{
			"type":          "my-type",
			"name":          "my-name",
			"firewall-mode": "illegal",
		},
		"invalid firewall mode in environment configuration: .*",
	},
}

func (*ConfigSuite) TestConfig(c *C) {
	homedir := c.MkDir()
	sshdir := filepath.Join(homedir, ".ssh")

	defer os.Setenv("HOME", os.Getenv("HOME"))
	os.Setenv("HOME", homedir)

	err := os.Mkdir(sshdir, 0777)
	c.Assert(err, IsNil)

	kfiles := []struct{ name, data string }{
		{"id_dsa.pub", "dsa"},
		{"id_rsa.pub", "rsa\n"},
		{"identity.pub", "identity"},
		{"authorized_keys", "auth0\n# first\nauth1\n\n"},
		{"authorized_keys2", "auth2\nauth3\n"},
	}
	for _, kf := range kfiles {
		err = ioutil.WriteFile(filepath.Join(sshdir, kf.name), []byte(kf.data), 0666)
		c.Assert(err, IsNil)
	}

	for i, test := range configTests {
		c.Logf("test %d", i)
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

		if path, _ := test.attrs["authorized-keys-path"].(string); path != "" {
			for _, kf := range kfiles {
				if kf.name == filepath.Base(path) {
					c.Assert(cfg.AuthorizedKeys(), Equals, kf.data)
					path = ""
					break
				}
			}
			if path != "" {
				c.Fatalf("authorized-keys-path refers to unknown test file: %s", path)
			}
			c.Assert(cfg.AllAttrs()["authorized-keys-path"], Equals, nil)
		} else if keys, _ := test.attrs["authorized-keys"].(string); keys != "" {
			c.Assert(cfg.AuthorizedKeys(), Equals, keys)
		} else {
			// Content of all the files that are read by default.
			want := "dsa\nrsa\nidentity\n"
			c.Assert(cfg.AuthorizedKeys(), Equals, want)
		}
	}
}

func (*ConfigSuite) TestConfigAttrs(c *C) {
	attrs := map[string]interface{}{
		"type":            "my-type",
		"name":            "my-name",
		"authorized-keys": "my-keys",
		"firewall-mode":   string(config.FwDefault),
		"default-series":  version.Current.Series,
		"unknown":         "my-unknown",
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
