// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient_test

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/lxc/lxd"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/container/lxd/lxdclient"
)

var (
	_ = gc.Suite(&configSuite{})
	_ = gc.Suite(&configFunctionalSuite{})
)

type configBaseSuite struct {
	lxdclient.BaseSuite

	remote lxdclient.Remote
}

func (s *configBaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.remote = lxdclient.NewRemote(lxdclient.RemoteInfo{
		Name: "my-remote",
		Host: "some-host",
		Cert: s.Cert,
	})
}

type configSuite struct {
	configBaseSuite
}

func (s *configSuite) TestSetDefaultsOkay(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "some-dir",
		Filename:  "config.yaml",
		Remote:    s.remote,
	}
	updated, err := cfg.SetDefaults()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updated, jc.DeepEquals, cfg)
}

func (s *configSuite) TestSetDefaultsMissingDirname(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "",
		Filename:  "config.yaml",
		Remote:    s.remote,
	}
	updated, err := cfg.SetDefaults()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updated, jc.DeepEquals, lxdclient.Config{
		Namespace: "my-ns",
		// TODO(ericsnow)  This will change on Windows once the LXD
		// code is cross-platform.
		Dirname:  "/.config/lxc", // IsolationSuite sets $HOME to "".
		Filename: "config.yaml",
		Remote:   s.remote,
	})
}

func (s *configSuite) TestSetDefaultsFilename(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "some-dir",
		Filename:  "",
		Remote:    s.remote,
	}
	updated, err := cfg.SetDefaults()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updated, jc.DeepEquals, lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "some-dir",
		Filename:  "config.yml",
		Remote:    s.remote,
	})
}

func (s *configSuite) TestSetDefaultsMissingRemote(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "some-dir",
		Filename:  "config.yaml",
	}
	updated, err := cfg.SetDefaults()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updated, jc.DeepEquals, lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "some-dir",
		Filename:  "config.yaml",
		Remote:    lxdclient.Local,
	})
}

func (s *configSuite) TestValidateOkay(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "some-dir",
		Filename:  "config.yaml",
		Remote:    s.remote,
	}
	err := cfg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *configSuite) TestValidateOnlyRemote(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "",
		Dirname:   "",
		Filename:  "",
		Remote:    s.remote,
	}
	err := cfg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *configSuite) TestValidateMissingRemote(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "some-dir",
		Filename:  "config.yaml",
	}
	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *configSuite) TestValidateZeroValue(c *gc.C) {
	var cfg lxdclient.Config
	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *configSuite) TestApplyOkay(c *gc.C) {
	// TODO(ericsnow) Finish!
}

func (s *configSuite) TestApplyInvalid(c *gc.C) {
	var cfg lxdclient.Config
	err := cfg.Apply()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *configSuite) TestWriteOkay(c *gc.C) {
	// TODO(ericsnow) Finish!
}

func (s *configSuite) TestWriteInvalid(c *gc.C) {
	var cfg lxdclient.Config
	err := cfg.Write()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

type configFunctionalSuite struct {
	configBaseSuite
}

func (s *configFunctionalSuite) SetUpTest(c *gc.C) {
	s.configBaseSuite.SetUpTest(c)

	origConfigDir := lxd.ConfigDir
	s.AddCleanup(func(c *gc.C) {
		lxd.ConfigDir = origConfigDir
	})
}

func (s *configFunctionalSuite) TestApply(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "some-dir",
		Filename:  "config.yaml",
		Remote:    s.remote,
	}
	err := cfg.Apply()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(lxd.ConfigDir, gc.Equals, cfg.Dirname)
}

func (s *configFunctionalSuite) TestWrite(c *gc.C) {
	dirname := c.MkDir()
	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   dirname,
		Filename:  "config.yaml",
		Remote:    s.remote,
	}
	err := cfg.Write()
	c.Assert(err, jc.ErrorIsNil)

	checkFiles(c, cfg)
}

func checkFiles(c *gc.C, cfg lxdclient.Config) {
	certificate := cfg.Remote.Cert()

	certPEM, err := ioutil.ReadFile(filepath.Join(cfg.Dirname, "client.crt"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(certPEM), gc.Equals, string(certificate.CertPEM))

	keyPEM, err := ioutil.ReadFile(filepath.Join(cfg.Dirname, "client.key"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(keyPEM), gc.Equals, string(certificate.KeyPEM))

	configData, err := ioutil.ReadFile(filepath.Join(cfg.Dirname, cfg.Filename))
	c.Assert(err, jc.ErrorIsNil)
	var config lxd.Config
	err = goyaml.Unmarshal(configData, &config)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, jc.DeepEquals, lxd.Config{
		DefaultRemote: "local",
		Remotes: map[string]lxd.RemoteConfig{
			//"local": lxd.LocalRemote,
			"local": lxd.RemoteConfig{
				Addr:   "unix://",
				Public: false,
			},
			cfg.Remote.Name: lxd.RemoteConfig{
				Addr:   cfg.Remote.Host,
				Public: false,
			},
		},
		//Aliases: nil,
	})
}
