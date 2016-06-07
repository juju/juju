package testing

import (
	"sort"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
)

type RepoSuite struct {
	JujuConnSuite
	CharmsPath string
}

func (s *RepoSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.CharmsPath = c.MkDir()
	// Change the environ's config to ensure we're using the one in state.
	updateAttrs := map[string]interface{}{"default-series": series.LatestLts()}
	err := s.State.UpdateModelConfig(updateAttrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RepoSuite) AssertService(c *gc.C, name string, expectCurl *charm.URL, unitCount, relCount int) (*state.Application, []*state.Relation) {
	svc, err := s.State.Application(name)
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
