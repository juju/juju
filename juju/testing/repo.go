package testing

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/utils"
)

// RepoSuite acts as a JujuConnSuite but also sets up
// $JUJU_REPOSITORY to point to a local charm repository.
type RepoSuite struct {
	JujuConnSuite
	SeriesPath string
	RepoPath   string
}

func (s *RepoSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	// Change the environ's config to ensure we're using the one in state,
	// not the one in the local environments.yaml
	updateAttrs := map[string]interface{}{"default-series": "precise"}
	err := s.State.UpdateEnvironConfig(updateAttrs, nil, nil)
	c.Assert(err, gc.IsNil)
	s.RepoPath = os.Getenv("JUJU_REPOSITORY")
	repoPath := c.MkDir()
	os.Setenv("JUJU_REPOSITORY", repoPath)
	s.SeriesPath = filepath.Join(repoPath, "precise")
	err = os.Mkdir(s.SeriesPath, 0777)
	c.Assert(err, gc.IsNil)
	// Create a symlink "quantal" -> "precise", because most charms
	// and machines are written with hard-coded "quantal" series,
	// hence they interact badly with a local repository that assumes
	// only "precise" charms are available.
	err = os.Symlink(s.SeriesPath, filepath.Join(repoPath, "quantal"))
	c.Assert(err, gc.IsNil)
}

func (s *RepoSuite) TearDownTest(c *gc.C) {
	os.Setenv("JUJU_REPOSITORY", s.RepoPath)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *RepoSuite) AssertService(c *gc.C, name string, expectCurl *charm.URL, unitCount, relCount int) (*state.Service, []*state.Relation) {
	svc, err := s.State.Service(name)
	c.Assert(err, gc.IsNil)
	ch, _, err := svc.Charm()
	c.Assert(err, gc.IsNil)
	c.Assert(ch.URL(), gc.DeepEquals, expectCurl)
	s.AssertCharmUploaded(c, expectCurl)
	units, err := svc.AllUnits()
	c.Logf("Service units: %+v", units)
	c.Assert(err, gc.IsNil)
	c.Assert(units, gc.HasLen, unitCount)
	s.AssertUnitMachines(c, units)
	rels, err := svc.Relations()
	c.Assert(err, gc.IsNil)
	c.Assert(rels, gc.HasLen, relCount)
	return svc, rels
}

func (s *RepoSuite) AssertCharmUploaded(c *gc.C, curl *charm.URL) {
	ch, err := s.State.Charm(curl)
	c.Assert(err, gc.IsNil)
	url := ch.BundleURL()
	resp, err := http.Get(url.String())
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()
	digest, _, err := utils.ReadSHA256(resp.Body)
	c.Assert(err, gc.IsNil)
	c.Assert(ch.BundleSha256(), gc.Equals, digest)
}

func (s *RepoSuite) AssertUnitMachines(c *gc.C, units []*state.Unit) {
	expectUnitNames := []string{}
	for _, u := range units {
		expectUnitNames = append(expectUnitNames, u.Name())
	}
	sort.Strings(expectUnitNames)

	machines, err := s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, len(units))
	unitNames := []string{}
	for _, m := range machines {
		mUnits, err := m.Units()
		c.Assert(err, gc.IsNil)
		c.Assert(mUnits, gc.HasLen, 1)
		unitNames = append(unitNames, mUnits[0].Name())
	}
	sort.Strings(unitNames)
	c.Assert(unitNames, gc.DeepEquals, expectUnitNames)
}
