// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package cache_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
)

type ApplicationSuite struct {
	entitySuite
}

var _ = gc.Suite(&ApplicationSuite{})

func (s *ApplicationSuite) SetUpTest(c *gc.C) {
	s.entitySuite.SetUpTest(c)
}

var appChange = cache.ApplicationChange{
	ModelUUID:   "model-uuid",
	Name:        "application-name",
	Exposed:     false,
	CharmURL:    "www.charm-url.com",
	Life:        life.Alive,
	MinUnits:    0,
	Constraints: constraints.Value{},
	Config: map[string]interface{}{
		"key":     "value",
		"another": "foo",
	},
	Subordinate:     false,
	Status:          status.StatusInfo{Status: status.Active},
	WorkloadVersion: "666",
}
