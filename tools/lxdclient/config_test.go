// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/tools/lxdclient"
)

var (
	_ = gc.Suite(&configSuite{})
)

type configBaseSuite struct {
	lxdclient.BaseSuite

	remote lxdclient.Remote
}

func (s *configBaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.remote = lxdclient.Remote{
		Name:     "my-remote",
		Host:     "some-host",
		Protocol: lxdclient.LXDProtocol,
		Cert:     s.Cert,
	}
}

type configSuite struct {
	configBaseSuite
}

func (s *configSuite) TestWithDefaultsOkay(c *gc.C) {
	cfg := lxdclient.Config{
		Remote: s.remote,
	}
	updated, err := cfg.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updated, jc.DeepEquals, cfg)
}

func (s *configSuite) TestWithDefaultsMissingRemote(c *gc.C) {
	cfg := lxdclient.Config{}
	updated, err := cfg.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updated, jc.DeepEquals, lxdclient.Config{
		Remote: lxdclient.Local,
	})
}

func (s *configSuite) TestWithDefaultsMissingStream(c *gc.C) {
	cfg := lxdclient.Config{
		Remote: s.remote,
	}
	updated, err := cfg.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updated, jc.DeepEquals, lxdclient.Config{
		Remote: s.remote,
	})
}

func (s *configSuite) TestValidateOkay(c *gc.C) {
	cfg := lxdclient.Config{
		Remote: s.remote,
	}
	err := cfg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *configSuite) TestValidateOnlyRemote(c *gc.C) {
	cfg := lxdclient.Config{
		Remote: s.remote,
	}
	err := cfg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *configSuite) TestValidateMissingRemote(c *gc.C) {
	cfg := lxdclient.Config{}
	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *configSuite) TestValidateZeroValue(c *gc.C) {
	var cfg lxdclient.Config
	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}
