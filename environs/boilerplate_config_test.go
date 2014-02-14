// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju/osenv"
	_ "launchpad.net/juju-core/provider/ec2"
	_ "launchpad.net/juju-core/provider/manual"
	_ "launchpad.net/juju-core/provider/openstack"
)

type BoilerplateConfigSuite struct {
}

var _ = gc.Suite(&BoilerplateConfigSuite{})

func (*BoilerplateConfigSuite) TestBoilerPlateGeneration(c *gc.C) {
	defer osenv.SetJujuHome(osenv.SetJujuHome(c.MkDir()))
	boilerplate_text := environs.BoilerplateConfig()
	_, err := environs.ReadEnvironsBytes([]byte(boilerplate_text))
	c.Assert(err, gc.IsNil)
}

func (*BoilerplateConfigSuite) TestBoilerPlateAliases(c *gc.C) {
	defer osenv.SetJujuHome(osenv.SetJujuHome(c.MkDir()))
	boilerplate_text := environs.BoilerplateConfig()
	// There should be only one occurrence of "manual", despite
	// there being an alias ("null"). There should be nothing for
	// aliases.
	n := strings.Count(boilerplate_text, "type: manual")
	c.Assert(n, gc.Equals, 1)
	n = strings.Count(boilerplate_text, "type: null")
	c.Assert(n, gc.Equals, 0)
}
