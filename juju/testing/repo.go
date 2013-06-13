package testing

import (
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"net/http"
	"os"
	"path/filepath"
	"sort"
)

// RepoSuite acts as a JujuConnSuite but also sets up
// $JUJU_REPOSITORY to point to a local charm repository.
type RepoSuite struct {
	JujuConnSuite
	SeriesPath string
	RepoPath   string
}

func (s *RepoSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	// Change the environ's config to ensure we're using the one in state,
	// not the one in the local environments.yaml
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)
	cfg, err = cfg.Apply(map[string]interface{}{"default-series": "precise"})
	c.Assert(err, IsNil)
	err = s.State.SetEnvironConfig(cfg)
	c.Assert(err, IsNil)
	s.RepoPath = os.Getenv("JUJU_REPOSITORY")
	repoPath := c.MkDir()
	os.Setenv("JUJU_REPOSITORY", repoPath)
	s.SeriesPath = filepath.Join(repoPath, "precise")
	err = os.Mkdir(s.SeriesPath, 0777)
	c.Assert(err, IsNil)
}

func (s *RepoSuite) TearDownTest(c *C) {
	os.Setenv("JUJU_REPOSITORY", s.RepoPath)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *RepoSuite) AssertService(c *C, name string, expectCurl *charm.URL, unitCount, relCount int) (*state.Service, []*state.Relation) {
	svc, err := s.State.Service(name)
	c.Assert(err, IsNil)
	ch, _, err := svc.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.URL(), DeepEquals, expectCurl)
	s.AssertCharmUploaded(c, expectCurl)
	units, err := svc.AllUnits()
	c.Logf("Service units: %+v", units)
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, unitCount)
	s.AssertUnitMachines(c, units)
	rels, err := svc.Relations()
	c.Assert(err, IsNil)
	c.Assert(rels, HasLen, relCount)
	return svc, rels
}

func (s *RepoSuite) AssertCharmUploaded(c *C, curl *charm.URL) {
	ch, err := s.State.Charm(curl)
	c.Assert(err, IsNil)
	url := ch.BundleURL()
	resp, err := http.Get(url.String())
	c.Assert(err, IsNil)
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	h := sha256.New()
	h.Write(body)
	digest := hex.EncodeToString(h.Sum(nil))
	c.Assert(ch.BundleSha256(), Equals, digest)
}

func (s *RepoSuite) AssertUnitMachines(c *C, units []*state.Unit) {
	expectUnitNames := []string{}
	for _, u := range units {
		expectUnitNames = append(expectUnitNames, u.Name())
	}
	sort.Strings(expectUnitNames)

	machines, err := s.State.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(machines, HasLen, len(units))
	unitNames := []string{}
	for _, m := range machines {
		mUnits, err := m.Units()
		c.Assert(err, IsNil)
		c.Assert(mUnits, HasLen, 1)
		unitNames = append(unitNames, mUnits[0].Name())
	}
	sort.Strings(unitNames)
	c.Assert(unitNames, DeepEquals, expectUnitNames)
}
