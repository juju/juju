package config_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
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
	}}

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

	for _, test := range configTests {
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

		if series, _ := test.attrs["default-series"].(string); series != "" {
			c.Assert(cfg.DefaultSeries(), Equals, series)
		} else {
			c.Assert(cfg.DefaultSeries(), Equals, config.CurrentSeries)
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
			c.Assert(cfg.Map()["authorized-keys-path"], Equals, nil)
		} else if keys, _ := test.attrs["authorized-keys"].(string); keys != "" {
			c.Assert(cfg.AuthorizedKeys(), Equals, keys)
		} else {
			// Content of all the files that are read by default.
			want := "dsa\nrsa\nidentity\n"
			c.Assert(cfg.AuthorizedKeys(), Equals, want)
		}
	}
}
