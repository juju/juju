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
	if series.MustHostSeries() == "wily" {
		cfg, _ := lxdclient.Config{}.WithDefaults()
		_, err := lxdclient.Connect(cfg, false)
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
}

func (s *providerSuite) TestDetectClouds(c *gc.C) {
	clouds, err := s.Provider.DetectClouds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 1)
	s.assertLocalhostCloud(c, clouds[0])
}

func (s *providerSuite) TestDetectCloud(c *gc.C) {
	cloud, err := s.Provider.DetectCloud("localhost")
	c.Assert(err, jc.ErrorIsNil)
	s.assertLocalhostCloud(c, cloud)
	cloud, err = s.Provider.DetectCloud("lxd")
	c.Assert(err, jc.ErrorIsNil)
	s.assertLocalhostCloud(c, cloud)
}

func (s *providerSuite) TestDetectCloudError(c *gc.C) {
	_, err := s.Provider.DetectCloud("foo")
	c.Assert(err, gc.ErrorMatches, `cloud foo not found`)
}

func (s *providerSuite) assertLocalhostCloud(c *gc.C, found cloud.Cloud) {
	c.Assert(found, jc.DeepEquals, cloud.Cloud{
		Name:      "localhost",
		Type:      "lxd",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Endpoint:  "1.2.3.4",
		Regions: []cloud.Region{{
			Name:     "localhost",
			Endpoint: "1.2.3.4",
		}},
		Description: "LXD Container Hypervisor",
	})
}

func (s *providerSuite) TestDetectRegions(c *gc.C) {
	regions, err := s.Provider.DetectRegions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(regions, jc.DeepEquals, []cloud.Region{{Name: lxdnames.DefaultRegion}})
}

func (s *providerSuite) TestValidate(c *gc.C) {
	validCfg, err := s.Provider.Validate(s.Config, nil)
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
	cred := cloud.NewCredential("foo", nil)
	_, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud: environs.CloudSpec{
			Type:       "lxd",
			Name:       "remotehost",
			Credential: &cred,
		},
	})
	c.Assert(err, gc.ErrorMatches, `validating cloud spec: "foo" auth-type not supported`)
}

func (s *ProviderFunctionalSuite) TestPrepareConfigInvalidCertificateAttrs(c *gc.C) {
	cred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{})
	_, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud: environs.CloudSpec{
			Type:       "lxd",
			Name:       "remotehost",
			Credential: &cred,
		},
	})
	c.Assert(err, gc.ErrorMatches, `validating cloud spec: certificate credentials not valid`)
}

func (s *ProviderFunctionalSuite) TestPrepareConfigEmptyAuthNonLocal(c *gc.C) {
	cred := cloud.NewEmptyCredential()
	_, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud: environs.CloudSpec{
			Type:       "lxd",
			Name:       "remotehost",
			Endpoint:   "8.8.8.8",
			Credential: &cred,
		},
	})
	c.Assert(err, gc.ErrorMatches, `validating cloud spec: "empty" auth-type for non-local LXD not supported`)
}
