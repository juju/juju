// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filestorage_test

// The filestorage structs are used as stubs in tests.
// The tests defined herein are simple smoke tests for the
// required reader and writer functionality.

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/storage"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type filestorageSuite struct {
	dir    string
	reader storage.StorageReader
	writer storage.StorageWriter
}

var _ = gc.Suite(&filestorageSuite{})

func (s *filestorageSuite) SetUpTest(c *gc.C) {
	s.dir = c.MkDir()
	var err error
	s.reader, err = filestorage.NewFileStorageReader(s.dir)
	c.Assert(err, gc.IsNil)
	s.writer, err = filestorage.NewFileStorageWriter(s.dir)
	c.Assert(err, gc.IsNil)
}

func (s *filestorageSuite) createFile(c *gc.C) (fullpath string, data []byte) {
	fullpath = filepath.Join(s.dir, "test-file")
	data = []byte{1, 2, 3, 4, 5}
	err := ioutil.WriteFile(fullpath, data, 0644)
	c.Assert(err, gc.IsNil)
	return fullpath, data
}

func (s *filestorageSuite) TestList(c *gc.C) {
	expectedpath, _ := s.createFile(c)
	files, err := storage.DefaultList(s.reader, "test-")
	c.Assert(err, gc.IsNil)
	_, file := filepath.Split(expectedpath)
	c.Assert(files, gc.DeepEquals, []string{file})
}

func (s *filestorageSuite) TestURL(c *gc.C) {
	expectedpath, _ := s.createFile(c)
	_, file := filepath.Split(expectedpath)
	url, err := s.reader.URL(file)
	c.Assert(err, gc.IsNil)
	c.Assert(url, gc.Equals, "file://"+expectedpath)
}

func (s *filestorageSuite) TestGet(c *gc.C) {
	expectedpath, data := s.createFile(c)
	_, file := filepath.Split(expectedpath)
	rc, err := storage.DefaultGet(s.reader, file)
	c.Assert(err, gc.IsNil)
	defer rc.Close()
	c.Assert(err, gc.IsNil)
	b, err := ioutil.ReadAll(rc)
	c.Assert(err, gc.IsNil)
	c.Assert(b, gc.DeepEquals, data)
}

func (s *filestorageSuite) TestPut(c *gc.C) {
	data := []byte{1, 2, 3, 4, 5}
	err := s.writer.Put("test-write", bytes.NewReader(data), int64(len(data)))
	c.Assert(err, gc.IsNil)
	b, err := ioutil.ReadFile(filepath.Join(s.dir, "test-write"))
	c.Assert(err, gc.IsNil)
	c.Assert(b, gc.DeepEquals, data)
}

func (s *filestorageSuite) TestRemove(c *gc.C) {
	expectedpath, _ := s.createFile(c)
	_, file := filepath.Split(expectedpath)
	err := s.writer.Remove(file)
	c.Assert(err, gc.IsNil)
	_, err = ioutil.ReadFile(expectedpath)
	c.Assert(err, gc.Not(gc.IsNil))
}

func (s *filestorageSuite) TestRemoveAll(c *gc.C) {
	expectedpath, _ := s.createFile(c)
	err := s.writer.RemoveAll()
	c.Assert(err, gc.IsNil)
	_, err = ioutil.ReadFile(expectedpath)
	c.Assert(err, gc.Not(gc.IsNil))
}
