// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common/apihttp"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

// Facades is the registry that tracks all of the Facades that will be
// exposed in the API. It can be used to List/Get/Register facades.
//
// Most implementers of a facade will probably want to use
// RegisterStandardFacade rather than Facades.Register, as it provides
// much cleaner syntax and semantics for calling during init().
//
// Developers in a happy future will not want to use Facades at all,
// eschewing globals in favour of explicitly building apis and supplying
// them directly to an apiserver.
var Facades = &facade.Registry{}

// RegisterFacade updates the global facade registry with a new version of a new type.
func RegisterFacade(name string, version int, factory facade.Factory, facadeType reflect.Type) {
	RegisterFacadeForFeature(name, version, factory, facadeType, "")
}

// RegisterFacadeForFeature updates the global facade registry with a new
// version of a new type. If the feature is non-empty, this facade is only
// available when the specified feature flag is set.
func RegisterFacadeForFeature(name string, version int, factory facade.Factory, facadeType reflect.Type, feature string) {
	err := Facades.Register(name, version, factory, facadeType, feature)
	if err != nil {
		// This is meant to be called during init() so errors should be
		// considered fatal.
		panic(err)
	}
	logger.Tracef("Registered facade %q v%d", name, version)
}

// validateNewFacade ensures that the facade factory we have has the right
// input and output parameters for being used as a NewFoo function.
func validateNewFacade(funcValue reflect.Value) error {
	if !funcValue.IsValid() {
		return fmt.Errorf("cannot wrap nil")
	}
	if funcValue.Kind() != reflect.Func {
		return fmt.Errorf("wrong type %q is not a function", funcValue.Kind())
	}
	funcType := funcValue.Type()
	funcName := runtime.FuncForPC(funcValue.Pointer()).Name()
	if funcType.NumIn() != 3 || funcType.NumOut() != 2 {
		return fmt.Errorf("function %q does not take 3 parameters and return 2",
			funcName)
	}

	type nastyFactory func(
		st *state.State,
		resources facade.Resources,
		authorizer facade.Authorizer,
	) (
		interface{}, error,
	)
	facadeType := reflect.TypeOf((*nastyFactory)(nil)).Elem()
	isSame := true
	for i := 0; i < 3; i++ {
		if funcType.In(i) != facadeType.In(i) {
			isSame = false
			break
		}
	}
	if funcType.Out(1) != facadeType.Out(1) {
		isSame = false
	}
	if !isSame {
		return fmt.Errorf("function %q does not have the signature func (*state.State, facade.Resources, facade.Authorizer) (*Type, error)",
			funcName)
	}
	return nil
}

// wrapNewFacade turns a given NewFoo(st, resources, authorizer) (*Instance, error)
// function and wraps it into a proper facade.Factory function.
func wrapNewFacade(newFunc interface{}) (facade.Factory, reflect.Type, error) {
	funcValue := reflect.ValueOf(newFunc)
	err := validateNewFacade(funcValue)
	if err != nil {
		return nil, reflect.TypeOf(nil), err
	}
	// So we know newFunc is a func with the right args in and out, so
	// wrap it into a helper function that matches the facade.Factory.
	wrapped := func(context facade.Context) (facade.Facade, error) {
		if context.ID() != "" {
			return nil, ErrBadId
		}
		st := context.State()
		auth := context.Auth()
		resources := context.Resources()
		// st, resources, or auth is nil, then reflect.Call dies
		// because reflect.ValueOf(anynil) is the Zero Value.
		// So we use &obj.Elem() which gives us a concrete Value object
		// that can refer to nil.
		in := []reflect.Value{
			reflect.ValueOf(&st).Elem(),
			reflect.ValueOf(&resources).Elem(),
			reflect.ValueOf(&auth).Elem(),
		}
		out := funcValue.Call(in)
		if out[1].Interface() != nil {
			err := out[1].Interface().(error)
			return nil, err
		}
		return out[0].Interface(), nil
	}
	return wrapped, funcValue.Type().Out(0), nil
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

	RegisterFacade(name, version, newFacade, facadeType)
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
	wrapped, facadeType, err := wrapNewFacade(newFunc)
	if err != nil {
		panic(err)
	}
	RegisterFacadeForFeature(name, version, wrapped, facadeType, feature)
}

var endpointRegistry = map[string]apihttp.HandlerSpec{}
var endpointRegistryOrder []string

// RegisterAPIModelEndpoint adds the provided endpoint to the registry.
// The pattern is prefixed with the model pattern: /model/:modeluuid.
func RegisterAPIModelEndpoint(pattern string, spec apihttp.HandlerSpec) error {
	if !strings.HasPrefix(pattern, "/") {
		pattern = "/" + pattern
	}
	pattern = "/model/:modeluuid" + pattern
	return registerAPIEndpoint(pattern, spec)
}

func registerAPIEndpoint(pattern string, spec apihttp.HandlerSpec) error {
	if _, ok := endpointRegistry[pattern]; ok {
		return errors.NewAlreadyExists(nil, fmt.Sprintf("endpoint %q already registered", pattern))
	}
	endpointRegistry[pattern] = spec
	endpointRegistryOrder = append(endpointRegistryOrder, pattern)
	return nil
}

// DefaultHTTPMethods are the HTTP methods supported by default by the API.
var DefaultHTTPMethods = []string{"GET", "POST", "HEAD", "PUT", "DEL", "OPTIONS"}

// ResolveAPIEndpoints builds the set of endpoint handlers for all
// registered API endpoints.
func ResolveAPIEndpoints(newArgs func(apihttp.HandlerConstraints) apihttp.NewHandlerArgs) []apihttp.Endpoint {
	var endpoints []apihttp.Endpoint
	for _, pattern := range endpointRegistryOrder {
		spec := endpointRegistry[pattern]
		args := newArgs(spec.Constraints)
		handler := spec.NewHandler(args)
		for _, method := range DefaultHTTPMethods {
			endpoints = append(endpoints, apihttp.Endpoint{
				Pattern: pattern,
				Method:  method,
				Handler: handler,
			})
		}
	}
	return endpoints
}
