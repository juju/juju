package testing

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/symlink"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
)

// BaseRepoSuite sets up $JUJU_REPOSITORY to point to a local charm repository.
type BaseRepoSuite struct {
	SeriesPath  string
	BundlesPath string
	RepoPath    string
}

func (s *BaseRepoSuite) SetUpSuite(c *gc.C)    {}
func (s *BaseRepoSuite) TearDownSuite(c *gc.C) {}

func (s *BaseRepoSuite) SetUpTest(c *gc.C) {
	// Set up a local repository.
	s.RepoPath = os.Getenv("JUJU_REPOSITORY")
	repoPath := c.MkDir()
	os.Setenv("JUJU_REPOSITORY", repoPath)
	s.SeriesPath = filepath.Join(repoPath, config.LatestLtsSeries())
	c.Assert(os.Mkdir(s.SeriesPath, 0777), jc.ErrorIsNil)
	// Create a symlink "quantal" -> "precise", because most charms
	// and machines are written with hard-coded "quantal" series,
	// hence they interact badly with a local repository that assumes
	// only "precise" charms are available.
	err := symlink.New(s.SeriesPath, filepath.Join(repoPath, "quantal"))
	c.Assert(err, jc.ErrorIsNil)
	s.BundlesPath = filepath.Join(repoPath, "bundle")
	c.Assert(os.Mkdir(s.BundlesPath, 0777), jc.ErrorIsNil)
}

func (s *BaseRepoSuite) TearDownTest(c *gc.C) {
	os.Setenv("JUJU_REPOSITORY", s.RepoPath)
}

// RepoSuite acts as a JujuConnSuite but also sets up
// $JUJU_REPOSITORY to point to a local charm repository.
type RepoSuite struct {
	JujuConnSuite
	BaseRepoSuite
}

func (s *RepoSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.BaseRepoSuite.SetUpSuite(c)
}

func (s *RepoSuite) TearDownSuite(c *gc.C) {
	s.BaseRepoSuite.TearDownSuite(c)
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *RepoSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.BaseRepoSuite.SetUpTest(c)
	// Change the environ's config to ensure we're using the one in state.
	updateAttrs := map[string]interface{}{"default-series": config.LatestLtsSeries()}
	err := s.State.UpdateModelConfig(updateAttrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RepoSuite) TearDownTest(c *gc.C) {
	s.BaseRepoSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *RepoSuite) AssertService(c *gc.C, name string, expectCurl *charm.URL, unitCount, relCount int) (*state.Service, []*state.Relation) {
	svc, err := s.State.Service(name)
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := svc.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, expectCurl)
	s.AssertCharmUploaded(c, expectCurl)

	units, err := svc.AllUnits()
	c.Logf("Service units: %+v", units)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, unitCount)
	s.AssertUnitMachines(c, units)
	rels, err := svc.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rels, gc.HasLen, relCount)
	return svc, rels
}

func (s *RepoSuite) AssertCharmUploaded(c *gc.C, curl *charm.URL) {
	ch, err := s.State.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)

	storage := storage.NewStorage(s.State.ModelUUID(), s.State.MongoSession())
	r, _, err := storage.Get(ch.StoragePath())
	c.Assert(err, jc.ErrorIsNil)
	defer r.Close()

	digest, _, err := utils.ReadSHA256(r)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.BundleSha256(), gc.Equals, digest)
}

func (s *RepoSuite) AssertUnitMachines(c *gc.C, units []*state.Unit) {
	tags := make([]names.UnitTag, len(units))
	expectUnitNames := make([]string, len(units))
	for i, u := range units {
		expectUnitNames[i] = u.Name()
		tags[i] = u.UnitTag()
	}

	// manually assign all units to machines.  This replaces work normally done
	// by the unitassigner code.
	errs, err := s.APIState.UnitAssigner().AssignUnits(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, make([]error, len(units)))

	sort.Strings(expectUnitNames)

	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, len(units))

	unitNames := []string{}
	for _, m := range machines {
		mUnits, err := m.Units()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(mUnits, gc.HasLen, 1)
		unitNames = append(unitNames, mUnits[0].Name())
	}
	sort.Strings(unitNames)
	c.Assert(unitNames, gc.DeepEquals, expectUnitNames)
}
