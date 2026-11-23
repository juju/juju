// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scriptlet

import (
	"github.com/canonical/starlark/starlark"
	"github.com/juju/tc"
)

func (s *scriptletSuite) TestRegisterEnv(c *tc.C) {
	registry := &EnvRegistry{}
	RegisterEnv(registry)
	c.Check(registry.App().Methods, tc.DeepEquals, []*starlark.Builtin{
		setStatusBuiltin,
	})
}

func (s *scriptletSuite) TestRegisterEnvIndependentRegistries(c *tc.C) {
	first := &EnvRegistry{}
	second := &EnvRegistry{}
	RegisterEnv(first)
	RegisterEnv(second)
	c.Check(first.App().Methods, tc.DeepEquals, second.App().Methods)
}
