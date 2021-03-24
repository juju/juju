// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix_test

import (
	"net/http"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/equinix"
	"github.com/juju/juju/testing"
)

type environProviderSuite struct {
	testing.BaseSuite
	provider environs.EnvironProvider
	spec     environscloudspec.CloudSpec
	requests []*http.Request
}

var _ = gc.Suite(&environProviderSuite{})

func (s *environProviderSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.provider = newProvider()
	s.spec = environscloudspec.CloudSpec{
		Type:       "equnix",
		Name:       "equnix metal",
		Region:     "ams1",
		Endpoint:   "https://api.packet.net/",
		Credential: fakeServicePrincipalCredential(),
	}
}

func fakeServicePrincipalCredential() *cloud.Credential {
	cred := cloud.NewCredential(
		"service-principal-secret",
		map[string]string{
			"project-id": "12345c2a-6789-4d4f-a3c4-7367d6b7cca8",
			"api-token":  "some-token",
		},
	)
	return &cred
}

func (s *environProviderSuite) TestPrepareConfig(c *gc.C) {
	cfg := makeTestModelConfig(c)
	cfg, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud:  s.spec,
		Config: cfg,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg, gc.NotNil)
}

func (s *environProviderSuite) TestOpen(c *gc.C) {

	env, err := environs.Open(s.provider, environs.OpenParams{
		Cloud:  s.spec,
		Config: makeTestModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
}

func (s *environProviderSuite) testOpenError(c *gc.C, spec environscloudspec.CloudSpec, expect string) {
	_, err := environs.Open(s.provider, environs.OpenParams{
		Cloud:  spec,
		Config: makeTestModelConfig(c),
	})
	c.Assert(err, gc.ErrorMatches, expect)
}

func newProvider() environs.EnvironProvider {
	return equinix.NewProvider()
}

func makeTestModelConfig(c *gc.C, extra ...testing.Attrs) *config.Config {
	attrs := testing.Attrs{
		"type":          "equinix",
		"agent-version": "1.2.3",
	}
	for _, extra := range extra {
		attrs = attrs.Merge(extra)
	}
	attrs = testing.FakeConfig().Merge(attrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}
