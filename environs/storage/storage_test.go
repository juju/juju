// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&datasourceSuite{})

type datasourceSuite struct {
	testing.FakeJujuHomeSuite
	stor    storage.Storage
	baseURL string
}

const existingEnv = `
environments:
    test:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`

func (s *datasourceSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	testing.WriteEnvironments(c, existingEnv)
	environ, err := environs.PrepareFromName("test", testing.Context(c), configstore.NewMem())
	c.Assert(err, gc.IsNil)
	s.stor = environ.Storage()
	s.baseURL, err = s.stor.URL("")
	c.Assert(err, gc.IsNil)
}

func (s *datasourceSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.FakeJujuHomeSuite.TearDownTest(c)
}

func (s *datasourceSuite) TestFetch(c *gc.C) {
	sampleData := "hello world"
	s.stor.Put("foo/bar/data.txt", bytes.NewReader([]byte(sampleData)), int64(len(sampleData)))
	ds := storage.NewStorageSimpleStreamsDataSource("test datasource", s.stor, "")
	rc, url, err := ds.Fetch("foo/bar/data.txt")
	c.Assert(err, gc.IsNil)
	defer rc.Close()
	c.Assert(url, gc.Equals, s.baseURL+"foo/bar/data.txt")
	data, err := ioutil.ReadAll(rc)
	c.Assert(data, gc.DeepEquals, []byte(sampleData))
}

func (s *datasourceSuite) TestFetchWithBasePath(c *gc.C) {
	sampleData := "hello world"
	s.stor.Put("base/foo/bar/data.txt", bytes.NewReader([]byte(sampleData)), int64(len(sampleData)))
	ds := storage.NewStorageSimpleStreamsDataSource("test datasource", s.stor, "base")
	rc, url, err := ds.Fetch("foo/bar/data.txt")
	c.Assert(err, gc.IsNil)
	defer rc.Close()
	c.Assert(url, gc.Equals, s.baseURL+"base/foo/bar/data.txt")
	data, err := ioutil.ReadAll(rc)
	c.Assert(data, gc.DeepEquals, []byte(sampleData))
}

func (s *datasourceSuite) TestFetchWithRetry(c *gc.C) {
	stor := &fakeStorage{shouldRetry: true}
	ds := storage.NewStorageSimpleStreamsDataSource("test datasource", stor, "base")
	ds.SetAllowRetry(true)
	_, _, err := ds.Fetch("foo/bar/data.txt")
	c.Assert(err, gc.ErrorMatches, "an error")
	c.Assert(stor.getName, gc.Equals, "base/foo/bar/data.txt")
	c.Assert(stor.invokeCount, gc.Equals, 10)
}

func (s *datasourceSuite) TestFetchWithNoRetry(c *gc.C) {
	// NB shouldRetry below is true indicating the fake storage is capable of
	// retrying, not that it will retry.
	stor := &fakeStorage{shouldRetry: true}
	ds := storage.NewStorageSimpleStreamsDataSource("test datasource", stor, "base")
	_, _, err := ds.Fetch("foo/bar/data.txt")
	c.Assert(err, gc.ErrorMatches, "an error")
	c.Assert(stor.getName, gc.Equals, "base/foo/bar/data.txt")
	c.Assert(stor.invokeCount, gc.Equals, 1)
}

func (s *datasourceSuite) TestURL(c *gc.C) {
	sampleData := "hello world"
	s.stor.Put("bar/data.txt", bytes.NewReader([]byte(sampleData)), int64(len(sampleData)))
	ds := storage.NewStorageSimpleStreamsDataSource("test datasource", s.stor, "")
	url, err := ds.URL("bar")
	c.Assert(err, gc.IsNil)
	expectedURL, _ := s.stor.URL("bar")
	c.Assert(url, gc.Equals, expectedURL)
}

func (s *datasourceSuite) TestURLWithBasePath(c *gc.C) {
	sampleData := "hello world"
	s.stor.Put("base/bar/data.txt", bytes.NewReader([]byte(sampleData)), int64(len(sampleData)))
	ds := storage.NewStorageSimpleStreamsDataSource("test datasource", s.stor, "base")
	url, err := ds.URL("bar")
	c.Assert(err, gc.IsNil)
	expectedURL, _ := s.stor.URL("base/bar")
	c.Assert(url, gc.Equals, expectedURL)
}

var _ = gc.Suite(&storageSuite{})

type storageSuite struct{}

type fakeStorage struct {
	getName     string
	listPrefix  string
	invokeCount int
	shouldRetry bool
}

func (s *fakeStorage) Get(name string) (io.ReadCloser, error) {
	s.getName = name
	s.invokeCount++
	return nil, fmt.Errorf("an error")
}

func (s *fakeStorage) List(prefix string) ([]string, error) {
	s.listPrefix = prefix
	s.invokeCount++
	return nil, fmt.Errorf("an error")
}

func (s *fakeStorage) URL(name string) (string, error) {
	return "", nil
}

func (s *fakeStorage) DefaultConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{Min: 10}
}

func (s *fakeStorage) ShouldRetry(error) bool {
	return s.shouldRetry
}

func (s *storageSuite) TestGetWithRetry(c *gc.C) {
	stor := &fakeStorage{shouldRetry: true}
	attempt := utils.AttemptStrategy{Min: 5}
	storage.GetWithRetry(stor, "foo", attempt)
	c.Assert(stor.getName, gc.Equals, "foo")
	c.Assert(stor.invokeCount, gc.Equals, 5)
}

func (s *storageSuite) TestGet(c *gc.C) {
	stor := &fakeStorage{shouldRetry: true}
	storage.Get(stor, "foo")
	c.Assert(stor.getName, gc.Equals, "foo")
	c.Assert(stor.invokeCount, gc.Equals, 10)
}

func (s *storageSuite) TestGetNoRetryAllowed(c *gc.C) {
	stor := &fakeStorage{}
	storage.Get(stor, "foo")
	c.Assert(stor.getName, gc.Equals, "foo")
	c.Assert(stor.invokeCount, gc.Equals, 1)
}

func (s *storageSuite) TestListWithRetry(c *gc.C) {
	stor := &fakeStorage{shouldRetry: true}
	attempt := utils.AttemptStrategy{Min: 5}
	storage.ListWithRetry(stor, "foo", attempt)
	c.Assert(stor.listPrefix, gc.Equals, "foo")
	c.Assert(stor.invokeCount, gc.Equals, 5)
}

func (s *storageSuite) TestList(c *gc.C) {
	stor := &fakeStorage{shouldRetry: true}
	storage.List(stor, "foo")
	c.Assert(stor.listPrefix, gc.Equals, "foo")
	c.Assert(stor.invokeCount, gc.Equals, 10)
}

func (s *storageSuite) TestListNoRetryAllowed(c *gc.C) {
	stor := &fakeStorage{}
	storage.List(stor, "foo")
	c.Assert(stor.listPrefix, gc.Equals, "foo")
	c.Assert(stor.invokeCount, gc.Equals, 1)
}
