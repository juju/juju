// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/lxd"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools/lxdclient"
)

type configSuite struct {
	lxd.BaseSuite

	config *config.Config
}

var _ = gc.Suite(&configSuite{})

func (s *configSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	cfg, err := testing.ModelConfig(c).Apply(lxd.ConfigAttrs)
	c.Assert(err, jc.ErrorIsNil)
	s.config = cfg
}

func (s *configSuite) TestDefaults(c *gc.C) {
	cfg := lxd.NewBaseConfig(c)
	ecfg := lxd.NewConfig(cfg)

	values, extras := ecfg.Values(c)
	c.Assert(extras, gc.HasLen, 0)

	c.Check(values, jc.DeepEquals, lxd.ConfigValues{
		Namespace:  cfg.Name(),
		RemoteURL:  "",
		ClientCert: "",
		ClientKey:  "",
		ServerCert: "",
	})
}

func (s *configSuite) TestClientConfigLocal(c *gc.C) {
	cfg := lxd.NewBaseConfig(c)
	ecfg := lxd.NewConfig(cfg)
	values, _ := ecfg.Values(c)
	c.Assert(values.RemoteURL, gc.Equals, "")

	clientCfg, err := ecfg.ClientConfig()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(clientCfg, jc.DeepEquals, lxdclient.Config{
		Namespace: cfg.Name(),
		Remote: lxdclient.Remote{
			Name:          "juju-remote",
			Host:          "",
			Cert:          nil,
			ServerPEMCert: "",
		},
	})
}

func (s *configSuite) TestClientConfigNonLocal(c *gc.C) {
	cfg := lxd.NewBaseConfig(c)
	ecfg := lxd.NewConfig(cfg)
	ecfg = ecfg.Apply(c, map[string]interface{}{
		"remote-url":  "10.0.0.1",
		"client-cert": "<a valid x.509 cert>",
		"client-key":  "<a valid x.509 key>",
		"server-cert": "<a valid x.509 server cert>",
	})

	clientCfg, err := ecfg.ClientConfig()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(clientCfg, jc.DeepEquals, lxdclient.Config{
		Namespace: cfg.Name(),
		Remote: lxdclient.Remote{
			Name: "juju-remote",
			Host: "10.0.0.1",
			Cert: &lxdclient.Cert{
				Name:    fmt.Sprintf("juju cert for env %q", s.config.Name()),
				CertPEM: []byte("<a valid x.509 cert>"),
				KeyPEM:  []byte("<a valid x.509 key>"),
			},
			ServerPEMCert: "<a valid x.509 server cert>",
		},
	})
}

func (s *configSuite) TestUpdateForClientConfigLocal(c *gc.C) {
	cfg := lxd.NewBaseConfig(c)
	ecfg := lxd.NewConfig(cfg)

	clientCfg, err := ecfg.ClientConfig()
	c.Assert(err, jc.ErrorIsNil)
	updated, err := ecfg.UpdateForClientConfig(clientCfg)
	c.Assert(err, jc.ErrorIsNil)

	values, extras := updated.Values(c)
	c.Assert(extras, gc.HasLen, 0)

	c.Check(values, jc.DeepEquals, lxd.ConfigValues{
		Namespace:  cfg.Name(),
		RemoteURL:  "",
		ClientCert: "",
		ClientKey:  "",
		ServerCert: "",
	})
}

func (s *configSuite) TestUpdateForClientConfigNonLocal(c *gc.C) {
	cfg := lxd.NewBaseConfig(c)
	ecfg := lxd.NewConfig(cfg)
	ecfg = ecfg.Apply(c, map[string]interface{}{
		"remote-url":  "10.0.0.1",
		"client-cert": "<a valid x.509 cert>",
		"client-key":  "<a valid x.509 key>",
		"server-cert": "<a valid x.509 server cert>",
	})

	before, extras := ecfg.Values(c)
	c.Assert(extras, gc.HasLen, 0)

	clientCfg, err := ecfg.ClientConfig()
	c.Assert(err, jc.ErrorIsNil)
	updated, err := ecfg.UpdateForClientConfig(clientCfg)
	c.Assert(err, jc.ErrorIsNil)

	after, extras := updated.Values(c)
	c.Assert(extras, gc.HasLen, 0)

	c.Check(before, jc.DeepEquals, lxd.ConfigValues{
		Namespace:  cfg.Name(),
		RemoteURL:  "10.0.0.1",
		ClientCert: "<a valid x.509 cert>",
		ClientKey:  "<a valid x.509 key>",
		ServerCert: "<a valid x.509 server cert>",
	})
	c.Check(after, jc.DeepEquals, lxd.ConfigValues{
		Namespace:  cfg.Name(),
		RemoteURL:  "10.0.0.1",
		ClientCert: "<a valid x.509 cert>",
		ClientKey:  "<a valid x.509 key>",
		ServerCert: "<a valid x.509 server cert>",
	})
}

