// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/configstore"
)

var _ = gc.Suite(&memInterfaceSuite{})

type memInterfaceSuite struct {
	interfaceSuite
}

func (s *memInterfaceSuite) SetUpSuite(c *gc.C) {
	s.interfaceSuite.SetUpSuite(c)
	s.NewStore = func(c *gc.C) configstore.Storage {
		return configstore.NewMem()
	}
}

func (s *memInterfaceSuite) TestMemInfoLocation(c *gc.C) {
	memStore := configstore.NewMem()
	memInfo := memStore.CreateInfo("foo")
	c.Assert(memInfo.Location(), gc.Equals, "memory")
}
