package testing

import (
	"go/build"
	"io/ioutil"
	"launchpad.net/juju-core/charm"
	. "launchpad.net/gocheck"
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

// CharmSuite is a test fixture that provides access to
// a set of testing charms. Charms are cloned before
// returning them. The series named "series" is always
// available.
type CharmSuite struct {
	// RepoPath holds the path to the charm repository.
	RepoPath string
}

func (s *CharmSuite) SetUpSuite(c *C) {}
func (s *CharmSuite) TearDownSuite(c *C) {}

func (s *CharmSuite) SetUpTest(c *C) {
	s.RepoPath = c.MkDir()
}

func (s *CharmSuite) TearDownTest(c *C) {
	s.RepoPath = ""
}

func (s *CharmSuite) Reset(c *C) {
	s.RepoPath = c.MkDir()
}

// CharmDir creates a charm directory holding a copy of
// the testing charm with the given name and series.
// It does nothing if the directory already exists.
func (s *CharmSuite) CharmDir(series, name string) *charm.Dir {
	// Read the directory first to give a nice error if the
	// charm does not exist.
	unclonedDir, err := charm.ReadDir(filepath.Join(repoPath, series, name))
	check(err)
	path := filepath.Join(s.RepoPath, series, name)
	_, err = os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			check(err)
		}
		s.ensureSeries(series)
		check(exec.Command("cp", "-r", unclonedDir.Path, path).Run())
	}
	d, err := charm.ReadDir(path)
	check(err)
	return d
}

func (s *CharmSuite) ensureSeries(series string) string{
	dir := filepath.Join(s.RepoPath, series)
	check(os.MkdirAll(dir, 0777))
	return dir
}

// CharmURL returns a local URL for a charm with the given series and name.
func (s *CharmSuite) CharmURL(series, name string) *charm.URL {
	return &charm.URL{
		Schema:   "local",
		Series:   series,
		Name:     name,
		Revision: -1,
	}
}

// CharmBundlePath creates a charm bundle holding a copy
// of the testing charm with the given name and series
// and returns its path.
func (s *CharmSuite) CharmBundle(series, name string) string {
	file, err := ioutil.TempFile(s.ensureSeries(series), name)
	check(err)
	defer file.Close()
	dir, err := charm.ReadDir(filepath.Join(repoPath, series, name))
	check(err)
	check(dir.BundleTo(file))
	path := file.Name() + ".charm"
	check(os.Rename(file.Name(), path))
	return path
}
