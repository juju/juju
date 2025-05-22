// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filestorage_test

// The filestorage structs are used as stubs in tests.
// The tests defined herein are simple smoke tests for the
// required reader and writer functionality.

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/storage"
)


type filestorageSuite struct {
	dir    string
	reader storage.StorageReader
	writer storage.StorageWriter
}

func TestFilestorageSuite(t *stdtesting.T) { tc.Run(t, &filestorageSuite{}) }
func (s *filestorageSuite) SetUpTest(c *tc.C) {
	s.dir = c.MkDir()
	var err error
	s.reader, err = filestorage.NewFileStorageReader(s.dir)
	c.Assert(err, tc.ErrorIsNil)
	s.writer, err = filestorage.NewFileStorageWriter(s.dir)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *filestorageSuite) createFile(c *tc.C, name string) (fullpath string, data []byte) {
	fullpath = filepath.Join(s.dir, name)
	dir := filepath.Dir(fullpath)
	c.Assert(os.MkdirAll(dir, 0755), tc.IsNil)
	data = []byte{1, 2, 3, 4, 5}
	err := os.WriteFile(fullpath, data, 0644)
	c.Assert(err, tc.ErrorIsNil)
	return fullpath, data
}

func (s *filestorageSuite) TestList(c *tc.C) {
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
		c.Assert(err, tc.ErrorIsNil)
		i := len(files)
		j := len(test.expected)
		c.Assert(i, tc.Equals, j)
		for i := range files {
			c.Assert(files[i], tc.SamePath, test.expected[i])
		}
	}
}

