// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/lxd"
)

var (
	_ = gc.Suite(&providerSuite{})
	_ = gc.Suite(&ProviderFunctionalSuite{})
)

type providerSuite struct {
	lxd.BaseSuite

	provider environs.EnvironProvider
}

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	provider, err := environs.Provider("lxd")
	c.Assert(err, jc.ErrorIsNil)
	s.provider = provider
}

func (s *providerSuite) TestRegistered(c *gc.C) {
	c.Check(s.provider, gc.Equals, lxd.Provider)
}

func (s *providerSuite) TestValidate(c *gc.C) {
	validCfg, err := s.provider.Validate(s.Config, nil)
	c.Assert(err, jc.ErrorIsNil)
	validAttrs := validCfg.AllAttrs()

	c.Check(s.Config.AllAttrs(), gc.DeepEquals, validAttrs)
}

func (s *providerSuite) TestSecretAttrs(c *gc.C) {
	obtainedAttrs, err := s.provider.SecretAttrs(s.Config)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(obtainedAttrs, gc.HasLen, 0)
}

func (s *providerSuite) TestBoilerplateConfig(c *gc.C) {
	// (wwitzel3) purposefully duplicated here so that this test will
	// fail if someone updates lxd/config.go without updating this test.
	var expected = `
lxd:
    type: lxd

    # namespace identifies the namespace to associate with containers
    # created by the provider.  It is prepended to the container names.
    # By default the environment's name is used as the namespace.
    #
    # Setting the namespace is useful when more than one environment
    # is using the same remote (e.g. the local LXD socket).
    #
    # namespace: lxd

    # remote-url is the URL to the LXD API server to use for managing
    # containers, if any. If not specified then the locally running LXD
    # server is used.
    #
    # Note: Juju does not set up remotes for you. Run the following
    # commands on an LXD remote's host to install LXD:
    #
    #   add-apt-repository ppa:ubuntu-lxc/lxd-stable
    #   apt-get update
    #   apt-get install lxd
    #
    # Before using a locally running LXD (the default for this provider)
    # after installing it, either through Juju or the LXD CLI ("lxc"),
    # you must either log out and back in or run this command:
    #
    #   newgrp lxd
    #
    # You will also need to prepare the "ubuntu" images that Juju uses:
    #
    #   lxc remote add images images.linuxcontainers.org
    #   lxd-images import ubuntu --alias ubuntu-wily wily
    #
    # (Also consider the --stream and --sync options.)
    #
    # You will need to prepare an image for each Ubuntu series for which
    # you want to create instances.  The alias must match the series:
    #
    #   lxd-images import ubuntu --alias ubuntu-trusty trusty
    #   lxd-images import ubuntu --alias ubuntu-wily wily
    #   lxd-images import ubuntu --alias ubuntu-xenial xenial
    #
    # See: https://linuxcontainers.org/lxd/getting-started-cli/
    #
    # Note: the LXD provider does not support using any series older
    # than wily for a controller instance.  However, non-controller
    # instances may be provisioned on earler series (e.g. trusty).
    #
    # remote-url:

    # The cert and key the client should use to connect to the remote
    # may also be provided. If not then they are auto-generated.
    #
    # client-cert:
    # client-key:

`[1:]
	boilerplateConfig := s.provider.BoilerplateConfig()

	c.Check(boilerplateConfig, gc.Equals, expected)
	c.Check(strings.Split(boilerplateConfig, "\n"), jc.DeepEquals, strings.Split(expected, "\n"))
}

type ProviderFunctionalSuite struct {
	lxd.BaseSuite

	provider environs.EnvironProvider
}

func (s *ProviderFunctionalSuite) SetUpTest(c *gc.C) {
	if !s.IsRunningLocally(c) {
		c.Skip("LXD not running locally")
	}

	s.BaseSuite.SetUpTest(c)

	provider, err := environs.Provider("lxd")
	c.Assert(err, jc.ErrorIsNil)

	s.provider = provider
}

func (s *ProviderFunctionalSuite) TestOpen(c *gc.C) {
	env, err := s.provider.Open(s.Config)
	c.Assert(err, jc.ErrorIsNil)
	envConfig := env.Config()

	c.Check(envConfig.Name(), gc.Equals, "testenv")
}

func (s *ProviderFunctionalSuite) TestPrepareForBootstrap(c *gc.C) {
	env, err := s.provider.PrepareForBootstrap(envtesting.BootstrapContext(c), s.Config)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(env, gc.NotNil)
}
