// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/provider/kubernetes"
	"github.com/juju/juju/internal/testing"
)

type initSuite struct {
	testing.BaseSuite
}

func TestInitSuite(t *stdtesting.T) {
	tc.Run(t, &initSuite{})
}

func (s *initSuite) TestLabelSelectorGlobalResourcesLifecycle(c *tc.C) {
	c.Assert(
		kubernetes.CompileLifecycleModelTeardownSelector().String(), tc.DeepEquals,
		`juju-resource-lifecycle notin (persistent)`,
	)
}
