// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
)

type MachineSuite struct {
	entitySuite
}

var _ = gc.Suite(&MachineSuite{})

func (s *MachineSuite) SetUpTest(c *gc.C) {
	s.entitySuite.SetUpTest(c)
}

var machineChange = cache.MachineChange{
	ModelUUID:      "model-uuid",
	Id:             "0",
	InstanceId:     "juju-gd4c23-0",
	AgentStatus:    status.StatusInfo{Status: status.Active},
	InstanceStatus: status.StatusInfo{Status: status.Active},
	Life:           life.Alive,
	Config: map[string]interface{}{
		"key":     "value",
		"another": "foo",
	},
	Series:                   "bionic",
	SupportedContainers:      []instance.ContainerType{},
	SupportedContainersKnown: false,
	HasVote:                  true,
	WantsVote:                true,
}
