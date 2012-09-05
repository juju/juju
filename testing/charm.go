package testing

import (
	"go/build"
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
	path := filepath.Join(s.RepoPath, series, name)
	info, err := os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			check(err)
		}
		unclonedPath := filepath.Join(repoPath, series, name)
		// First check that the source exists, so that we don't
		// see an obscure status error message from the cp command.
		if _, err := os.Stat(unclonedPath); err != nil {
			panic(err)
		}
		s.ensureSeries(series)
		check(exec.Command("cp", "-r", unclonedPath, path).Run())
	}
	d, err := charm.ReadDir(path)
	check(err)
	return d
}

func (s *CharmSuite) ensureSeries(series string) {
	check(os.MkdirAll(filepath.Join(s.RepoPath, series), 0777))
}

// CharmURL returns a URL for the charm with the given series and name,
// first calling CharmDir to ensure that the charm exists.
func (s *CharmSuite) CharmURL(series, name string) *charm.URL {
	s.CharmDir(series, name)
	return &charm.URL{
		Schema:   "local",
		Series:   series,
		Name:     name,
		Revision: -1,
	}
}

// CharmBundlePath creates a charm bundle holding a copy
// of the testing charm with the given name and series
// and returns its path. If does nothing if the bundle already exists.
func (s *CharmSuite) CharmBundle(dst, series, name string) string {
	s.ensureSeries(series)
	path := filepath.Join(s.RepoPath, series, name)
	path := filepath.Join(dst, "bundle.charm")
	file, err := os.Create(path)
	check(err)
	defer file.Close()
	check(dir.BundleTo(file))
	return path
}
