package testing

import (
	"go/build"
	"io/ioutil"
	"launchpad.net/juju-core/charm"
	"os"
	"os/exec"
	"path/filepath"
)

// repoPath holds the path to the uncloned testing charms.
var repoPath string

func init() {
	p, err := build.Import("launchpad.net/juju-core/testing", "", build.FindOnly)
	check(err)

	repoPath = filepath.Join(p.Dir, "repo")
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

// A Repo provides access to a set of testing charms.
// Charms are cloned before being made available.
// The series named "series" is always available.
type Repo struct {
	// Path holds the path to the charm repository.
	Path string
}

// Dir is a convenience method that calls DirWithSeries with
// the series "series".
func (s *Repo) Dir(name string) *charm.Dir {
	return s.DirWithSeries("series", name)
}

// DirWithSeries returns the charm.Dir with the requested testing
// charm. If the charm directory doesn't yet exist in the
// temporary repository, it will be copied from the pristine
// testing repository.
func (s *Repo) DirWithSeries(series, name string) *charm.Dir {
	// Read the directory first to give a nice error if the
	// charm does not exist.
	unclonedDir, err := charm.ReadDir(filepath.Join(repoPath, series, name))
	check(err)
	path := filepath.Join(s.Path, series, name)
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		s.ensureSeries(series)
		check(exec.Command("cp", "-r", unclonedDir.Path, path).Run())
	} else {
		check(err)
	}
	d, err := charm.ReadDir(path)
	check(err)
	if d.Path != path {
		panic("unexpected charm dir path: " + d.Path)
	}
	return d
}

func (s *Repo) ensureSeries(series string) string {
	if s.Path == "" {
		panic("CharmSuite used outside test")
	}
	dir := filepath.Join(s.Path, series)
	check(os.MkdirAll(dir, 0777))
	return dir
}

// URL returns a local URL for a charm with the given series and name.
func (s *Repo) URL(series, name string) *charm.URL {
	return &charm.URL{
		Schema:   "local",
		Series:   series,
		Name:     name,
		Revision: -1,
	}
}

// Bundle is a convenience method that calls BundleWithSeries with
// the series "series".
func (s *Repo) Bundle(name string) string {
	return s.BundleWithSeries("series", name)
}

// BundleWithSeries creates a bundle in the charm repository holding a copy
// of the testing charm with the given name and series and returns its
// path.
func (s *Repo) BundleWithSeries(series, name string) string {
	file, err := ioutil.TempFile(s.ensureSeries(series), name)
	check(err)
	dir, err := charm.ReadDir(filepath.Join(repoPath, series, name))
	check(err)
	check(dir.BundleTo(file))
	file.Close()
	path := file.Name() + ".charm"
	check(os.Rename(file.Name(), path))
	return path
}
