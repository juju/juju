// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

// BundleDir defines a bundle from a given directory.
type BundleDir struct {
	Path   string
	data   *BundleData
	readMe string

	containsOverlays bool
}

// Trick to ensure *BundleDir implements the Bundle interface.
var _ Bundle = (*BundleDir)(nil)

// ReadBundleDir returns a BundleDir representing an expanded
// bundle directory. It does not verify the bundle data.
func ReadBundleDir(path string) (dir *BundleDir, err error) {
	dir = &BundleDir{Path: path}
	file, err := os.Open(dir.join("bundle.yaml"))
	if err != nil {
		return nil, err
	}
	dir.data, dir.containsOverlays, err = readBaseFromMultidocBundle(file)
	file.Close()
	if err != nil {
		return nil, err
	}
	readMe, err := ioutil.ReadFile(dir.join("README.md"))
	if err != nil {
		return nil, fmt.Errorf("cannot read README file: %v", err)
	}
	dir.readMe = string(readMe)
	return dir, nil
}

// Data returns the contents of the bundle's bundle.yaml file.
func (dir *BundleDir) Data() *BundleData {
	return dir.data
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
	return writeArchive(w, dir.Path, -1, "", nil, nil)
}

// join builds a path rooted at the bundle's expanded directory
// path and the extra path components provided.
func (dir *BundleDir) join(parts ...string) string {
	parts = append([]string{dir.Path}, parts...)
	return filepath.Join(parts...)
}
