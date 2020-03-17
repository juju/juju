// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/testing"
)

type initSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&initSuite{})

func (s *initSuite) TestLabelSelectorGlobalResourcesLifecycle(c *gc.C) {
	c.Assert(
		provider.CompileLifecycleApplicationRemovalSelector().String(), gc.DeepEquals,
		`juju-resource-lifecycle notin (model,persistent)`,
	)
	c.Assert(
		provider.CompileLifecycleModelTeardownSelector().String(), gc.DeepEquals,
		`juju-resource-lifecycle notin (persistent)`,
	)
}
