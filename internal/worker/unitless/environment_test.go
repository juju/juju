// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
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
	app := globalEnv().App()
	names := make([]string, 0, len(app.Methods))
	for _, method := range app.Methods {
		names = append(names, method.Name())
	}
	c.Check(app.Name, tc.Equals, "juju")
	c.Check(names, tc.DeepEquals, []string{"set_status"})
}

func (s *environmentSuite) TestGlobalEnvReturnsSameRegistry(c *tc.C) {
	first := globalEnv()
	second := globalEnv()
	c.Check(first, tc.NotNil)
	c.Check(second, tc.Equals, first)
}