func (s *configSuite) TestUpdateForClientConfigGeneratedCert(c *gc.C) {
	cfg := lxd.NewBaseConfig(c)
	ecfg := lxd.NewConfig(cfg)
	ecfg = ecfg.Apply(c, map[string]interface{}{
		"remote-url":  "10.0.0.1",
		"client-cert": "",
		"client-key":  "",
		"server-cert": "",
	})

	before, extras := ecfg.Values(c)
	c.Assert(extras, gc.HasLen, 0)

	clientCfg, err := ecfg.ClientConfig()
	c.Assert(err, jc.ErrorIsNil)
	updated, err := ecfg.UpdateForClientConfig(clientCfg)
	c.Assert(err, jc.ErrorIsNil)

	after, extras := updated.Values(c)
	c.Assert(extras, gc.HasLen, 0)

	c.Check(before, jc.DeepEquals, lxd.ConfigValues{
		Namespace:  cfg.Name(),
		RemoteURL:  "10.0.0.1",
		ClientCert: "",
		ClientKey:  "",
		ServerCert: "",
	})
	after.CheckCert(c)
	after.ClientCert = ""
	after.ClientKey = ""
	after.ServerCert = ""
	c.Check(after, jc.DeepEquals, lxd.ConfigValues{
		Namespace:  cfg.Name(),
		RemoteURL:  "10.0.0.1",
		ClientCert: "",
		ClientKey:  "",
		ServerCert: "",
	})
}

// TODO(ericsnow) Each test only deals with a single field, so having
// multiple values in insert and remove (in configTestSpec) is a little
// misleading and unecessary.

// configTestSpec defines a subtest to run in a table driven test.
type configTestSpec struct {
	// info describes the subtest.
	info string
	// insert holds attrs that should be merged into the config.
	insert testing.Attrs
	// remove has the names of attrs that should be removed.
	remove []string
	// expect defines the expected attributes in a success case.
	expect testing.Attrs
	// err is the error message to expect in a failure case.
	err string
}

func (ts configTestSpec) checkSuccess(c *gc.C, value interface{}, err error) {
	if !c.Check(err, jc.ErrorIsNil) {
		return
	}

	var cfg *config.Config
	switch typed := value.(type) {
	case *config.Config:
		cfg = typed
	case environs.Environ:
		cfg = typed.Config()
	}

	attrs := cfg.AllAttrs()
	for field, value := range ts.expect {
		c.Check(attrs[field], gc.Equals, value)
	}
}

func (ts configTestSpec) checkFailure(c *gc.C, err error, msg string) {
	c.Check(err, gc.ErrorMatches, msg+": "+ts.err)
}

func (ts configTestSpec) checkAttrs(c *gc.C, attrs map[string]interface{}, cfg *config.Config) {
	for field, expected := range cfg.UnknownAttrs() {
		value := attrs[field]
		c.Check(value, gc.Equals, expected)
	}
}

func (ts configTestSpec) attrs() testing.Attrs {
	attrs := lxd.ConfigAttrs
	return attrs.Merge(ts.insert).Delete(ts.remove...)
}

