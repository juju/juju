// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/tools/lxdclient"
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
		Remote:    s.remote,
	}
	updated, err := cfg.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updated, jc.DeepEquals, cfg)
}

func (s *configSuite) TestWithDefaultsMissingRemote(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "my-ns",
	}
	updated, err := cfg.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updated, jc.DeepEquals, lxdclient.Config{
		Namespace: "my-ns",
		Remote:    lxdclient.Local,
	})
}

func (s *configSuite) TestValidateOkay(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Remote:    s.remote,
	}
	err := cfg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *configSuite) TestValidateOnlyRemote(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "",
		Remote:    s.remote,
	}
	err := cfg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *configSuite) TestValidateMissingRemote(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "my-ns",
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

func (s *configSuite) TestUsingTCPRemoteOkay(c *gc.C) {
	// TODO(ericsnow) Finish!
}

func (s *configSuite) TestUsingTCPRemoteNoop(c *gc.C) {
	cfg := lxdclient.Config{
		Namespace: "my-ns",
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

func (s *configFunctionalSuite) TestUsingTCPRemote(c *gc.C) {
	if s.client == nil {
		c.Skip("LXD not running locally")
	}

	cfg := lxdclient.Config{
		Namespace: "my-ns",
		Remote:    lxdclient.Local,
	}
	nonlocal, err := cfg.UsingTCPRemote()
	c.Assert(err, jc.ErrorIsNil)

	checkValidRemote(c, &nonlocal.Remote)
	c.Check(nonlocal, jc.DeepEquals, lxdclient.Config{
		Namespace: "my-ns",
		Remote: lxdclient.Remote{
			Name:          lxdclient.Local.Name,
			Host:          nonlocal.Remote.Host,
			Cert:          nonlocal.Remote.Cert,
			ServerPEMCert: nonlocal.Remote.ServerPEMCert,
		},
	})
	c.Check(nonlocal.Remote.Host, gc.Not(gc.Equals), "")
	c.Check(nonlocal.Remote.Cert.CertPEM, gc.Not(gc.Equals), "")
	c.Check(nonlocal.Remote.Cert.KeyPEM, gc.Not(gc.Equals), "")
	c.Check(nonlocal.Remote.ServerPEMCert, gc.Not(gc.Equals), "")
	// TODO(ericsnow) Check that the server has the certs.
}

func newLocalClient(c *gc.C) *lxdclient.Client {
	client, err := lxdclient.Connect(lxdclient.Config{
		Namespace: "my-ns",
		Remote:    lxdclient.Local,
	})
	if err != nil {
		c.Log(err)
		return nil
	}
	return client
}
