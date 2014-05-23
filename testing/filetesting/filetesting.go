// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package filetesting

import (
	"io/ioutil"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"
)

// Entry represents a filesystem entity that can be created; and whose
// correctness can be verified.
type Entry interface {

	// GetPath returns the slash-separated relative path that this
	// entry represents.
	GetPath() string

	// Create causes the entry to be created, relative to basePath. It returns
	// a copy of the receiver.
	Create(c *gc.C, basePath string) Entry

	// Check checks that the entry exists, relative to basePath, and matches
	// the entry that would be created by Create. It returns a copy of the
	// receiver.
	Check(c *gc.C, basePath string) Entry
}

var (
	_ Entry = Dir{}
	_ Entry = File{}
	_ Entry = Symlink{}
	_ Entry = Removed{}
)

// Entries supplies convenience methods on Entry slices.
type Entries []Entry

// Paths returns the slash-separated path of every entry.
func (e Entries) Paths() []string {
	result := make([]string, len(e))
	for i, entry := range e {
		result[i] = entry.GetPath()
	}
	return result
}

// Create creates every entry relative to basePath and returns a copy of itself.
func (e Entries) Create(c *gc.C, basePath string) Entries {
	result := make([]Entry, len(e))
	for i, entry := range e {
		result[i] = entry.Create(c, basePath)
	}
	return result
}

// Check checks every entry relative to basePath and returns a copy of itself.
func (e Entries) Check(c *gc.C, basePath string) Entries {
	result := make([]Entry, len(e))
	for i, entry := range e {
		result[i] = entry.Check(c, basePath)
	}
	return result
}

// AsRemoveds returns a slice of Removed entries whose paths correspond to
// those in e.
func (e Entries) AsRemoveds() Entries {
	result := make([]Entry, len(e))
	for i, entry := range e {
		result[i] = Removed{entry.GetPath()}
	}
	return result
}

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

func (d Dir) GetPath() string {
	return d.Path
}

func (d Dir) Create(c *gc.C, basePath string) Entry {
	path := join(basePath, d.Path)
	err := os.MkdirAll(path, d.Perm)
	c.Assert(err, gc.IsNil)
	err = os.Chmod(path, d.Perm)
	c.Assert(err, gc.IsNil)
	return d
}

func (d Dir) Check(c *gc.C, basePath string) Entry {
	fileInfo, err := os.Lstat(join(basePath, d.Path))
	if !c.Check(err, gc.IsNil) {
		return d
	}
	c.Check(fileInfo.Mode()&os.ModePerm, gc.Equals, d.Perm)
	c.Check(fileInfo.Mode()&os.ModeType, gc.Equals, os.ModeDir)
	return d
}

// File is an Entry that allows plain files to be created and verified. The
// Path field should use "/" as the path separator.
type File struct {
	Path string
	Data string
	Perm os.FileMode
}

func (f File) GetPath() string {
	return f.Path
}

func (f File) Create(c *gc.C, basePath string) Entry {
	err := ioutil.WriteFile(join(basePath, f.Path), []byte(f.Data), f.Perm)
	c.Assert(err, gc.IsNil)
	return f
}

func (f File) Check(c *gc.C, basePath string) Entry {
	path := join(basePath, f.Path)
	fileInfo, err := os.Lstat(path)
	if !c.Check(err, gc.IsNil) {
		return f
	}
	mode := fileInfo.Mode()
	c.Check(mode&os.ModeType, gc.Equals, os.FileMode(0))
	c.Check(mode&os.ModePerm, gc.Equals, f.Perm)
	data, err := ioutil.ReadFile(path)
	c.Check(err, gc.IsNil)
	c.Check(string(data), gc.Equals, f.Data)
	return f
}

// Symlink is an Entry that allows symlinks to be created and verified. The
// Path field should use "/" as the path separator.
type Symlink struct {
	Path string
	Link string
}

func (s Symlink) GetPath() string {
	return s.Path
}

func (s Symlink) Create(c *gc.C, basePath string) Entry {
	err := os.Symlink(s.Link, join(basePath, s.Path))
	c.Assert(err, gc.IsNil)
	return s
}

func (s Symlink) Check(c *gc.C, basePath string) Entry {
	link, err := os.Readlink(join(basePath, s.Path))
	c.Check(err, gc.IsNil)
	c.Check(link, gc.Equals, s.Link)
	return s
}

// Removed is an Entry that indicates the absence of any entry. The Path
// field should use "/" as the path separator.
type Removed struct {
	Path string
}

func (r Removed) GetPath() string {
	return r.Path
}

func (r Removed) Create(c *gc.C, basePath string) Entry {
	err := os.RemoveAll(join(basePath, r.Path))
	c.Assert(err, gc.IsNil)
	return r
}

func (r Removed) Check(c *gc.C, basePath string) Entry {
	_, err := os.Lstat(join(basePath, r.Path))
	// isNotExist allows us to handle the following case:
	//  File{"foo", ...}.Create(...)
	//  Removed{"foo/bar"}.Check(...)
	// ...where os.IsNotExist would not work.
	c.Assert(err, jc.Satisfies, isNotExist)
	return r
}
