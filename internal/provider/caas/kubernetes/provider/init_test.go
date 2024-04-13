// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/provider/caas/kubernetes/provider"
	"github.com/juju/juju/testing"
)

type initSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&initSuite{})

func (s *initSuite) TestLabelSelectorGlobalResourcesLifecycle(c *gc.C) {
	c.Assert(
		provider.CompileLifecycleModelTeardownSelector().String(), gc.DeepEquals,
		`juju-resource-lifecycle notin (persistent)`,
	)
}
