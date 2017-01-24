// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/firewaller"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestManifoldFirewallModeNone(c *gc.C) {
	ctx := &mockDependencyContext{
		env: &mockEnviron{
			config: coretesting.CustomModelConfig(c, coretesting.Attrs{
				"firewall-mode": config.FwNone,
			}),
		},
	}

	manifold := firewaller.Manifold(firewaller.ManifoldConfig{
		APICallerName: "api-caller",
		EnvironName:   "environ",
	})
	_, err := manifold.Start(ctx)
	c.Assert(err, gc.Equals, dependency.ErrUninstall)
}

type mockDependencyContext struct {
	dependency.Context
	env *mockEnviron
}

func (m *mockDependencyContext) Get(name string, out interface{}) error {
	if name == "environ" {
		*(out.(*environs.Environ)) = m.env
	}
	return nil
}

type mockEnviron struct {
	environs.Environ
	config *config.Config
}

func (e *mockEnviron) Config() *config.Config {
	return e.config
}
