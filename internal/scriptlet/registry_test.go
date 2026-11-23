// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scriptlet

import (
	"fmt"

	"github.com/canonical/starlark/starlark"
	"github.com/juju/tc"
)

func (s *scriptletSuite) TestRegistryApp(c *tc.C) {
	registry := &EnvRegistry{}
	registry.AddAppMethod(builtin)

	app := registry.App()
	c.Check(app.Name, tc.Equals, "juju")
	c.Check(app.Methods, tc.DeepEquals, []*starlark.Builtin{builtin})

	// Later additions rejected once an app object has been exposed.
	c.Check(func() {
		registry.AddAppMethod(builtin)
	}, tc.PanicMatches, ".*frozen.*")
}

func (s *scriptletSuite) TestRegistryRejectsSameNameBuiltins(c *tc.C) {
	registry := &EnvRegistry{}
	registry.AddAppMethod(builtin)

	expected := fmt.Sprintf(".*%s.*", builtin.Name())
	c.Check(func() {
		registry.AddAppMethod(sameNameBuiltin)
	}, tc.PanicMatches, expected)
}

var builtin = starlark.NewBuiltinWithSafety("builtin", starlark.NotSafe, func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	return starlark.None, nil
})

var sameNameBuiltin = starlark.NewBuiltinWithSafety("builtin", starlark.NotSafe, func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	return starlark.None, nil
})
