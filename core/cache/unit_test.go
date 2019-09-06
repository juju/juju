// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/settings"
	"github.com/juju/juju/core/status"
)

type UnitSuite struct {
	cache.EntitySuite
}

var _ = gc.Suite(&UnitSuite{})

func (s *UnitSuite) TestWatchCharmConfigNewWatcher(c *gc.C) {
	m := s.NewModel(modelChange)
	m.UpdateApplication(appChange, s.Manager)
	m.UpdateUnit(unitChange, s.Manager)

	u, err := m.Unit(unitChange.Name)
	c.Assert(err, jc.ErrorIsNil)

	w, err := u.WatchConfigSettings()
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *UnitSuite) TestConfigSettingsNoBranch(c *gc.C) {
	m := s.NewModel(modelChange)
	m.UpdateCharm(charmChange, s.Manager)
	m.UpdateApplication(appChange, s.Manager)
	m.UpdateUnit(unitChange, s.Manager)

	u, err := m.Unit(unitChange.Name)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := u.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)

	expected := charm.Settings{
		"key":       "value",
		"another":   "foo",
		"something": "else",
	}
	c.Assert(cfg, gc.DeepEquals, expected)
}

func (s *UnitSuite) TestConfigSettingsBranch(c *gc.C) {
	m := s.NewModel(modelChange)
	m.UpdateCharm(charmChange, s.Manager)
	m.UpdateApplication(appChange, s.Manager)
	m.UpdateUnit(unitChange, s.Manager)

	br := branchChange
	br.AssignedUnits = map[string][]string{appChange.Name: {unitChange.Name}}
	br.Config = map[string]settings.ItemChanges{
		appChange.Name: {
			settings.MakeAddition("new-key", "new-value"),
			settings.MakeDeletion("key", "this-will-revert-to-default"),
			settings.MakeModification("another", "foo", "new-foo"),
		},
	}
	m.UpdateBranch(br, s.Manager)

	u, err := m.Unit(unitChange.Name)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := u.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)

	expected := charm.Settings{
		"key":       "default-value",
		"another":   "new-foo",
		"new-key":   "new-value",
		"something": "else",
	}
	c.Assert(cfg, gc.DeepEquals, expected)
}

func (s *UnitSuite) TestConfigSettingsDefaultsOnly(c *gc.C) {
	appNoCfg := appChange
	appNoCfg.Config = nil

	m := s.NewModel(modelChange)
	m.UpdateCharm(charmChange, s.Manager)
	m.UpdateApplication(appNoCfg, s.Manager)
	m.UpdateUnit(unitChange, s.Manager)

	u, err := m.Unit(unitChange.Name)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := u.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)

	expected := charm.Settings{
		"key":       "default-value",
		"something": "else",
	}
	c.Assert(cfg, gc.DeepEquals, expected)
}

var unitChange = cache.UnitChange{
	ModelUUID:      "model-uuid",
	Name:           "application-name/0",
	Application:    "application-name",
	Series:         "bionic",
	Life:           life.Alive,
	CharmURL:       "www.charm-url.com-1",
	PublicAddress:  "",
	PrivateAddress: "",
	MachineId:      "0",
	Ports:          nil,
	PortRanges:     nil,
	Subordinate:    false,
	WorkloadStatus: status.StatusInfo{Status: status.Active},
	AgentStatus:    status.StatusInfo{Status: status.Active},
}

func (s *UnitSuite) TestWaitForChangeUnitCharmURL(c *gc.C) {
	m := s.NewModel(modelChange)
	// We could just pass nil through as the cancel channel as this is the success
	// case, but it just feels cleaner to use a real channel.
	cancel := make(chan struct{})
	defer close(cancel)
	done := m.WaitForChange(unitChange.Name, cache.UnitCharmURLField, unitChange.CharmURL, cancel)

	m.UpdateUnit(unitChange, s.Manager)

	select {
	case <-done:
		// All good.
	case <-time.After(testing.LongWait):
		c.Errorf("change not noticed")
	}
}

func (s *UnitSuite) TestWaitForChangeUnitCharmURLDifferent(c *gc.C) {
	// If the charm is updated with a different charmURL than expected, the done
	// channel is not signalled.
	m := s.NewModel(modelChange)

	cancel := make(chan struct{})
	defer close(cancel)
	done := m.WaitForChange(unitChange.Name, cache.UnitCharmURLField, "not-going-to-happen", cancel)

	m.UpdateUnit(unitChange, s.Manager)

	select {
	case <-done:
		c.Errorf("unexpected change signalled")
	case <-time.After(testing.ShortWait):
		// All good.
	}
}
