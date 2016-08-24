// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/lxd"
	"github.com/juju/juju/provider/lxd/lxdnames"
	"github.com/juju/juju/tools/lxdclient"
)

// This is a quick hack to make wily pass with it's default, but unsupported,
// version of LXD. Wily is supported until 2016-7-??. AFAIU LXD will not be
// backported to wily... so we have this:|
// TODO(redir): Remove after wiley or in yakkety.
func skipIfWily(c *gc.C) {
	if series.HostSeries() == "wily" {
		cfg, _ := lxdclient.Config{}.WithDefaults()
		_, err := lxdclient.Connect(cfg)
		// We try to create a client here. On wily this should fail, because
		// the default 0.20 lxd version should make juju/tools/lxdclient return
		// an error.
		if err != nil {
			c.Skip(fmt.Sprintf("Skipping LXD tests because %s", err))
		}
	}
}

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

func (s *providerSuite) TestDetectRegions(c *gc.C) {
	c.Assert(s.provider, gc.Implements, new(environs.CloudRegionDetector))
	regions, err := s.provider.(environs.CloudRegionDetector).DetectRegions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(regions, jc.DeepEquals, []cloud.Region{{Name: lxdnames.DefaultRegion}})
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

type ProviderFunctionalSuite struct {
	lxd.BaseSuite

	provider environs.EnvironProvider
}

func (s *ProviderFunctionalSuite) SetUpTest(c *gc.C) {
	if !s.IsRunningLocally(c) {
		c.Skip("LXD not running locally")
	}

	// TODO(redir): Remove after wily or in yakkety.
	skipIfWily(c)

	s.BaseSuite.SetUpTest(c)

	provider, err := environs.Provider("lxd")
	c.Assert(err, jc.ErrorIsNil)

	s.provider = provider
}

func (s *ProviderFunctionalSuite) TestOpen(c *gc.C) {
	env, err := s.provider.Open(environs.OpenParams{
		Cloud:  lxdCloudSpec(),
		Config: s.Config,
	})
	c.Assert(err, jc.ErrorIsNil)
	envConfig := env.Config()

	c.Check(envConfig.Name(), gc.Equals, "testenv")
}

func (s *ProviderFunctionalSuite) TestPrepareConfig(c *gc.C) {
	cfg, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud:  lxdCloudSpec(),
		Config: s.Config,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg, gc.NotNil)
}

func (s *ProviderFunctionalSuite) TestPrepareConfigUnsupportedAuthType(c *gc.C) {
	cred := cloud.NewCredential(cloud.CertificateAuthType, nil)
	_, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud: environs.CloudSpec{
			Type:       "lxd",
			Name:       "remotehost",
			Credential: &cred,
		},
	})
	c.Assert(err, gc.ErrorMatches, `validating cloud spec: "certificate" auth-type not supported`)
}

func (s *ProviderFunctionalSuite) TestPrepareConfigNonEmptyEndpoint(c *gc.C) {
	_, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud: environs.CloudSpec{
			Type:     "lxd",
			Name:     "remotehost",
			Endpoint: "1.2.3.4",
		},
	})
	c.Assert(err, gc.ErrorMatches, `validating cloud spec: non-empty endpoint "1.2.3.4" not valid`)
}
