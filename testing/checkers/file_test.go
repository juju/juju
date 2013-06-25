// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package checkers_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	. "launchpad.net/gocheck"
	. "launchpad.net/juju-core/testing/checkers"
)

type FileSuite struct{}

var _ = Suite(&FileSuite{})

func (s *FileSuite) TestIsNonEmptyFile(c *C) {
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, IsNil)
	fmt.Fprintf(file, "something")
	file.Close()

	c.Assert(file.Name(), IsNonEmptyFile)
}

func (s *FileSuite) TestIsNonEmptyFileWithEmptyFile(c *C) {
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, IsNil)
	file.Close()

	result, message := IsNonEmptyFile.Check([]interface{}{file.Name()}, nil)
	c.Assert(result, IsFalse)
	c.Assert(message, Equals, file.Name()+" is empty")
}

func (s *FileSuite) TestIsNonEmptyFileWithMissingFile(c *C) {
	name := filepath.Join(c.MkDir(), "missing")

	result, message := IsNonEmptyFile.Check([]interface{}{name}, nil)
	c.Assert(result, IsFalse)
	c.Assert(message, Equals, name+" does not exist")
}

func (s *FileSuite) TestIsNonEmptyFileWithNumber(c *C) {
	result, message := IsNonEmptyFile.Check([]interface{}{42}, nil)
	c.Assert(result, IsFalse)
	c.Assert(message, Equals, "obtained value is not a string and has no .String(), int:42")
}

func (s *FileSuite) TestIsDirectory(c *C) {
	dir := c.MkDir()
	c.Assert(dir, IsDirectory)
}

func (s *FileSuite) TestIsDirectoryMissing(c *C) {
	absentDir := filepath.Join(c.MkDir(), "foo")

	result, message := IsDirectory.Check([]interface{}{absentDir}, nil)
	c.Assert(result, IsFalse)
	c.Assert(message, Equals, absentDir+" does not exist")
}

func (s *FileSuite) TestIsDirectoryWithFile(c *C) {
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, IsNil)
	file.Close()

	result, message := IsDirectory.Check([]interface{}{file.Name()}, nil)
	c.Assert(result, IsFalse)
	c.Assert(message, Equals, file.Name()+" is not a directory")
}

func (s *FileSuite) TestIsDirectoryWithNumber(c *C) {
	result, message := IsDirectory.Check([]interface{}{42}, nil)
	c.Assert(result, IsFalse)
	c.Assert(message, Equals, "obtained value is not a string and has no .String(), int:42")
}

func (s *FileSuite) TestDoesNotExist(c *C) {
	absentDir := filepath.Join(c.MkDir(), "foo")
	c.Assert(absentDir, DoesNotExist)
}

func (s *FileSuite) TestDoesNotExistWithPath(c *C) {
	dir := c.MkDir()
	result, message := DoesNotExist.Check([]interface{}{dir}, nil)
	c.Assert(result, IsFalse)
	c.Assert(message, Equals, dir+" exists")
}

func (s *FileSuite) TestDoesNotExistWithNumber(c *C) {
	result, message := DoesNotExist.Check([]interface{}{42}, nil)
	c.Assert(result, IsFalse)
	c.Assert(message, Equals, "obtained value is not a string and has no .String(), int:42")
}
