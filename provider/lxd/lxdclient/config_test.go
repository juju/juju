// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient_test

import (
	"io/ioutil"
	"path"
	"path/filepath"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	"github.com/lxc/lxd"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/provider/lxd/lxdclient"
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

	s.remote = lxdclient.Remote{
		Name: "my-remote",
		Host: "some-host",
		Cert: s.Cert,
	}
}

type configSuite struct {
	configBaseSuite
}

func (s *configSuite) TestWithDefaultsOkay(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "some-dir",
		Remote:    s.remote,
	}
	updated, err := cfg.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updated, jc.DeepEquals, cfg)
}

func (s *configSuite) TestWithDefaultsMissingDirname(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "",
		Remote:    s.remote,
	}
	updated, err := cfg.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("path.Clean of dirname is %s (dirname is %s)", path.Clean(updated.Dirname), updated.Dirname)
	c.Check(updated, jc.DeepEquals, lxdclient.Config{
		Namespace: "my-ns",
		// TODO(ericsnow)  This will change on Windows once the LXD
		// code is cross-platform.
		Dirname: "/.config/lxc",
		Remote:  s.remote,
	})
}

func (s *configSuite) TestWithDefaultsMissingRemote(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "some-dir",
	}
	updated, err := cfg.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updated, jc.DeepEquals, lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "some-dir",
		Remote:    lxdclient.Local,
	})
}

func (s *configSuite) TestValidateOkay(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "some-dir",
		Remote:    s.remote,
	}
	err := cfg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *configSuite) TestValidateOnlyRemote(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "",
		Dirname:   "",
		Remote:    s.remote,
	}
	err := cfg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *configSuite) TestValidateMissingRemote(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "some-dir",
	}
	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *configSuite) TestValidateZeroValue(c *gc.C) {
	var cfg lxdclient.Config
	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *configSuite) TestWriteOkay(c *gc.C) {
	c.Skip("not implemented yet")
	// TODO(ericsnow) Finish!
}

func (s *configSuite) TestWriteRemoteAlreadySet(c *gc.C) {
	c.Skip("not implemented yet")
	// TODO(ericsnow) Finish!
}

func (s *configSuite) TestWriteInvalid(c *gc.C) {
	var cfg lxdclient.Config
	err := cfg.Write()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *configSuite) TestUsingTCPRemoteOkay(c *gc.C) {
	// TODO(ericsnow) Finish!
}

func (s *configSuite) TestUsingTCPRemoteNoop(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "some-dir",
		Remote:    s.remote,
	}
	nonlocal, err := cfg.UsingTCPRemote()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(nonlocal, jc.DeepEquals, cfg)
}

type configFunctionalSuite struct {
	configBaseSuite

	client *lxdclient.Client
}

func (s *configFunctionalSuite) SetUpTest(c *gc.C) {
	s.configBaseSuite.SetUpTest(c)

	s.client = newLocalClient(c)

	origConfigDir := lxd.ConfigDir
	s.AddCleanup(func(c *gc.C) {
		lxd.ConfigDir = origConfigDir
	})

	if s.client != nil {
		origCerts, err := s.client.ListCerts()
		c.Assert(err, jc.ErrorIsNil)
		s.AddCleanup(func(c *gc.C) {
			certs, err := s.client.ListCerts()
			c.Assert(err, jc.ErrorIsNil)

			orig := set.NewStrings(origCerts...)
			added := set.NewStrings(certs...).Difference(orig)
			for _, fingerprint := range added.Values() {
				err := s.client.RemoveCertByFingerprint(fingerprint)
				if err != nil {
					c.Logf("could not remove cert %q: %v", fingerprint, err)
				}
			}
		})
	}
}

func (s *configFunctionalSuite) TestWrite(c *gc.C) {
	dirname := c.MkDir()
	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   dirname,
		Remote:    s.remote,
	}
	err := cfg.Write()
	c.Assert(err, jc.ErrorIsNil)

	checkFiles(c, cfg)
}

func (s *configFunctionalSuite) TestUsingTCPRemote(c *gc.C) {
	if s.client == nil {
		c.Skip("LXD not running locally")
	}

	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "some-dir",
		Remote:    lxdclient.Local,
	}
	nonlocal, err := cfg.UsingTCPRemote()
	c.Assert(err, jc.ErrorIsNil)

	checkValidRemote(c, &nonlocal.Remote)
	c.Check(nonlocal, jc.DeepEquals, lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   "some-dir",
		Remote: lxdclient.Remote{
			Name: lxdclient.Local.Name,
			Host: nonlocal.Remote.Host,
			Cert: nonlocal.Remote.Cert,
		},
	})
	// TODO(ericsnow) Check that the server has the certs.
}

func newLocalClient(c *gc.C) *lxdclient.Client {
	origConfigDir := lxd.ConfigDir
	defer func() {
		lxd.ConfigDir = origConfigDir
	}()

	client, err := lxdclient.Connect(lxdclient.Config{
		Namespace: "my-ns",
		Dirname:   c.MkDir(),
		Remote:    lxdclient.Local,
	})
	if err != nil {
		c.Log(err)
		return nil
	}
	return client
}

func checkFiles(c *gc.C, cfg lxdclient.Config) {
	var certificate lxdclient.Cert
	if cfg.Remote.Cert != nil {
		certificate = *cfg.Remote.Cert
	}

	filename := filepath.Join(cfg.Dirname, "client.crt")
	c.Logf("reading cert PEM from %q", filename)
	certPEM, err := ioutil.ReadFile(filename)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(certPEM), gc.Equals, string(certificate.CertPEM))

	filename = filepath.Join(cfg.Dirname, "client.key")
	c.Logf("reading key PEM from %q", filename)
	keyPEM, err := ioutil.ReadFile(filename)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(keyPEM), gc.Equals, string(certificate.KeyPEM))

	filename = filepath.Join(cfg.Dirname, "config.yml")
	c.Logf("reading config from %q", filename)
	configData, err := ioutil.ReadFile(filename)
	c.Assert(err, jc.ErrorIsNil)
	var config lxd.Config
	err = goyaml.Unmarshal(configData, &config)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config.Aliases, gc.HasLen, 0)
	config.Aliases = nil
	c.Check(config, jc.DeepEquals, lxd.Config{
		DefaultRemote: "local",
		Remotes: map[string]lxd.RemoteConfig{
			"local": lxd.LocalRemote,
			cfg.Remote.Name: lxd.RemoteConfig{
				Addr:   "https://" + cfg.Remote.Host + ":8443",
				Public: false,
			},
		},
		Aliases: nil,
	})
}