func (ts configTestSpec) newConfig(c *gc.C) *config.Config {
	attrs := ts.attrs()
	cfg, err := testing.ModelConfig(c).Apply(attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func (ts configTestSpec) fixCfg(c *gc.C, cfg *config.Config) *config.Config {
	fixes := make(map[string]interface{})

	// Set changed values.
	fixes = updateAttrs(fixes, ts.insert)

	newCfg, err := cfg.Apply(fixes)
	c.Assert(err, jc.ErrorIsNil)
	return newCfg
}

func updateAttrs(attrs, updates testing.Attrs) testing.Attrs {
	updated := make(testing.Attrs, len(attrs))
	for k, v := range attrs {
		updated[k] = v
	}
	for k, v := range updates {
		updated[k] = v
	}
	return updated
}

var newConfigTests = []configTestSpec{{
	info:   "namespace is optional",
	remove: []string{"namespace"},
	expect: testing.Attrs{"namespace": "testenv"},
}, {
	info:   "namespace can be empty",
	insert: testing.Attrs{"namespace": ""},
	expect: testing.Attrs{"namespace": "testenv"},
}, {
	info:   "remote-url is optional",
	remove: []string{"remote-url"},
	expect: testing.Attrs{"remote-url": ""},
}, {
	info:   "remote-url can be empty",
	insert: testing.Attrs{"remote-url": ""},
	expect: testing.Attrs{"remote-url": ""},
}, {
	info:   "client-cert is optional",
	remove: []string{"client-cert"},
	expect: testing.Attrs{"client-cert": ""},
}, {
	info:   "client-cert can be empty",
	insert: testing.Attrs{"client-cert": ""},
	expect: testing.Attrs{"client-cert": ""},
}, {
	info:   "client-key is optional",
	remove: []string{"client-key"},
	expect: testing.Attrs{"client-key": ""},
}, {
	info:   "client-key can be empty",
	insert: testing.Attrs{"client-key": ""},
	expect: testing.Attrs{"client-key": ""},
}, {
	info:   "server-cert is optional",
	remove: []string{"server-cert"},
	expect: testing.Attrs{"server-cert": ""},
}, {
	info:   "unknown field is not touched",
	insert: testing.Attrs{"unknown-field": 12345},
	expect: testing.Attrs{"unknown-field": 12345},
}}

func (s *configSuite) TestNewModelConfig(c *gc.C) {
	// TODO(ericsnow) Move to a functional suite.
	if !s.IsRunningLocally(c) {
		c.Skip("LXD not running locally")
	}

	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		testConfig := test.newConfig(c)
		environ, err := environs.New(testConfig)

		// Check the result
		if test.err != "" {
			test.checkFailure(c, err, "invalid config")
		} else {
			test.checkSuccess(c, environ, err)
		}
	}
}

// TODO(wwitzel3) refactor to provider_test file
func (s *configSuite) TestValidateNewConfig(c *gc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		testConfig := test.newConfig(c)
		validatedConfig, err := lxd.Provider.Validate(testConfig, nil)

		// Check the result
		if test.err != "" {
			test.checkFailure(c, err, "invalid config")
		} else {
			c.Check(validatedConfig, gc.NotNil)
			test.checkSuccess(c, validatedConfig, err)
		}
	}
}

// TODO(wwitzel3) refactor to the provider_test file
func (s *configSuite) TestValidateOldConfig(c *gc.C) {
	for i, test := range newConfigTests {
		c.Logf("test %d: %s", i, test.info)

		oldcfg := test.newConfig(c)
		var err error
		oldcfg, err = lxd.Provider.Validate(oldcfg, nil)
		c.Assert(err, jc.ErrorIsNil)
		newcfg := test.fixCfg(c, s.config)
		expected := updateAttrs(lxd.ConfigAttrs, test.insert)

		// Validate the new config (relative to the old one) using the
		// provider.
		validatedConfig, err := lxd.Provider.Validate(newcfg, oldcfg)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid base config")
		} else {
			if !c.Check(err, jc.ErrorIsNil) {
				continue
			}
			// We verify that Validate filled in the defaults
			// appropriately.
			c.Check(validatedConfig, gc.NotNil)
			test.checkAttrs(c, expected, validatedConfig)
		}
	}
}

// TODO(ericsnow) Add tests for client-cert and client-key.

var changeConfigTests = []configTestSpec{{
	info:   "no change, no error",
	expect: lxd.ConfigAttrs,
}, {
	info:   "cannot change namespace",
	insert: testing.Attrs{"namespace": "spam"},
	err:    "namespace: cannot change from testenv to spam",
	//}, {
	// TODO(ericsnow) This triggers cert generation...
	//	info:   "cannot change remote-url",
	//	insert: testing.Attrs{"remote-url": "eggs"},
	//	err:    "remote-url: cannot change from  to eggs",
}, {
	info:   "can insert unknown field",
	insert: testing.Attrs{"unknown": "ignoti"},
	expect: testing.Attrs{"unknown": "ignoti"},
}}

// TODO(wwitzel3) refactor this to the provider_test file.
func (s *configSuite) TestValidateChange(c *gc.C) {
	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)

		testConfig := test.newConfig(c)
		validatedConfig, err := lxd.Provider.Validate(testConfig, s.config)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid config change")
		} else {
			test.checkSuccess(c, validatedConfig, err)
		}
	}
}

func (s *configSuite) TestSetConfig(c *gc.C) {
	// TODO(ericsnow) Move to a functional suite.
	if !s.IsRunningLocally(c) {
		c.Skip("LXD not running locally")
	}

	for i, test := range changeConfigTests {
		c.Logf("test %d: %s", i, test.info)

		environ, err := environs.New(s.config)
		c.Assert(err, jc.ErrorIsNil)

		testConfig := test.newConfig(c)
		err = environ.SetConfig(testConfig)

		// Check the result.
		if test.err != "" {
			test.checkFailure(c, err, "invalid config change")
			expected, err := lxd.Provider.Validate(s.config, nil)
			c.Assert(err, jc.ErrorIsNil)
			test.checkAttrs(c, environ.Config().AllAttrs(), expected)
		} else {
			test.checkSuccess(c, environ.Config(), err)
		}
	}
}
