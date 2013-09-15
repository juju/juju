// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"bytes"
	"io/ioutil"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/testing"
)

var _ = gc.Suite(&datasourceSuite{})

type datasourceSuite struct {
	home    *testing.FakeHome
	storage environs.Storage
}

func (s *datasourceSuite) SetUpTest(c *gc.C) {
	s.home = testing.MakeFakeHome(c, existingEnv, "existing")
	environ, err := environs.PrepareFromName("test")
	c.Assert(err, gc.IsNil)
	s.storage = environ.Storage()
}

func (s *datasourceSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.home.Restore()
}

func (s *datasourceSuite) TestFetch(c *gc.C) {
	sampleData := "hello world"
	s.storage.Put("foo/bar/data.txt", bytes.NewReader([]byte(sampleData)), int64(len(sampleData)))
	ds := environs.NewStorageSimpleStreamsDataSource(s.storage, "")
	rc, url, err := ds.Fetch("foo/bar/data.txt")
	c.Assert(err, gc.IsNil)
	defer rc.Close()
	c.Assert(url, gc.Equals, "foo/bar/data.txt")
	data, err := ioutil.ReadAll(rc)
	c.Assert(data, gc.DeepEquals, []byte(sampleData))
}

func (s *datasourceSuite) TestFetchWithBasePath(c *gc.C) {
	sampleData := "hello world"
	s.storage.Put("base/foo/bar/data.txt", bytes.NewReader([]byte(sampleData)), int64(len(sampleData)))
	ds := environs.NewStorageSimpleStreamsDataSource(s.storage, "base")
	rc, url, err := ds.Fetch("foo/bar/data.txt")
	c.Assert(err, gc.IsNil)
	defer rc.Close()
	c.Assert(url, gc.Equals, "base/foo/bar/data.txt")
	data, err := ioutil.ReadAll(rc)
	c.Assert(data, gc.DeepEquals, []byte(sampleData))
}

func (s *datasourceSuite) TestURL(c *gc.C) {
	ds := environs.NewStorageSimpleStreamsDataSource(s.storage, "")
	url, err := ds.URL("bar")
	c.Assert(err, gc.IsNil)
	expectedURL, _ := s.storage.URL("bar")
	c.Assert(url, gc.Equals, expectedURL)
}

func (s *datasourceSuite) TestURLWithBasePath(c *gc.C) {
	ds := environs.NewStorageSimpleStreamsDataSource(s.storage, "base")
	url, err := ds.URL("bar")
	c.Assert(err, gc.IsNil)
	expectedURL, _ := s.storage.URL("base/bar")
	c.Assert(url, gc.Equals, expectedURL)
}
