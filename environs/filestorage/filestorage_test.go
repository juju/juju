// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filestorage_test

// The filestorage structs are used as stubs in tests.
// The tests defined herein are simple smoke tests for the
// required reader and writer functionality.

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/storage"
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
	c.Assert(err, jc.ErrorIsNil)
	s.writer, err = filestorage.NewFileStorageWriter(s.dir)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *filestorageSuite) createFile(c *gc.C, name string) (fullpath string, data []byte) {
	fullpath = filepath.Join(s.dir, name)
	dir := filepath.Dir(fullpath)
	c.Assert(os.MkdirAll(dir, 0755), gc.IsNil)
	data = []byte{1, 2, 3, 4, 5}
	err := ioutil.WriteFile(fullpath, data, 0644)
	c.Assert(err, jc.ErrorIsNil)
	return fullpath, data
}

func (s *filestorageSuite) TestList(c *gc.C) {
	names := []string{
		"a/b/c",
		"a/bb",
		"a/c",
		"aa",
		"b/c/d",
	}
	for _, name := range names {
		s.createFile(c, name)
	}
	type test struct {
		prefix   string
		expected []string
	}
	for i, test := range []test{
		{"a", []string{"a/b/c", "a/bb", "a/c", "aa"}},
		{"a/b", []string{"a/b/c", "a/bb"}},
		{"a/b/c", []string{"a/b/c"}},
		{"", names},
	} {
		c.Logf("test %d: prefix=%q", i, test.prefix)
		files, err := storage.List(s.reader, test.prefix)
		c.Assert(err, jc.ErrorIsNil)
		i := len(files)
		j := len(test.expected)
		c.Assert(i, gc.Equals, j)
		for i := range files {
			c.Assert(files[i], jc.SamePath, test.expected[i])
		}
	}
}

