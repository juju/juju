// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/settings"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
)

const (
	branchName      = "test-branch"
	defaultPassword = "default-pass"
	defaultCharmURL = "default-charm-url"
	defaultUnitName = "redis/0"
)

type charmConfigWatcherSuite struct {
	EntitySuite
}

var _ = gc.Suite(&charmConfigWatcherSuite{})

func (s *charmConfigWatcherSuite) TestTrackingBranchChangedNotified(c *gc.C) {
	w := s.newWatcher(c, defaultUnitName, defaultCharmURL)
	s.assertOneChange(c, w, map[string]interface{}{"password": defaultPassword}, defaultCharmURL)

	// Publish a tracked branch change with altered config.
	b := Branch{
		details: BranchChange{
			Name:   branchName,
			Config: map[string]settings.ItemChanges{"redis": {settings.MakeAddition("password", "new-pass")}},
		},
	}
	s.Hub.Publish(branchChange, b)

	s.assertOneChange(c, w, map[string]interface{}{"password": "new-pass"}, defaultCharmURL)
	w.AssertStops()
}

func (s *charmConfigWatcherSuite) TestNotTrackingBranchChangedNotNotified(c *gc.C) {
	// This will initialise the watcher without branch info.
	w := s.newWatcher(c, "redis/9", defaultCharmURL)
	s.assertOneChange(c, w, map[string]interface{}{}, defaultCharmURL)

	// Publish a branch change with altered config.
	b := Branch{
		details: BranchChange{
			Name:   branchName,
			Config: map[string]settings.ItemChanges{"redis": {settings.MakeAddition("password", "new-pass")}},
		},
	}
	s.Hub.Publish(branchChange, b)

	// Nothing should change.
	w.AssertNoChange()
	w.AssertStops()
}

func (s *charmConfigWatcherSuite) TestDifferentBranchChangedNotNotified(c *gc.C) {
	w := s.newWatcher(c, defaultUnitName, defaultCharmURL)
	s.assertOneChange(c, w, map[string]interface{}{"password": defaultPassword}, defaultCharmURL)

	// Publish a branch change with a different name to the tracked one.
	b := Branch{
		details: BranchChange{
			Name:   "some-other-branch",
			Config: map[string]settings.ItemChanges{"redis": {settings.MakeAddition("password", "new-pass")}},
		},
	}
	s.Hub.Publish(branchChange, b)

	w.AssertNoChange()
	w.AssertStops()
}

func (s *charmConfigWatcherSuite) TestTrackingBranchMasterChangedNotified(c *gc.C) {
	w := s.newWatcher(c, defaultUnitName, defaultCharmURL)
	s.assertOneChange(c, w, map[string]interface{}{"password": defaultPassword}, defaultCharmURL)

	// Publish a change to master configuration.
	hc, _ := newHashCache(map[string]interface{}{"databases": 4}, nil, nil)
	s.Hub.Publish(applicationConfigChange, hc)

	s.assertOneChange(c, w, map[string]interface{}{"password": defaultPassword, "databases": 4}, defaultCharmURL)
	w.AssertStops()
}

func (s *charmConfigWatcherSuite) TestTrackingBranchCommittedNotNotified(c *gc.C) {
	w := s.newWatcher(c, "redis/0", defaultCharmURL)
	s.assertOneChange(c, w, map[string]interface{}{"password": defaultPassword}, defaultCharmURL)

	// Publish a branch removal.
	s.Hub.Publish(modelBranchRemove, branchName)
	w.AssertNoChange()
	w.AssertStops()
}

func (s *charmConfigWatcherSuite) TestNotTrackedBranchSeesMasterConfig(c *gc.C) {
	// Watcher is for a unit not tracking the branch.
	w := s.newWatcher(c, "redis/9", defaultCharmURL)
	s.assertOneChange(c, w, map[string]interface{}{}, defaultCharmURL)
	w.AssertStops()
}

func (s *charmConfigWatcherSuite) TestSameUnitDifferentCharmURLYieldsDifferentHash(c *gc.C) {
	w := s.newWatcher(c, defaultUnitName, defaultCharmURL)
	s.assertOneChange(c, w, map[string]interface{}{"password": defaultPassword}, defaultCharmURL)
	h1 := w.Watcher.(*CharmConfigWatcher).configHash
	w.AssertStops()

	w = s.newWatcher(c, defaultUnitName, "different-charm-url")
	s.assertOneChange(c, w, map[string]interface{}{"password": defaultPassword}, "different-charm-url")
	h2 := w.Watcher.(*CharmConfigWatcher).configHash
	w.AssertStops()

	c.Check(h1, gc.Not(gc.Equals), h2)
}

func (s *charmConfigWatcherSuite) newWatcher(c *gc.C, unitName string, charmURL string) StringsWatcherC {
	appName, err := names.UnitApplication(unitName)
	c.Assert(err, jc.ErrorIsNil)

	// The topics can be arbitrary here;
	// these tests are isolated from actual cache behaviour.
	cfg := charmConfigWatcherConfig{
		model:                s.newStubModel(),
		unitName:             unitName,
		appName:              appName,
		charmURL:             charmURL,
		appConfigChangeTopic: applicationConfigChange,
		branchChangeTopic:    branchChange,
		branchRemoveTopic:    modelBranchRemove,
		hub:                  s.Hub,
		res:                  s.NewResident(),
	}

	w, err := newCharmConfigWatcher(cfg)
	c.Assert(err, jc.ErrorIsNil)

	// Wrap the watcher and ensure we get the default notification.
	wc := NewStringsWatcherC(c, w)
	return wc
}

// newStub model sets up a cached model containing a redis application
// and a branch with 2 redis units tracking it.
func (s *charmConfigWatcherSuite) newStubModel() *stubCharmConfigModel {
	app := newApplication(s.Gauges, s.Hub, s.NewResident())
	app.setDetails(ApplicationChange{
		Name:   "redis",
		Config: map[string]interface{}{}},
	)

	branch := newBranch(s.Gauges, s.Hub, s.NewResident())
	branch.setDetails(BranchChange{
		Name:          branchName,
		AssignedUnits: map[string][]string{"redis": {"redis/0", "redis/1"}},
		Config:        map[string]settings.ItemChanges{"redis": {settings.MakeAddition("password", defaultPassword)}},
	})

	return &stubCharmConfigModel{
		app:      *app,
		branches: map[string]Branch{"0": *branch},
	}
}

// assertWatcherConfig unwraps the charm config watcher and ensures that its
// configuration hash matches that of the input configuration map.
func (s *charmConfigWatcherSuite) assertOneChange(
	c *gc.C, wc StringsWatcherC, cfg map[string]interface{}, extra ...string,
) {
	h, err := hash(cfg, extra...)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange([]string{h})
}

type stubCharmConfigModel struct {
	app      Application
	branches map[string]Branch
}

func (m *stubCharmConfigModel) Application(name string) (Application, error) {
	if name == m.app.details.Name {
		return m.app, nil
	}
	return Application{}, errors.NotFoundf("application %q", name)
}

func (m *stubCharmConfigModel) Branches() []Branch {
	branches := make([]Branch, len(m.branches))
	i := 0
	for _, b := range m.branches {
		branches[i] = b.copy()
		i += 1
	}
	return branches
}
