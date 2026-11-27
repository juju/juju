// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scriptlet

import (
	"fmt"

	"github.com/canonical/starlark/starlark"
	"github.com/juju/tc"
)

func (s *scriptletSuite) TestRegisterEnv(c *tc.C) {
	registry := &EnvRegistry{}
	RegisterEnv(registry)

	var names []string
	for _, method := range registry.App().Methods {
		names = append(names, method.Name())
	}
	c.Check(names, tc.DeepEquals, []string{"set_status"})
}

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

func (s *scriptletSuite) TestRegistryIntentCollectors(c *tc.C) {
	registry := &EnvRegistry{}
	registry.AddIntentCollector(StatusCollector{})

	collectors := registry.IntentCollectors()
	statusCollector := IntentCollector[*StatusCollector](collectors)
	statusCollector.Status = "active"

	// Each call yields fresh, independent collectors.
	c.Check(IntentCollector[*StatusCollector](registry.IntentCollectors()), tc.DeepEquals, &StatusCollector{})
}

func (s *scriptletSuite) TestRegistryAddIntentCollectorFunc(c *tc.C) {
	registry := &EnvRegistry{}
	id := 0
	registry.AddIntentCollectorFunc(func() any {
		id++
		return &testCollector{id: id}
	})
	registry.AddIntentCollector(StatusCollector{})

	firstCollectors := registry.IntentCollectors()
	secondCollectors := registry.IntentCollectors()

	// The constructor is called once per event, its result is
	// registered, and both collector types are retrievable.
	c.Check(IntentCollector[*testCollector](firstCollectors), tc.DeepEquals, &testCollector{id: 1})
	c.Check(IntentCollector[*testCollector](secondCollectors), tc.DeepEquals, &testCollector{id: 2})
	c.Check(IntentCollector[*StatusCollector](firstCollectors), tc.NotNil)
	c.Check(IntentCollector[*StatusCollector](secondCollectors), tc.NotNil)
}

func (s *scriptletSuite) TestIntentCollectorDuplicatePanics(c *tc.C) {
	registry := &EnvRegistry{}
	registry.AddIntentCollector(testCollector{})
	registry.AddIntentCollectorFunc(func() any { return &testCollector{} })

	c.Check(func() { registry.IntentCollectors() },
		tc.PanicMatches, `intent collector already registered \*scriptlet\.testCollector`)
}

// TestIntentCollectorsFreezes asserts that the first IntentCollectors
// call freezes the registry against further collector registration
// through either registration method.
func (s *scriptletSuite) TestIntentCollectorsFreezes(c *tc.C) {
	registry := &EnvRegistry{}
	registry.IntentCollectors()

	c.Check(func() {
		registry.AddIntentCollector(testCollector{})
	}, tc.PanicMatches, ".*frozen.*")
	c.Check(func() {
		registry.AddIntentCollectorFunc(func() any { return &testCollector{} })
	}, tc.PanicMatches, ".*frozen.*")
}

func (s *scriptletSuite) TestRegistryAddIntentCollectorPanics(c *tc.C) {
	registry := &EnvRegistry{}
	c.Check(func() { registry.AddIntentCollector(&StatusCollector{}) },
		tc.PanicMatches, ".*must be a struct.*")
	c.Check(func() { registry.AddIntentCollector(StatusCollector{Status: "active"}) },
		tc.PanicMatches, ".*must have zero value.*")
}

type testCollector struct {
	id int
}

var builtin = starlark.NewBuiltinWithSafety("builtin", starlark.NotSafe, func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	return starlark.None, nil
})

var sameNameBuiltin = starlark.NewBuiltinWithSafety("builtin", starlark.NotSafe, func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	return starlark.None, nil
})
