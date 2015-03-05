// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
)

type environProviderSuite struct {
	providerSuite
}

var _ = gc.Suite(&environProviderSuite{})

func (*environProviderSuite) TestOpen(c *gc.C) {
	prov := azureEnvironProvider{}
	attrs := makeAzureConfigMap(c)
	attrs["name"] = "my-shiny-new-env"
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	env, err := prov.Open(cfg)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(env.Config().Name(), gc.Equals, attrs["name"])
}

func (environProviderSuite) TestOpenReturnsNilInterfaceUponFailure(c *gc.C) {
	prov := azureEnvironProvider{}
	attrs := makeAzureConfigMap(c)
	// Make the config invalid.
	attrs["location"] = ""
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	env, err := prov.Open(cfg)
	// When Open() fails (i.e. returns a non-nil error), it returns an
	// environs.Environ interface object with a nil value and a nil
	// type.
	c.Check(env, gc.Equals, nil)
	c.Check(err, gc.ErrorMatches, ".*environment has no location; you need to set one.*")
}

func (*environProviderSuite) TestPrepareSetsAvailabilitySets(c *gc.C) {
	prov := azureEnvironProvider{}
	attrs := makeAzureConfigMap(c)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	// Make sure the the value isn't set.
	_, ok := cfg.AllAttrs()["availability-sets-enabled"]
	c.Assert(ok, jc.IsFalse)

	cfg, err = prov.PrepareForCreateEnvironment(cfg)
	c.Assert(err, jc.ErrorIsNil)
	value, ok := cfg.AllAttrs()["availability-sets-enabled"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, jc.IsTrue)
}
