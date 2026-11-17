// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scriptlet

import (
	"fmt"
	"sync/atomic"

	"github.com/canonical/starform/starform"
	"github.com/canonical/starlark/starlark"
)

// GlobalEnvRegistry returns the shared environment registry for Juju
// scriptlets.
func GlobalEnvRegistry() *EnvRegistry {
	return globalEnvRegistry
}

var globalEnvRegistry = &EnvRegistry{}

// EnvRegistry holds declarations which define the scriptlet environment.
type EnvRegistry struct {
	frozen          atomic.Bool
	jujuMethods     []*starlark.Builtin
	jujuMethodNames map[string]struct{}
}

// AddAppMethod registers the given app method. It is not thread-safe and
// cannot be called on a frozen registry.
func (er *EnvRegistry) AddAppMethod(method *starlark.Builtin) {
	if er.frozen.Load() {
		panic("internal error: environment frozen")
	}

	if er.jujuMethodNames == nil {
		er.jujuMethodNames = make(map[string]struct{})
	}

	name := method.Name()
	if _, ok := er.jujuMethodNames[name]; ok {
		panic(fmt.Sprintf("internal error: method '%s' added twice", name))
	}
	er.jujuMethodNames[name] = struct{}{}
	er.jujuMethods = append(er.jujuMethods, method)
}

func (er *EnvRegistry) Freeze() {
	er.frozen.Store(true)
}

// App constructs an app object and freezes this registry.
func (er *EnvRegistry) App() *starform.AppObject {
	er.Freeze()

	methods := make([]*starlark.Builtin, len(er.jujuMethods))
	copy(methods, er.jujuMethods)

	return &starform.AppObject{
		Name:    "juju",
		Methods: methods,
	}
}
