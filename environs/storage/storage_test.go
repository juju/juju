// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"bytes"
	"fmt"
	"io"
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/internal/testing"
)

func TestDatasourceSuite(t *stdtesting.T) {
	tc.Run(t, &datasourceSuite{})
}

type datasourceSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	stor    storage.Storage
	baseURL string
}

func (s *datasourceSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	storageDir := c.MkDir()
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, tc.ErrorIsNil)
	s.stor = stor
	s.baseURL, err = s.stor.URL("")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *datasourceSuite) TestFetch(c *tc.C) {
	sampleData := "hello world"
	s.stor.Put("foo/bar/data.txt", bytes.NewReader([]byte(sampleData)), int64(len(sampleData)))
	ds := storage.NewStorageSimpleStreamsDataSource("test datasource", s.stor, "", simplestreams.DEFAULT_CLOUD_DATA, false)
	rc, url, err := ds.Fetch(c.Context(), "foo/bar/data.txt")
	c.Assert(err, tc.ErrorIsNil)
	defer rc.Close()
	c.Assert(url, tc.Equals, s.baseURL+"/foo/bar/data.txt")
	data, err := io.ReadAll(rc)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, []byte(sampleData))
}

func (s *datasourceSuite) TestFetchWithBasePath(c *tc.C) {
	sampleData := "hello world"
	s.stor.Put("base/foo/bar/data.txt", bytes.NewReader([]byte(sampleData)), int64(len(sampleData)))
	ds := storage.NewStorageSimpleStreamsDataSource("test datasource", s.stor, "base", simplestreams.DEFAULT_CLOUD_DATA, false)
	rc, url, err := ds.Fetch(c.Context(), "foo/bar/data.txt")
	c.Assert(err, tc.ErrorIsNil)
	defer rc.Close()
	c.Assert(url, tc.Equals, s.baseURL+"/base/foo/bar/data.txt")
	data, err := io.ReadAll(rc)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, []byte(sampleData))
}

func (s *datasourceSuite) TestFetchWithRetry(c *tc.C) {
	stor := &fakeStorage{shouldRetry: true}
	ds := storage.NewStorageSimpleStreamsDataSource("test datasource", stor, "base", simplestreams.DEFAULT_CLOUD_DATA, false)
	_, _, err := ds.Fetch(c.Context(), "foo/bar/data.txt")
	c.Assert(err, tc.ErrorMatches, "an error")
	c.Assert(stor.getName, tc.Equals, "base/foo/bar/data.txt")
	c.Assert(stor.invokeCount, tc.Equals, 10)
}

func (s *datasourceSuite) TestURL(c *tc.C) {
	sampleData := "hello world"
	s.stor.Put("bar/data.txt", bytes.NewReader([]byte(sampleData)), int64(len(sampleData)))
	ds := storage.NewStorageSimpleStreamsDataSource("test datasource", s.stor, "", simplestreams.DEFAULT_CLOUD_DATA, false)
	url, err := ds.URL("bar")
	c.Assert(err, tc.ErrorIsNil)
	expectedURL, _ := s.stor.URL("bar")
	c.Assert(url, tc.Equals, expectedURL)
}

func (s *datasourceSuite) TestURLWithBasePath(c *tc.C) {
	sampleData := "hello world"
	s.stor.Put("base/bar/data.txt", bytes.NewReader([]byte(sampleData)), int64(len(sampleData)))
	ds := storage.NewStorageSimpleStreamsDataSource("test datasource", s.stor, "base", simplestreams.DEFAULT_CLOUD_DATA, false)
	url, err := ds.URL("bar")
	c.Assert(err, tc.ErrorIsNil)
	expectedURL, _ := s.stor.URL("base/bar")
	c.Assert(url, tc.Equals, expectedURL)
}
func TestStorageSuite(t *stdtesting.T) {
	tc.Run(t, &storageSuite{})
}

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
	// TODO(katco): 2016-08-09: lp:1611427
	return utils.AttemptStrategy{Min: 10}
}

func (s *fakeStorage) ShouldRetry(error) bool {
	return s.shouldRetry
}

func (s *storageSuite) TestGetWithRetry(c *tc.C) {
	stor := &fakeStorage{shouldRetry: true}
	// TODO(katco): 2016-08-09: lp:1611427
	attempt := utils.AttemptStrategy{Min: 5}
	storage.GetWithRetry(stor, "foo", attempt)
	c.Assert(stor.getName, tc.Equals, "foo")
	c.Assert(stor.invokeCount, tc.Equals, 5)
}

func (s *storageSuite) TestGet(c *tc.C) {
	stor := &fakeStorage{shouldRetry: true}
	storage.Get(stor, "foo")
	c.Assert(stor.getName, tc.Equals, "foo")
	c.Assert(stor.invokeCount, tc.Equals, 10)
}

func (s *storageSuite) TestGetNoRetryAllowed(c *tc.C) {
	stor := &fakeStorage{}
	storage.Get(stor, "foo")
	c.Assert(stor.getName, tc.Equals, "foo")
	c.Assert(stor.invokeCount, tc.Equals, 1)
}

func (s *storageSuite) TestListWithRetry(c *tc.C) {
	stor := &fakeStorage{shouldRetry: true}
	// TODO(katco): 2016-08-09: lp:1611427
	attempt := utils.AttemptStrategy{Min: 5}
	storage.ListWithRetry(stor, "foo", attempt)
	c.Assert(stor.listPrefix, tc.Equals, "foo")
	c.Assert(stor.invokeCount, tc.Equals, 5)
}

func (s *storageSuite) TestList(c *tc.C) {
	stor := &fakeStorage{shouldRetry: true}
	storage.List(stor, "foo")
	c.Assert(stor.listPrefix, tc.Equals, "foo")
	c.Assert(stor.invokeCount, tc.Equals, 10)
}

func (s *storageSuite) TestListNoRetryAllowed(c *tc.C) {
	stor := &fakeStorage{}
	storage.List(stor, "foo")
	c.Assert(stor.listPrefix, tc.Equals, "foo")
	c.Assert(stor.invokeCount, tc.Equals, 1)
}
