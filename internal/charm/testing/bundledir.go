// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/juju/juju/internal/charm"
)

// BundleDir defines a bundle from a given directory.
type BundleDir struct {
	Path        string
	data        *charm.BundleData
	bundleBytes []byte
	readMe      string

	containsOverlays bool
}

// ReadBundleDir returns a BundleDir representing an expanded
// bundle directory. It does not verify the bundle data.
func ReadBundleDir(path string) (dir *BundleDir, err error) {
	dir = &BundleDir{
		Path: path,
	}
	b, err := os.ReadFile(dir.join("bundle.yaml"))
	if err != nil {
		return nil, err
	}
	dir.bundleBytes = b
	dir.data, dir.containsOverlays, err = charm.ReadBaseFromMultidocBundle(b)
	if err != nil {
		return nil, err
	}
	readMe, err := os.ReadFile(dir.join("README.md"))
	if err != nil {
		return nil, fmt.Errorf("cannot read README file: %v", err)
	}
	dir.readMe = string(readMe)
	return dir, nil
}

// Data returns the contents of the bundle's bundle.yaml file.
func (dir *BundleDir) Data() *charm.BundleData {
	return dir.data
}

// BundleBytes implements Bundle.BundleBytes
func (dir *BundleDir) BundleBytes() []byte {
	return dir.bundleBytes
}

// ReadMe returns the contents of the bundle's README.md file.
func (dir *BundleDir) ReadMe() string {
	return dir.readMe
}

// ContainsOverlays returns true if the bundle contains any overlays.
func (dir *BundleDir) ContainsOverlays() bool {
	return dir.containsOverlays
}

func (dir *BundleDir) ArchiveTo(w io.Writer) error {
	return writeArchive(w, dir.Path, -1, "", nil)
}

// join builds a path rooted at the bundle's expanded directory
// path and the extra path components provided.
func (dir *BundleDir) join(parts ...string) string {
	parts = append([]string{dir.Path}, parts...)
	return filepath.Join(parts...)
}
