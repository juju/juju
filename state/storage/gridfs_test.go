// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"crypto/md5"
	"encoding/hex"
	"io/ioutil"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state/storage"
	"launchpad.net/juju-core/testing"
)

var _ = gc.Suite(&gridfsSuite{})

type gridfsSuite struct {
	testing.BaseSuite
	testing.MgoSuite
	stor storage.ResourceStorage
}

func (s *gridfsSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *gridfsSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *gridfsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.stor = storage.NewGridFS("test", s.Session)
}

func (s *gridfsSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func assertPut(c *gc.C, stor storage.ResourceStorage, path, data string) {
	r := strings.NewReader(data)
	checksum, err := stor.Put(path, r, int64(len(data)))
	c.Assert(err, gc.IsNil)
	md5Hash := md5.New()
	_, err = md5Hash.Write([]byte(data))
	c.Assert(err, gc.IsNil)
	c.Assert(checksum, gc.Equals, hex.EncodeToString(md5Hash.Sum(nil)))
	assertGet(c, stor, path, data)
}

func (s *gridfsSuite) TestPut(c *gc.C) {
	assertPut(c, s.stor, "/path/to/file", "hello world")
}

func (s *gridfsSuite) TestPutSameFileOverwrites(c *gc.C) {
	assertPut(c, s.stor, "/path/to/file", "hello world")
	assertPut(c, s.stor, "/path/to/file", "hello again")
}

func assertGet(c *gc.C, stor storage.ResourceStorage, path, expected string) {
	r, err := stor.Get(path)
	c.Assert(err, gc.IsNil)
	defer r.Close()
	data, err := ioutil.ReadAll(r)
	c.Assert(err, gc.IsNil)
	c.Assert(data, gc.DeepEquals, []byte(expected))
}

func (s *gridfsSuite) TestGetNonExistent(c *gc.C) {
	_, err := s.stor.Get("missing")
	c.Assert(err, gc.ErrorMatches, `failed to open GridFS file "missing": not found`)
}

func (s *gridfsSuite) TestGet(c *gc.C) {
	data := "hello world"
	r := strings.NewReader(data)
	_, err := s.stor.Put("/path/to/file", r, int64(len(data)))
	c.Assert(err, gc.IsNil)
	assertGet(c, s.stor, "/path/to/file", data)
}

func (s *gridfsSuite) TestRemove(c *gc.C) {
	path := "/path/to/file"
	assertPut(c, s.stor, path, "hello world")
	err := s.stor.Remove(path)
	c.Assert(err, gc.IsNil)
	_, err = s.stor.Get(path)
	c.Assert(err, gc.ErrorMatches, `failed to open GridFS file "/path/to/file": not found`)
}

func (s *gridfsSuite) TestRemoveNonExistent(c *gc.C) {
	err := s.stor.Remove("/path/to/file")
	c.Assert(err, gc.IsNil)
}

func (s *gridfsSuite) TestNamespaceSeparation(c *gc.C) {
	anotherStor := storage.NewGridFS("another", s.Session)
	path := "/path/to/file"
	assertPut(c, anotherStor, path, "hello world")
	_, err := s.stor.Get(path)
	c.Assert(err, gc.ErrorMatches, `failed to open GridFS file "/path/to/file": not found`)
}

func (s *gridfsSuite) TestNamespaceSeparationRemove(c *gc.C) {
	anotherStor := storage.NewGridFS("another", s.Session)
	path := "/path/to/file"
	assertPut(c, s.stor, path, "hello world")
	assertPut(c, anotherStor, path, "hello again")
	err := s.stor.Remove(path)
	c.Assert(err, gc.IsNil)
	assertGet(c, anotherStor, "/path/to/file", "hello again")
}
