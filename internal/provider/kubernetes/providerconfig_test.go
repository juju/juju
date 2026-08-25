// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	provider "github.com/juju/juju/internal/provider/kubernetes"
	"github.com/juju/juju/internal/testing"
)

func (s *providerSuite) configProvider(c *tc.C) environs.ModelConfigProvider {
	return s.provider.(environs.ModelConfigProvider)
}

func (s *providerSuite) TestConfigSchemaEnableServiceLinks(c *tc.C) {
	schemaFields := s.configProvider(c).ConfigSchema()
	_, ok := schemaFields[provider.EnableServiceLinksKey]
	c.Assert(ok, tc.IsTrue)
}

func (s *providerSuite) TestModelConfigDefaultsEnableServiceLinks(c *tc.C) {
	defaults, err := s.configProvider(c).ModelConfigDefaults(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(defaults[provider.EnableServiceLinksKey], tc.Equals, true)
}

func (s *providerSuite) TestValidateEnableServiceLinks(c *tc.C) {
	config := fakeConfig(c, testing.Attrs{
		provider.EnableServiceLinksKey: false,
	})
	validCfg, err := s.provider.Validate(c.Context(), config, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(validCfg.AllAttrs()[provider.EnableServiceLinksKey], tc.Equals, false)
}

func (s *providerSuite) TestValidateEnableServiceLinksInvalid(c *tc.C) {
	config := fakeConfig(c, testing.Attrs{
		provider.EnableServiceLinksKey: "not-a-bool",
	})
	_, err := s.provider.Validate(c.Context(), config, nil)
	c.Assert(err, tc.NotNil)
}
