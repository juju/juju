// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package null_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/null"
	"launchpad.net/juju-core/testing/testbase"
)

type instanceSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&instanceSuite{})

func (s *instanceSuite) TestAddresses(c *gc.C) {
	configValues := null.MinimalConfigValues()
	configValues["bootstrap-host"] = "boxen0"
	cfg, err := config.New(config.UseDefaults, configValues)
	c.Assert(err, gc.IsNil)
	env, err := null.ProviderInstance.Open(cfg)
	c.Assert(err, gc.IsNil)
	insts, err := env.AllInstances()
	c.Assert(err, gc.IsNil)
	c.Assert(insts, gc.HasLen, 1)
	s.PatchValue(null.InstanceHostAddresses, func(host string) ([]instance.Address, error) {
		return []instance.Address{
			instance.NewAddress("192.168.0.1"),
			instance.NewAddress("nickname"),
			instance.NewAddress("boxen0"),
		}, nil
	})
	addrs, err := insts[0].Addresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addrs, gc.HasLen, 3)
	// The last address is marked public, all others are unknown.
	c.Assert(addrs[0].NetworkScope, gc.Equals, instance.NetworkUnknown)
	c.Assert(addrs[1].NetworkScope, gc.Equals, instance.NetworkUnknown)
	c.Assert(addrs[2].NetworkScope, gc.Equals, instance.NetworkPublic)
}
