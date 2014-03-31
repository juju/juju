// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
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
	c.Assert(err, gc.IsNil)

	env, err := prov.Open(cfg)
	c.Assert(err, gc.IsNil)

	c.Check(env.Name(), gc.Equals, attrs["name"])
}

func (environProviderSuite) TestOpenReturnsNilInterfaceUponFailure(c *gc.C) {
	prov := azureEnvironProvider{}
	attrs := makeAzureConfigMap(c)
	// Make the config invalid.
	attrs["location"] = ""
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)

	env, err := prov.Open(cfg)
	// When Open() fails (i.e. returns a non-nil error), it returns an
	// environs.Environ interface object with a nil value and a nil
	// type.
	c.Check(env, gc.Equals, nil)
	c.Check(err, gc.ErrorMatches, ".*environment has no location; you need to set one.*")
}
