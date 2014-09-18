// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"io/ioutil"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state"
)

type StorageSuite struct {
	ConnSuite
}

var _ = gc.Suite(&StorageSuite{})

func (s *StorageSuite) TestStorageGet(c *gc.C) {
	stor := s.State.Storage()

	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	ms := state.GetManagedStorage(s.State, env.UUID(), s.State.MongoSession())
	err = ms.PutForEnvironment(env.UUID(), "abc", strings.NewReader("abc"), 3)
	c.Assert(err, gc.IsNil)

	r, length, err := stor.Get("abc")
	c.Assert(err, gc.IsNil)
	defer r.Close()
	c.Assert(length, gc.Equals, int64(3))

	data, err := ioutil.ReadAll(r)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "abc")
}

func (s *StorageSuite) TestStoragePut(c *gc.C) {
	err := s.State.Storage().Put("path", strings.NewReader("abcdef"), 3)
	c.Assert(err, gc.IsNil)

	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	ms := state.GetManagedStorage(s.State, env.UUID(), s.State.MongoSession())
	r, length, err := ms.GetForEnvironment(env.UUID(), "path")
	c.Assert(err, gc.IsNil)
	defer r.Close()

	c.Assert(length, gc.Equals, int64(3))
	data, err := ioutil.ReadAll(r)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "abc")
}

func (s *StorageSuite) TestStorageRemove(c *gc.C) {
	err := s.State.Storage().Put("path", strings.NewReader("abcdef"), 3)
	c.Assert(err, gc.IsNil)

	err = s.State.Storage().Remove("path")
	c.Assert(err, gc.IsNil)

	_, _, err = s.State.Storage().Get("path")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	err = s.State.Storage().Remove("path")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
