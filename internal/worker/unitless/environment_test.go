// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"sync"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/scriptlet"
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

func (s *environmentSuite) TestGlobalEnvConcurrentCallsShareRegistry(c *tc.C) {
	const callers = 16
	registries := make([]*scriptlet.EnvRegistry, callers)
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(callers)
	for i := range callers {
		go func() {
			defer workers.Done()
			<-start
			registries[i] = globalEnv()
		}()
	}
	close(start)
	workers.Wait()

	c.Assert(registries[0], tc.NotNil)
	for _, registry := range registries[1:] {
		c.Check(registry, tc.Equals, registries[0])
	}
}
