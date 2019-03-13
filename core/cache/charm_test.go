// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/cache"
)

type CharmSuite struct {
	entitySuite
}

var _ = gc.Suite(&CharmSuite{})

func (s *CharmSuite) SetUpTest(c *gc.C) {
	s.entitySuite.SetUpTest(c)
}

var charmChange = cache.CharmChange{
	ModelUUID: "model-uuid",
	CharmURL:  "www.charm-url.com",
}
