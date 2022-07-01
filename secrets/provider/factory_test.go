// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/secrets"
	"github.com/juju/juju/v3/secrets/provider"
	"github.com/juju/juju/v3/state"
)

type FactorySuite struct{}

var _ = gc.Suite(&FactorySuite{})

func (s *FactorySuite) TestUnknownProvider(c *gc.C) {
	cfg := secrets.ProviderConfig{}
	_, err := provider.NewSecretProvider("foo", cfg)
	c.Assert(err, gc.ErrorMatches, `secrets provider type "foo" not supported`)
}

func (s *FactorySuite) TestJujuProvider(c *gc.C) {
	cfg := secrets.ProviderConfig{
		"juju-backend": &state.State{},
	}
	p, err := provider.NewSecretProvider("juju", cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.NotNil)
}
