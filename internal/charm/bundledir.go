// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/juju/juju/core/logger"
)

// BundleDir defines a bundle from a given directory.
type BundleDir struct {
	Path        string
	data        *BundleData
	bundleBytes []byte
	readMe      string

	containsOverlays bool
	logger           logger.Logger
}

// Trick to ensure *BundleDir implements the Bundle interface.
var _ Bundle = (*BundleDir)(nil)

// ReadBundleDir returns a BundleDir representing an expanded
// bundle directory. It does not verify the bundle data.
func ReadBundleDir(path string, options ...ReadOption) (dir *BundleDir, err error) {
	opts := newReadOptions(options)

	dir = &BundleDir{
		Path:   path,
		logger: opts.logger,
	}
	b, err := os.ReadFile(dir.join("bundle.yaml"))
	if err != nil {
		return nil, err
	}
	dir.bundleBytes = b
	dir.data, dir.containsOverlays, err = readBaseFromMultidocBundle(b)
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
func (dir *BundleDir) Data() *BundleData {
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
	return writeArchive(w, dir.Path, -1, "", nil, nil, dir.logger)
}

// join builds a path rooted at the bundle's expanded directory
// path and the extra path components provided.
func (dir *BundleDir) join(parts ...string) string {
	parts = append([]string{dir.Path}, parts...)
	return filepath.Join(parts...)
}
