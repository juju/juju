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

type BundleDir struct {
	Path   string
	data   *BundleData
	readMe string
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
	dir.data, err = ReadBundleData(file)
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

func (dir *BundleDir) Data() *BundleData {
	return dir.data
}

func (dir *BundleDir) ReadMe() string {
	return dir.readMe
}

func (dir *BundleDir) ArchiveTo(w io.Writer) error {
	return writeArchive(w, dir.Path, -1, nil)
}

// join builds a path rooted at the bundle's expanded directory
// path and the extra path components provided.
func (dir *BundleDir) join(parts ...string) string {
	parts = append([]string{dir.Path}, parts...)
	return filepath.Join(parts...)
}
