// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
)

type EnvironProviderSuite struct {
	ProviderSuite
}

var _ = Suite(new(EnvironProviderSuite))

func (EnvironProviderSuite) TestOpen(c *C) {
	prov := azureEnvironProvider{}
	attrs := makeAzureConfigMap(c)
	attrs["name"] = "my-shiny-new-env"
	cfg, err := config.New(attrs)
	c.Assert(err, IsNil)

	env, err := prov.Open(cfg)
	c.Assert(err, IsNil)

	c.Check(env.Name(), Equals, attrs["name"])
}
