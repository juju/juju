// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"reflect"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

// Facades is the registry that tracks all of the Facades that will be
// exposed in the API. It can be used to List/Get/Register facades.
//
// Most implementers of a facade will probably want to use
// RegisterStandardFacade rather than Facades.Register, as it provides
// much cleaner syntax and semantics for calling during init().
var facades = &facade.Registry{}

// GetFacades is temporary and will go away soon. It should only be
// called by the apiserver.
func GetFacades() *facade.Registry {
	return facades
}

// RegisterFacade updates the global facade registry with a new version of a new type.
func RegisterFacade(name string, version int, factory facade.Factory, facadeType reflect.Type) {
	RegisterFacadeForFeature(name, version, factory, facadeType, "")
}

// RegisterFacadeForFeature updates the global facade registry with a new
// version of a new type. If the feature is non-empty, this facade is only
// available when the specified feature flag is set.
func RegisterFacadeForFeature(name string, version int, factory facade.Factory, facadeType reflect.Type, feature string) {
	err := facades.Register(name, version, factory, facadeType, feature)
	if err != nil {
		// This is meant to be called during init() so errors should be
		// considered fatal.
		panic(err)
	}
	logger.Tracef("Registered facade %q v%d", name, version)
}

// NewHookContextFacadeFn specifies the function signature that can be
// used to register a hook context facade.
type NewHookContextFacadeFn func(*state.State, *state.Unit) (interface{}, error)

// RegisterHookContextFacade registers facades for use within a hook
// context. This function handles the translation from a
// hook-context-facade to a standard facade so the caller's factory
// method can elide unnecessary arguments. This function also handles
// any necessary authorization for the client.
//
// XXX(fwereade): this is fundamentally broken, because it (1)
// arbitrarily creates a new facade for a tiny fragment of a specific
// client worker's reponsibilities and (2) actively conceals necessary
// auth information from the facade. Don't call it; actively work to
// delete code that uses it, and rewrite it properly.
func RegisterHookContextFacade(name string, version int, newHookContextFacade NewHookContextFacadeFn, facadeType reflect.Type) {

	newFacade := func(context facade.Context) (facade.Facade, error) {
		authorizer := context.Auth()
		st := context.State()

		if !authorizer.AuthUnitAgent() {
			return nil, ErrPerm
		}

		// Verify that the unit's ID matches a unit that we know
		// about.
		tag := authorizer.GetAuthTag()
		if _, ok := tag.(names.UnitTag); !ok {
			return nil, errors.Errorf("expected names.UnitTag, got %T", tag)
		}

		unit, err := st.Unit(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}

		return newHookContextFacade(st, unit)
	}

	facades.Register(name, version, newFacade, facadeType, "")
}

// RegisterStandardFacade registers a factory function for a normal New* style
// function. This requires that the function has the form:
// NewFoo(*state.State, facade.Resources, facade.Authorizer) (*Type, error)
// With that syntax, we will create a helper function that wraps calling NewFoo
// with the right parameters, and returns the *Type correctly.
func RegisterStandardFacade(name string, version int, newFunc interface{}) {
	RegisterStandardFacadeForFeature(name, version, newFunc, "")
}

// RegisterStandardFacadeForFeature registers a factory function for a normal
// New* style function. This requires that the function has the form:
// NewFoo(*state.State, facade.Resources, facade.Authorizer) (*Type, error)
// With that syntax, we will create a helper function that wraps calling
// NewFoo with the right parameters, and returns the *Type correctly. If the
// feature is non-empty, this facade is only available when the specified
// feature flag is set.
func RegisterStandardFacadeForFeature(name string, version int, newFunc interface{}, feature string) {
	facades.RegisterStandard(name, version, newFunc, feature)
}
