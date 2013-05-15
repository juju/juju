// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
)

type EmptyStorageSuite struct{}

var _ = Suite(&EmptyStorageSuite{})

func (s *EmptyStorageSuite) TestGet(c *C) {
	f, err := environs.EmptyStorage.Get("anything")
	c.Assert(f, IsNil)
	c.Assert(err, ErrorMatches, `file "anything" not found`)
}

func (s *EmptyStorageSuite) TestURL(c *C) {
	url, err := environs.EmptyStorage.URL("anything")
	c.Assert(url, Equals, "")
	c.Assert(err, ErrorMatches, `file "anything" not found`)
}

func (s *EmptyStorageSuite) TestList(c *C) {
	names, err := environs.EmptyStorage.List("anything")
	c.Assert(names, IsNil)
	c.Assert(err, IsNil)
}
