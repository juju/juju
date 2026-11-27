// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scriptlet

import (
	"fmt"
	"reflect"
	"sync/atomic"

	"github.com/canonical/starform/starform"
	"github.com/canonical/starlark/starlark"
)

// RegisterEnv registers the common scriptlet environment features.
func RegisterEnv(env *EnvRegistry) {
	env.AddAppMethod(setStatusBuiltin)
	env.AddIntentCollector(StatusCollector{})
}

// EnvRegistry holds declarations which define the scriptlet environment.
// It is frozen by its first App or IntentCollectors call.
type EnvRegistry struct {
	frozen               atomic.Bool
	jujuMethods          []*starlark.Builtin
	jujuMethodNames      map[string]struct{}
	intentCollectorFuncs []func() any
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

// AddIntentCollector registers the given zero-value struct collector,
// instantiated fresh for each event.
func (er *EnvRegistry) AddIntentCollector(collector any) {
	collectorValue := reflect.ValueOf(collector)
	if collectorValue.Kind() != reflect.Struct {
		panic("internal error: intent collector must be a struct")
	}
	// Forbid sharing.
	if !collectorValue.IsZero() {
		panic("internal error: intent collector must have zero value")
	}
	er.AddIntentCollectorFunc(func() any {
		return reflect.New(collectorValue.Type()).Interface()
	})
}

// AddIntentCollectorFunc registers the given collector constructor.
func (er *EnvRegistry) AddIntentCollectorFunc(collectorFunc func() any) {
	if er.frozen.Load() {
		panic("internal error: environment frozen")
	}

	er.intentCollectorFuncs = append(er.intentCollectorFuncs, collectorFunc)
}

// App constructs an app object and freezes this registry.
func (er *EnvRegistry) App() *starform.AppObject {
	er.frozen.Store(true)

	return &starform.AppObject{
		Name:    "juju",
		Methods: er.jujuMethods,
	}
}

// IntentCollectors returns fresh collectors, freezing this registry.
func (er *EnvRegistry) IntentCollectors() *IntentCollectors {
	er.frozen.Store(true)

	ret := &IntentCollectors{}
	for _, intentCollectorFunc := range er.intentCollectorFuncs {
		ret.add(intentCollectorFunc())
	}
	return ret
}