func (s *filestorageSuite) TestListHidesTempDir(c *gc.C) {
	err := s.writer.Put("test-write", bytes.NewReader(nil), 0)
	c.Assert(err, jc.ErrorIsNil)
	files, err := storage.List(s.reader, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(files, gc.DeepEquals, []string{"test-write"})
	files, err = storage.List(s.reader, "no-such-directory")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(files, gc.DeepEquals, []string(nil))
	// We also pretend the .tmp directory doesn't exist. If you call a
	// directory that doesn't exist, we just return an empty list of
	// strings, so we force the same behavior for '.tmp'
	// we poke in a file so it would have something to return
	s.createFile(c, ".tmp/test-file")
	files, err = storage.List(s.reader, ".tmp")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(files, gc.DeepEquals, []string(nil))
	// For consistency, we refuse all other possibilities as well
	s.createFile(c, ".tmp/foo/bar")
	files, err = storage.List(s.reader, ".tmp/foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(files, gc.DeepEquals, []string(nil))
	s.createFile(c, ".tmpother/foo")
	files, err = storage.List(s.reader, ".tmpother")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(files, gc.DeepEquals, []string(nil))
}

func (s *filestorageSuite) TestURL(c *gc.C) {
	expectedpath, _ := s.createFile(c, "test-file")
	_, file := filepath.Split(expectedpath)
	url, err := s.reader.URL(file)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url, gc.Equals, utils.MakeFileURL(expectedpath))
}

func (s *filestorageSuite) TestGet(c *gc.C) {
	expectedpath, data := s.createFile(c, "test-file")
	_, file := filepath.Split(expectedpath)
	rc, err := storage.Get(s.reader, file)
	c.Assert(err, jc.ErrorIsNil)
	defer rc.Close()
	c.Assert(err, jc.ErrorIsNil)
	b, err := ioutil.ReadAll(rc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(b, gc.DeepEquals, data)

	// Get on a non-existant path returns errors.NotFound
	_, err = s.reader.Get("nowhere")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Get on a directory returns errors.NotFound
	s.createFile(c, "dir/file")
	_, err = s.reader.Get("dir")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *filestorageSuite) TestGetRefusesTemp(c *gc.C) {
	s.createFile(c, ".tmp/test-file")
	_, err := storage.Get(s.reader, ".tmp/test-file")
	c.Check(err, gc.NotNil)
	c.Check(err, jc.Satisfies, os.IsNotExist)
	s.createFile(c, ".tmp/foo/test-file")
	_, err = storage.Get(s.reader, ".tmp/foo/test-file")
	c.Check(err, gc.NotNil)
	c.Check(err, jc.Satisfies, os.IsNotExist)
}

func (s *filestorageSuite) TestPut(c *gc.C) {
	data := []byte{1, 2, 3, 4, 5}
	err := s.writer.Put("test-write", bytes.NewReader(data), int64(len(data)))
	c.Assert(err, jc.ErrorIsNil)
	b, err := ioutil.ReadFile(filepath.Join(s.dir, "test-write"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(b, gc.DeepEquals, data)
}

func (s *filestorageSuite) TestPutRefusesTmp(c *gc.C) {
	data := []byte{1, 2, 3, 4, 5}
	err := s.writer.Put(".tmp/test-write", bytes.NewReader(data), int64(len(data)))
	c.Assert(err, gc.NotNil)
	c.Check(err, jc.Satisfies, os.IsPermission)
	c.Check(*err.(*os.PathError), gc.Equals, os.PathError{
		Op:   "Put",
		Path: ".tmp/test-write",
		Err:  os.ErrPermission,
	})
	_, err = ioutil.ReadFile(filepath.Join(s.dir, ".tmp", "test-write"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *filestorageSuite) TestRemove(c *gc.C) {
	expectedpath, _ := s.createFile(c, "test-file")
	_, file := filepath.Split(expectedpath)
	err := s.writer.Remove(file)
	c.Assert(err, jc.ErrorIsNil)
	_, err = ioutil.ReadFile(expectedpath)
	c.Assert(err, gc.Not(gc.IsNil))
}

func (s *filestorageSuite) TestRemoveAll(c *gc.C) {
	expectedpath, _ := s.createFile(c, "test-file")
	err := s.writer.RemoveAll()
	c.Assert(err, jc.ErrorIsNil)
	_, err = ioutil.ReadFile(expectedpath)
	c.Assert(err, gc.Not(gc.IsNil))
}

func (s *filestorageSuite) TestPutTmpDir(c *gc.C) {
	// Put should create and clean up the temporary directory
	err := s.writer.Put("test-write", bytes.NewReader(nil), 0)
	c.Assert(err, jc.ErrorIsNil)
	_, err = os.Stat(s.dir + "/.tmp")
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	// To deal with recovering from hard failure, we
	// don't care if the temporary directory already exists. It
	// still removes it, though.
	err = os.Mkdir(s.dir+"/.tmp", 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = s.writer.Put("test-write", bytes.NewReader(nil), 0)
	c.Assert(err, jc.ErrorIsNil)
	_, err = os.Stat(s.dir + "/.tmp")
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *filestorageSuite) TestPathRelativeToHome(c *gc.C) {
	homeDir := utils.Home()
	tempDir, err := ioutil.TempDir(homeDir, "")
	c.Assert(err, jc.ErrorIsNil)
	defer os.RemoveAll(tempDir)
	dirName := strings.Replace(tempDir, homeDir, "", -1)
	reader, err := filestorage.NewFileStorageReader(filepath.Join(utils.Home(), dirName))
	c.Assert(err, jc.ErrorIsNil)
	url, err := reader.URL("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url, gc.Equals, utils.MakeFileURL(filepath.Join(homeDir, dirName)))
}

func (s *filestorageSuite) TestRelativePath(c *gc.C) {
	dir := c.MkDir()
	err := os.MkdirAll(filepath.Join(dir, "a", "b", "c"), os.ModePerm)
	c.Assert(err, jc.ErrorIsNil)
	cwd, err := os.Getwd()
	c.Assert(err, jc.ErrorIsNil)
	err = os.Chdir(filepath.Join(dir, "a", "b", "c"))
	c.Assert(err, jc.ErrorIsNil)
	defer os.Chdir(cwd)
	reader, err := filestorage.NewFileStorageReader("../..")
	c.Assert(err, jc.ErrorIsNil)
	url, err := reader.URL("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url, gc.Equals, utils.MakeFileURL(dir)+"/a")
}
