// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repo

// File moved from github.com/juju/charmrepo/v7/testing/charm.go
// Attempting to remove use of the charmrepo package from juju
// with the removal of charmstore support.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/juju/juju/charm"
	"github.com/juju/utils/v4/fs"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

// NewRepo returns a new testing charm repository rooted at the given
// path, relative to the package directory of the calling package, using
// defaultSeries as the default series.
func NewRepo(path, defaultSeries string) *CharmRepo {
	// Find the repo directory. This is only OK to do
	// because this is running in a test context
	// so we know the source is available.
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		panic("cannot get caller")
	}
	r := &CharmRepo{
		path:          filepath.Join(filepath.Dir(file), path),
		defaultSeries: defaultSeries,
	}
	_, err := os.Stat(r.path)
	if err != nil {
		panic(fmt.Errorf("cannot read repository found at %q: %v", r.path, err))
	}
	return r
}

// CharmRepo represents a charm repository used for testing.
type CharmRepo struct {
	path          string
	defaultSeries string
}

func (r *CharmRepo) Path() string {
	return r.path
}

func clone(dst, src string) string {
	dst = filepath.Join(dst, filepath.Base(src))
	check(fs.Copy(src, dst))
	return dst
}

// BundleDirPath returns the path to a bundle directory with the given name in the
// default series
func (r *CharmRepo) BundleDirPath(name string) string {
	return filepath.Join(r.Path(), "bundle", name)
}

// BundleDir returns the actual charm.BundleDir named name.
func (r *CharmRepo) BundleDir(name string) *charm.BundleDir {
	b, err := charm.ReadBundleDir(r.BundleDirPath(name))
	check(err)
	return b
}

// CharmDirPath returns the path to a charm directory with the given name in the
// default series
func (r *CharmRepo) CharmDirPath(name string) string {
	return filepath.Join(r.Path(), r.defaultSeries, name)
}

// CharmDir returns the actual charm.CharmDir named name.
func (r *CharmRepo) CharmDir(name string) *charm.CharmDir {
	ch, err := charm.ReadCharmDir(r.CharmDirPath(name))
	check(err)
	return ch
}

// ClonedDirPath returns the path to a new copy of the default charm directory
// named name.
func (r *CharmRepo) ClonedDirPath(dst, name string) string {
	return clone(dst, r.CharmDirPath(name))
}

// ClonedDirPath returns the path to a new copy of the default bundle directory
// named name.
func (r *CharmRepo) ClonedBundleDirPath(dst, name string) string {
	return clone(dst, r.BundleDirPath(name))
}

// RenamedClonedDirPath returns the path to a new copy of the default
// charm directory named name, renamed to newName.
func (r *CharmRepo) RenamedClonedDirPath(dst, name, newName string) string {
	dstPath := filepath.Join(dst, newName)
	err := fs.Copy(r.CharmDirPath(name), dstPath)
	check(err)
	return dstPath
}

// ClonedDir returns an actual charm.CharmDir based on a new copy of the charm directory
// named name, in the directory dst.
func (r *CharmRepo) ClonedDir(dst, name string) *charm.CharmDir {
	ch, err := charm.ReadCharmDir(r.ClonedDirPath(dst, name))
	check(err)
	return ch
}

// ClonedURL makes a copy of the charm directory. It will create a directory
// with the series name if it does not exist, and then clone the charm named
// name into that directory. The return value is a URL pointing at the local
// charm.
func (r *CharmRepo) ClonedURL(dst, series, name string) *charm.URL {
	dst = filepath.Join(dst, series)
	if err := os.MkdirAll(dst, os.FileMode(0777)); err != nil {
		panic(fmt.Errorf("cannot make destination directory: %v", err))
	}
	clone(dst, r.CharmDirPath(name))
	return &charm.URL{
		Schema:   "local",
		Name:     name,
		Revision: -1,
		Series:   series,
	}
}

// CharmArchivePath returns the path to a new charm archive file
// in the directory dst, created from the charm directory named name.
func (r *CharmRepo) CharmArchivePath(dst, name string) string {
	dir := r.CharmDir(name)
	path := filepath.Join(dst, "archive.charm")
	file, err := os.Create(path)
	check(err)
	defer func() { _ = file.Close() }()
	check(dir.ArchiveTo(file))
	return path
}

// BundleArchivePath returns the path to a new bundle archive file
// in the directory dst, created from the bundle directory named name.
func (r *CharmRepo) BundleArchivePath(dst, name string) string {
	dir := r.BundleDir(name)
	path := filepath.Join(dst, "archive.bundle")
	file, err := os.Create(path)
	check(err)
	defer func() { _ = file.Close() }()
	check(dir.ArchiveTo(file))
	return path
}

// CharmArchive returns an actual charm.CharmArchive created from a new
// charm archive file created from the charm directory named name, in
// the directory dst.
func (r *CharmRepo) CharmArchive(dst, name string) *charm.CharmArchive {
	ch, err := charm.ReadCharmArchive(r.CharmArchivePath(dst, name))
	check(err)
	return ch
}

// BundleArchive returns an actual charm.BundleArchive created from a new
// bundle archive file created from the bundle directory named name, in
// the directory dst.
func (r *CharmRepo) BundleArchive(dst, name string) *charm.BundleArchive {
	b, err := charm.ReadBundleArchive(r.BundleArchivePath(dst, name))
	check(err)
	return b
}
