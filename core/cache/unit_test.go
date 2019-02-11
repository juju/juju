// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package cache_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/status"
)

type UnitSuite struct {
	entitySuite
}

var _ = gc.Suite(&UnitSuite{})

func (s *UnitSuite) SetUpTest(c *gc.C) {
	s.entitySuite.SetUpTest(c)
}

var unitChange = cache.UnitChange{
	ModelUUID:      "model-uuid",
	Name:           "application-name/0",
	Application:    "application-name",
	Series:         "bionic",
	CharmURL:       "www.charm-url.com",
	PublicAddress:  "",
	PrivateAddress: "",
	MachineId:      "0",
	Subordinate:    false,
	WorkloadStatus: status.StatusInfo{Status: status.Active},
	AgentStatus:    status.StatusInfo{Status: status.Active},
}
