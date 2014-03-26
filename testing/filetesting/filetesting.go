// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package filetesting

import (
	"io/ioutil"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"
)

// Entry represents a filesystem entity that can be created; and whose
// correctness can be verified.
type Entry interface {

	// Create causes the entry to be created, relative to basePath.
	Create(c *gc.C, basePath string)

	// Check checks that the entry exists, relative to basePath, and matches
	// the entry that would be created by Create.
	Check(c *gc.C, basePath string)
}

var _ Entry = Dir{}
var _ Entry = File{}
var _ Entry = Symlink{}

// join joins a slash-separated path to a filesystem basePath.
func join(basePath, path string) string {
	return filepath.Join(basePath, filepath.FromSlash(path))
}

// Dir is an Entry that allows directories to be created and verified. The
// Path field should use "/" as the path separator.
type Dir struct {
	Path string
	Perm os.FileMode
}

func (d Dir) Create(c *gc.C, basePath string) {
	err := os.MkdirAll(join(basePath, d.Path), d.Perm)
	c.Assert(err, gc.IsNil)
}

func (d Dir) Check(c *gc.C, basePath string) {
	fileInfo, err := os.Lstat(join(basePath, d.Path))
	if !c.Check(err, gc.IsNil) {
		return
	}
	c.Check(fileInfo.Mode()&os.ModePerm, gc.Equals, d.Perm)
	c.Check(fileInfo.Mode()&os.ModeType, gc.Equals, os.ModeDir)
}

// File is an Entry that allows plain files to be created and verified. The
// Path field should use "/" as the path separator.
type File struct {
	Path string
	Data string
	Perm os.FileMode
}

func (f File) Create(c *gc.C, basePath string) {
	err := ioutil.WriteFile(join(basePath, f.Path), []byte(f.Data), f.Perm)
	c.Assert(err, gc.IsNil)
}

func (f File) Check(c *gc.C, basePath string) {
	path := join(basePath, f.Path)
	fileInfo, err := os.Lstat(path)
	if !c.Check(err, gc.IsNil) {
		return
	}
	mode := fileInfo.Mode()
	c.Check(mode&os.ModeType, gc.Equals, os.FileMode(0))
	c.Check(mode&os.ModePerm, gc.Equals, f.Perm)
	data, err := ioutil.ReadFile(path)
	c.Check(err, gc.IsNil)
	c.Check(string(data), gc.Equals, f.Data)
}

// Symlink is an Entry that allows symlinks to be created and verified. The
// Path field should use "/" as the path separator.
type Symlink struct {
	Path string
	Link string
}

func (s Symlink) Create(c *gc.C, basePath string) {
	err := os.Symlink(s.Link, join(basePath, s.Path))
	c.Assert(err, gc.IsNil)
}

func (s Symlink) Check(c *gc.C, basePath string) {
	link, err := os.Readlink(join(basePath, s.Path))
	c.Check(err, gc.IsNil)
	c.Check(link, gc.Equals, s.Link)
}
