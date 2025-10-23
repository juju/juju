// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package filetesting

import (
	"os"

	"github.com/juju/tc"
)

// C provides Asset and Check for filetesting test helpers.
type C interface {
	Assert(obtained any, checker tc.Checker, args ...any)
	Check(obtained any, checker tc.Checker, args ...any) bool
}

// Entry represents a filesystem entity that can be created; and whose
// correctness can be verified.
type Entry interface {

	// GetPath returns the slash-separated relative path that this
	// entry represents.
	GetPath() string

	// Create causes the entry to be created, relative to basePath. It returns
	// a copy of the receiver.
	Create(c C, basePath string) Entry

	// Check checks that the entry exists, relative to basePath, and matches
	// the entry that would be created by Create. It returns a copy of the
	// receiver.
	Check(c C, basePath string) Entry
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
	panic("unimplemented")
}

// Create creates every entry relative to basePath and returns a copy of itself.
func (e Entries) Create(c C, basePath string) Entries {
	panic("unimplemented")
}

// Check checks every entry relative to basePath and returns a copy of itself.
func (e Entries) Check(c C, basePath string) Entries {
	panic("unimplemented")
}

// AsRemoveds returns a slice of Removed entries whose paths correspond to
// those in e.
func (e Entries) AsRemoveds() Entries {
	panic("unimplemented")
}

// Dir is an Entry that allows directories to be created and verified. The
// Path field should use "/" as the path separator.
type Dir struct {
	Path string
	Perm os.FileMode
}

func (d Dir) GetPath() string {
	panic("unimplemented")
}

func (d Dir) Create(c C, basePath string) Entry {
	panic("unimplemented")
}

func (d Dir) Check(c C, basePath string) Entry {
	panic("unimplemented")
}

// File is an Entry that allows plain files to be created and verified. The
// Path field should use "/" as the path separator.
type File struct {
	Path string
	Data string
	Perm os.FileMode
}

func (f File) GetPath() string {
	panic("unimplemented")
}

func (f File) Create(c C, basePath string) Entry {
	panic("unimplemented")
}

func (f File) Check(c C, basePath string) Entry {
	panic("unimplemented")
}

// Symlink is an Entry that allows symlinks to be created and verified. The
// Path field should use "/" as the path separator.
type Symlink struct {
	Path string
	Link string
}

func (s Symlink) GetPath() string {
	panic("unimplemented")
}

func (s Symlink) Create(c C, basePath string) Entry {
	panic("unimplemented")
}

func (s Symlink) Check(c C, basePath string) Entry {
	panic("unimplemented")
}

// Removed is an Entry that indicates the absence of any entry. The Path
// field should use "/" as the path separator.
type Removed struct {
	Path string
}

func (r Removed) GetPath() string {
	panic("unimplemented")
}

func (r Removed) Create(c C, basePath string) Entry {
	panic("unimplemented")
}

func (r Removed) Check(c C, basePath string) Entry {
	panic("unimplemented")
}
