// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package filetesting

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/juju/tc"
)

// Entry represents a filesystem entity that can be created; and whose
// correctness can be verified.
type Entry interface {

	// GetPath returns the slash-separated relative path that this
	// entry represents.
	GetPath() string

	// Create causes the entry to be created, relative to basePath. It returns
	// a copy of the receiver.
	Create(c *tc.C, basePath string) Entry

	// Check checks that the entry exists, relative to basePath, and matches
	// the entry that would be created by Create. It returns a copy of the
	// receiver.
	Check(c *tc.C, basePath string) Entry
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
func (e Entries) Create(c *tc.C, basePath string) Entries {
	result := make([]Entry, len(e))
	for i, entry := range e {
		result[i] = entry.Create(c, basePath)
	}
	return result
}

// Check checks every entry relative to basePath and returns a copy of itself.
func (e Entries) Check(c *tc.C, basePath string) Entries {
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

func (d Dir) Create(c *tc.C, basePath string) Entry {
	path := join(basePath, d.Path)
	err := os.MkdirAll(path, d.Perm)
	c.Assert(err, tc.IsNil)
	err = os.Chmod(path, d.Perm)
	c.Assert(err, tc.IsNil)
	return d
}

func (d Dir) Check(c *tc.C, basePath string) Entry {
	path := join(basePath, d.Path)
	fileInfo, err := os.Lstat(path)
	comment := tc.Commentf("dir %q", path)
	if !c.Check(err, tc.IsNil, comment) {
		return d
	}
	// Skip until we implement proper permissions checking
	if runtime.GOOS != "windows" {
		c.Check(fileInfo.Mode()&os.ModePerm, tc.Equals, d.Perm, comment)
	}
	c.Check(fileInfo.Mode()&os.ModeType, tc.Equals, os.ModeDir, comment)
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

func (f File) Create(c *tc.C, basePath string) Entry {
	path := join(basePath, f.Path)
	err := ioutil.WriteFile(path, []byte(f.Data), f.Perm)
	c.Assert(err, tc.IsNil)
	err = os.Chmod(path, f.Perm)
	c.Assert(err, tc.IsNil)
	return f
}

func (f File) Check(c *tc.C, basePath string) Entry {
	path := join(basePath, f.Path)
	fileInfo, err := os.Lstat(path)
	comment := tc.Commentf("file %q", path)
	if !c.Check(err, tc.IsNil, comment) {
		return f
	}
	// Skip until we implement proper permissions checking
	if runtime.GOOS != "windows" {
		mode := fileInfo.Mode()
		c.Check(mode&os.ModeType, tc.Equals, os.FileMode(0), comment)
		c.Check(mode&os.ModePerm, tc.Equals, f.Perm, comment)
	}
	data, err := ioutil.ReadFile(path)
	c.Check(err, tc.IsNil, comment)
	c.Check(string(data), tc.Equals, f.Data, comment)
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

func (s Symlink) Create(c *tc.C, basePath string) Entry {
	err := os.Symlink(s.Link, join(basePath, s.Path))
	c.Assert(err, tc.IsNil)
	return s
}

func (s Symlink) Check(c *tc.C, basePath string) Entry {
	path := join(basePath, s.Path)
	comment := tc.Commentf("symlink %q", path)
	link, err := os.Readlink(path)
	c.Check(err, tc.IsNil, comment)
	c.Check(link, tc.Equals, s.Link, comment)
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

func (r Removed) Create(c *tc.C, basePath string) Entry {
	err := os.RemoveAll(join(basePath, r.Path))
	c.Assert(err, tc.IsNil)
	return r
}

func (r Removed) Check(c *tc.C, basePath string) Entry {
	path := join(basePath, r.Path)
	_, err := os.Lstat(path)
	// isNotExist allows us to handle the following case:
	//  File{"foo", ...}.Create(...)
	//  Removed{"foo/bar"}.Check(...)
	// ...where os.IsNotExist would not work.
	c.Check(err, tc.Satisfies, isNotExist, tc.Commentf("removed %q", path))
	return r
}