func (s *filestorageSuite) TestListHidesTempDir(c *tc.C) {
	err := s.writer.Put("test-write", bytes.NewReader(nil), 0)
	c.Assert(err, tc.ErrorIsNil)
	files, err := storage.List(s.reader, "")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(files, tc.DeepEquals, []string{"test-write"})
	files, err = storage.List(s.reader, "no-such-directory")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(files, tc.DeepEquals, []string(nil))
	// We also pretend the .tmp directory doesn't exist. If you call a
	// directory that doesn't exist, we just return an empty list of
	// strings, so we force the same behavior for '.tmp'
	// we poke in a file so it would have something to return
	s.createFile(c, ".tmp/test-file")
	files, err = storage.List(s.reader, ".tmp")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(files, tc.DeepEquals, []string(nil))
	// For consistency, we refuse all other possibilities as well
	s.createFile(c, ".tmp/foo/bar")
	files, err = storage.List(s.reader, ".tmp/foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(files, tc.DeepEquals, []string(nil))
	s.createFile(c, ".tmpother/foo")
	files, err = storage.List(s.reader, ".tmpother")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(files, tc.DeepEquals, []string(nil))
}

func (s *filestorageSuite) TestURL(c *tc.C) {
	expectedpath, _ := s.createFile(c, "test-file")
	_, file := filepath.Split(expectedpath)
	url, err := s.reader.URL(file)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(url, tc.Equals, utils.MakeFileURL(expectedpath))
}

func (s *filestorageSuite) TestGet(c *tc.C) {
	expectedpath, data := s.createFile(c, "test-file")
	_, file := filepath.Split(expectedpath)
	rc, err := storage.Get(s.reader, file)
	c.Assert(err, tc.ErrorIsNil)
	defer rc.Close()
	c.Assert(err, tc.ErrorIsNil)
	b, err := io.ReadAll(rc)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(b, tc.DeepEquals, data)

	// Get on a non-existent path returns errors.NotFound
	_, err = s.reader.Get("nowhere")
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	// Get on a directory returns errors.NotFound
	s.createFile(c, "dir/file")
	_, err = s.reader.Get("dir")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *filestorageSuite) TestGetRefusesTemp(c *tc.C) {
	s.createFile(c, ".tmp/test-file")
	_, err := storage.Get(s.reader, ".tmp/test-file")
	c.Check(err, tc.NotNil)
	c.Check(err, tc.Satisfies, os.IsNotExist)
	s.createFile(c, ".tmp/foo/test-file")
	_, err = storage.Get(s.reader, ".tmp/foo/test-file")
	c.Check(err, tc.NotNil)
	c.Check(err, tc.Satisfies, os.IsNotExist)
}

func (s *filestorageSuite) TestPut(c *tc.C) {
	data := []byte{1, 2, 3, 4, 5}
	err := s.writer.Put("test-write", bytes.NewReader(data), int64(len(data)))
	c.Assert(err, tc.ErrorIsNil)
	b, err := os.ReadFile(filepath.Join(s.dir, "test-write"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(b, tc.DeepEquals, data)
}

func (s *filestorageSuite) TestPutRefusesTmp(c *tc.C) {
	data := []byte{1, 2, 3, 4, 5}
	err := s.writer.Put(".tmp/test-write", bytes.NewReader(data), int64(len(data)))
	c.Assert(err, tc.NotNil)
	c.Check(err, tc.Satisfies, os.IsPermission)
	c.Check(*err.(*os.PathError), tc.Equals, os.PathError{
		Op:   "Put",
		Path: ".tmp/test-write",
		Err:  os.ErrPermission,
	})
	_, err = os.ReadFile(filepath.Join(s.dir, ".tmp", "test-write"))
	c.Assert(err, tc.Satisfies, os.IsNotExist)
}

func (s *filestorageSuite) TestRemove(c *tc.C) {
	expectedpath, _ := s.createFile(c, "test-file")
	_, file := filepath.Split(expectedpath)
	err := s.writer.Remove(file)
	c.Assert(err, tc.ErrorIsNil)
	_, err = os.ReadFile(expectedpath)
	c.Assert(err, tc.Not(tc.IsNil))
}

func (s *filestorageSuite) TestRemoveAll(c *tc.C) {
	expectedpath, _ := s.createFile(c, "test-file")
	err := s.writer.RemoveAll()
	c.Assert(err, tc.ErrorIsNil)
	_, err = os.ReadFile(expectedpath)
	c.Assert(err, tc.Not(tc.IsNil))
}

func (s *filestorageSuite) TestPutTmpDir(c *tc.C) {
	// Put should create and clean up the temporary directory
	err := s.writer.Put("test-write", bytes.NewReader(nil), 0)
	c.Assert(err, tc.ErrorIsNil)
	_, err = os.Stat(s.dir + "/.tmp")
	c.Assert(err, tc.Satisfies, os.IsNotExist)

	// To deal with recovering from hard failure, we
	// don't care if the temporary directory already exists. It
	// still removes it, though.
	err = os.Mkdir(s.dir+"/.tmp", 0755)
	c.Assert(err, tc.ErrorIsNil)
	err = s.writer.Put("test-write", bytes.NewReader(nil), 0)
	c.Assert(err, tc.ErrorIsNil)
	_, err = os.Stat(s.dir + "/.tmp")
	c.Assert(err, tc.Satisfies, os.IsNotExist)
}

func (s *filestorageSuite) TestPathRelativeToHome(c *tc.C) {
	homeDir := utils.Home()
	tempDir, err := os.MkdirTemp(homeDir, "")
	c.Assert(err, tc.ErrorIsNil)
	defer os.RemoveAll(tempDir)
	dirName := strings.Replace(tempDir, homeDir, "", -1)
	reader, err := filestorage.NewFileStorageReader(filepath.Join(utils.Home(), dirName))
	c.Assert(err, tc.ErrorIsNil)
	url, err := reader.URL("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(url, tc.Equals, utils.MakeFileURL(filepath.Join(homeDir, dirName)))
}

func (s *filestorageSuite) TestRelativePath(c *tc.C) {
	dir := c.MkDir()
	err := os.MkdirAll(filepath.Join(dir, "a", "b", "c"), os.ModePerm)
	c.Assert(err, tc.ErrorIsNil)
	cwd, err := os.Getwd()
	c.Assert(err, tc.ErrorIsNil)
	err = os.Chdir(filepath.Join(dir, "a", "b", "c"))
	c.Assert(err, tc.ErrorIsNil)
	defer os.Chdir(cwd)
	reader, err := filestorage.NewFileStorageReader("../..")
	c.Assert(err, tc.ErrorIsNil)
	url, err := reader.URL("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(url, tc.Equals, utils.MakeFileURL(dir)+"/a")
}
