package testing

import (
	"fmt"
	"go/build"
	"launchpad.net/juju-core/charm"
	"os"
	"os/exec"
	"path/filepath"
)

func init() {
	p, err := build.Import("launchpad.net/juju-core/testing", "", build.FindOnly)
	check(err)
	Charms = &Repo{Path: filepath.Join(p.Dir, "repo")}
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

// Repo represents a charm repository used for testing.
type Repo struct {
	Path string
}

// Charms represents the specific charm repository stored in this package and
// used by the Juju unit tests. The series named "series" always exists.
var Charms *Repo

func clone(dst, src string) string {
	check(exec.Command("cp", "-r", src, dst).Run())
	return filepath.Join(dst, filepath.Base(src))
}

// DirPath returns the path to a charm directory named name
// and series.
func (r *Repo) DirPath(series, name string) string {
	return filepath.Join(r.Path, series, name)
}

// Dir returns the actual charm.Dir named name.
func (r *Repo) Dir(series, name string) *charm.Dir {
	ch, err := charm.ReadDir(r.DirPath(series, name))
	check(err)
	return ch
}

// ClonedDirPath returns the path to a new copy of the charm directory
// named name with the given series.
func (r *Repo) ClonedDirPath(dst, series, name string) string {
	return clone(dst, r.DirPath(series, name))
}

// ClonedDir returns an actual charm.Dir based on a new copy of the charm directory
// named name, with the given series, in the directory dst.
func (r *Repo) ClonedDir(dst, series, name string) *charm.Dir {
	ch, err := charm.ReadDir(r.ClonedDirPath(dst, series, name))
	check(err)
	return ch
}

// ClonedURL makes a copy of the charm directory named name
// into the destination directory (creating it if necessary),
// and returns a URL for it.
func (r *Repo) ClonedURL(dst, series, name string) *charm.URL {
	dst = filepath.Join(dst, series)
	if err := os.MkdirAll(dst, 0777); err != nil {
		panic(fmt.Errorf("cannot make destination directory: %v", err))
	}
	clone(dst, r.DirPath(series, name))
	return &charm.URL{
		Schema:   "local",
		Series:   series,
		Name:     name,
		Revision: -1,
	}
}

// BundlePath returns the path to a new charm bundle file created from the charm
// directory named name, with the given series, in the directory dst.
func (r *Repo) BundlePath(dst, series, name string) string {
	dir := r.Dir(series, name)
	path := filepath.Join(dst, "bundle.charm")
	file, err := os.Create(path)
	check(err)
	defer file.Close()
	check(dir.BundleTo(file))
	return path
}

// Bundle returns an actual charm.Bundle created from a new charm bundle file
// created from the charm directory named name, with the given series, in the
// directory dst.
func (r *Repo) Bundle(dst, series, name string) *charm.Bundle {
	ch, err := charm.ReadBundle(r.BundlePath(dst, series, name))
	check(err)
	return ch
}
