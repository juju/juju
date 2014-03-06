// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/configstore"
)

var _ = gc.Suite(&memInterfaceSuite{})

type memInterfaceSuite struct {
	interfaceSuite
}

func (s *memInterfaceSuite) SetUpSuite(c *gc.C) {
	s.NewStore = func(c *gc.C) configstore.Storage {
		return configstore.NewMem()
	}
}

func (s *memInterfaceSuite) TestMemInfoLocation(c *gc.C) {
	memStore := configstore.NewMem()
	memInfo, _ := memStore.CreateInfo("foo")
	c.Assert(memInfo.Location(), gc.Equals, "memory")
}
