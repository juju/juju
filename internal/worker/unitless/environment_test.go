// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"slices"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type environmentSuite struct {
	coretesting.BaseSuite
}

func TestEnvironmentSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &environmentSuite{})
	})
}

func (s *environmentSuite) TestGlobalEnvContainsCommonBuiltins(c *tc.C) {
	registry := globalEnv()
	app := registry.App()
	names := make([]string, 0, len(app.Methods))
	for _, method := range app.Methods {
		names = append(names, method.Name())
	}
	c.Check(app.Name, tc.Equals, "juju")

	// One representative builtin per package contributing RegisterEnv.
	c.Check(slices.Contains(names, "set_status"), tc.IsTrue)
}

func (s *environmentSuite) TestGlobalEnvReturnsSameRegistry(c *tc.C) {
	first := globalEnv()
	second := globalEnv()
	c.Check(first, tc.NotNil)
	c.Check(second, tc.Equals, first)
}
