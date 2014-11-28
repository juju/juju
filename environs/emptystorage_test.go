// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/testing"
)

type EmptyStorageSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&EmptyStorageSuite{})

func (s *EmptyStorageSuite) TestGet(c *gc.C) {
	f, err := storage.Get(environs.EmptyStorage, "anything")
	c.Assert(f, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `file "anything" not found`)
}

func (s *EmptyStorageSuite) TestURL(c *gc.C) {
	url, err := environs.EmptyStorage.URL("anything")
	c.Assert(url, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `file "anything" not found`)
}

func (s *EmptyStorageSuite) TestList(c *gc.C) {
	names, err := storage.List(environs.EmptyStorage, "anything")
	c.Assert(names, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)
}
