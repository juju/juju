// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"io/ioutil"
	"strings"

	gc "launchpad.net/gocheck"
)

type StorageSuite struct {
	ConnSuite
}

var _ = gc.Suite(&StorageSuite{})

func (s *StorageSuite) TestStorageGet(c *gc.C) {
	stor := s.State.Storage()
	err := stor.Put("abc", strings.NewReader("abc"), 3)
	c.Assert(err, gc.IsNil)
	r, length, err := stor.Get("abc")
	c.Assert(err, gc.IsNil)
	defer r.Close()
	c.Assert(length, gc.Equals, int64(3))

	data, err := ioutil.ReadAll(r)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "abc")
}
