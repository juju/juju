// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/life"
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
