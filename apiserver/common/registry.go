// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"reflect"
	"runtime"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/state"
)

// FacadeFactory represent a way of creating a Facade from the current
// connection to the State.
type FacadeFactory func(
	st *state.State, resources *Resources, authorizer Authorizer, id string,
) (
	interface{}, error,
)

type facadeRecord struct {
	factory    FacadeFactory
	facadeType reflect.Type
	// If the feature is not the empty string, then this facade
	// is only returned when that feature flag is set.
	feature string
}

// RegisterFacade updates the global facade registry with a new version of a new type.
func RegisterFacade(name string, version int, factory FacadeFactory, facadeType reflect.Type) {
	RegisterFacadeForFeature(name, version, factory, facadeType, "")
}

// RegisterFacadeForFeature updates the global facade registry with a new
// version of a new type. If the feature is non-empty, this facade is only
// available when the specified feature flag is set.
func RegisterFacadeForFeature(name string, version int, factory FacadeFactory, facadeType reflect.Type, feature string) {
	err := Facades.Register(name, version, factory, facadeType, feature)
	if err != nil {
		// This is meant to be called during init() so errors should be
		// considered fatal.
		panic(err)
	}
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
	facadeType := reflect.TypeOf((*FacadeFactory)(nil)).Elem()
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
		return fmt.Errorf("function %q does not have the signature func (*state.State, *common.Resources, common.Authorizer) (*Type, error)",
			funcName)
	}
	return nil
}

// wrapNewFacade turns a given NewFoo(st, resources, authorizer) (*Instance, error)
// function and wraps it into a proper FacadeFactory function.
func wrapNewFacade(newFunc interface{}) (FacadeFactory, reflect.Type, error) {
	funcValue := reflect.ValueOf(newFunc)
	err := validateNewFacade(funcValue)
	if err != nil {
		return nil, reflect.TypeOf(nil), err
	}
	// So we know newFunc is a func with the right args in and out, so
	// wrap it into a helper function that matches the FacadeFactory.
	wrapped := func(
		st *state.State, resources *Resources, auth Authorizer, id string,
	) (
		interface{}, error,
	) {
		if id != "" {
			return nil, ErrBadId
		}
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

// RegisterStandardFacade registers a factory function for a normal New* style
// function. This requires that the function has the form:
// NewFoo(*state.State, *common.Resources, common.Authorizer) (*Type, error)
// With that syntax, we will create a helper function that wraps calling NewFoo
// with the right parameters, and returns the *Type correctly.
func RegisterStandardFacade(name string, version int, newFunc interface{}) {
	RegisterStandardFacadeForFeature(name, version, newFunc, "")
}

// RegisterStandardFacadeForFeature registers a factory function for a normal
// New* style function. This requires that the function has the form:
// NewFoo(*state.State, *common.Resources, common.Authorizer) (*Type, error)
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

// Facades is the registry that tracks all of the Facades that will be exposed in the API.
// It can be used to List/Get/Register facades.
// Most implementers of a facade will probably want to use
// RegisterStandardFacade rather than Facades.Register, as it provides much
// cleaner syntax and semantics for calling during init().
var Facades = &FacadeRegistry{}

// versions is our internal structure for tracking specific versions of a
// single facade. We use a map to be able to quickly lookup a version.
type versions map[int]facadeRecord

// FacadeRegistry is responsible for tracking what Facades are going to be exported in the API.
// See the variable "Facades" for the singleton that tracks them.
// It would be possible to have multiple registries if we decide to change how
// the API exposes methods based on Login information.
type FacadeRegistry struct {
	facades map[string]versions
}

// Register adds a single named facade at a given version to the registry.
// FacadeFactory will be called when someone wants to instantiate an object of
// this facade, and facadeType defines the concrete type that the returned object will be.
// The Type information is used to define what methods will be exported in the
// API, and it must exactly match the actual object returned by the factory.
func (f *FacadeRegistry) Register(name string, version int, factory FacadeFactory, facadeType reflect.Type, feature string) error {
	if f.facades == nil {
		f.facades = make(map[string]versions, 1)
	}
	record := facadeRecord{
		factory:    factory,
		facadeType: facadeType,
		feature:    feature,
	}
	if vers, ok := f.facades[name]; ok {
		if _, ok := vers[version]; ok {
			fullname := fmt.Sprintf("%s(%d)", name, version)
			return fmt.Errorf("object %q already registered", fullname)
		}
		vers[version] = record
	} else {
		f.facades[name] = versions{version: record}
	}
	return nil
}

// lookup translates a facade name and version into a facadeRecord.
func (f *FacadeRegistry) lookup(name string, version int) (facadeRecord, error) {
	if versions, ok := f.facades[name]; ok {
		if record, ok := versions[version]; ok {
			if featureflag.Enabled(record.feature) {
				return record, nil
			}
		}
	}
	return facadeRecord{}, errors.NotFoundf("%s(%d)", name, version)
}

// GetFactory returns just the FacadeFactory for a given Facade name and version.
// See also GetType for getting the type information instead of the creation factory.
func (f *FacadeRegistry) GetFactory(name string, version int) (FacadeFactory, error) {
	record, err := f.lookup(name, version)
	if err != nil {
		return nil, err
	}
	return record.factory, nil
}

// GetType returns the type information for a given Facade name and version.
// This can be used for introspection purposes (to determine what methods are
// available, etc).
func (f *FacadeRegistry) GetType(name string, version int) (reflect.Type, error) {
	record, err := f.lookup(name, version)
	if err != nil {
		return nil, err
	}
	return record.facadeType, nil
}

// FacadeDescription describes the name and what versions of a facade have been
// registered.
type FacadeDescription struct {
	Name     string
	Versions []int
}

// descriptionFromVersions aggregates the information in a versions map into a
// more friendly form for List().
func descriptionFromVersions(name string, vers versions) FacadeDescription {
	intVersions := make([]int, 0, len(vers))
	for version, record := range vers {
		if featureflag.Enabled(record.feature) {
			intVersions = append(intVersions, version)
		}
	}
	sort.Ints(intVersions)
	return FacadeDescription{
		Name:     name,
		Versions: intVersions,
	}
}

// List returns a slice describing each of the registered Facades.
func (f *FacadeRegistry) List() []FacadeDescription {
	names := make([]string, 0, len(f.facades))
	for name := range f.facades {
		names = append(names, name)
	}
	sort.Strings(names)
	descriptions := make([]FacadeDescription, 0, len(f.facades))
	for _, name := range names {
		facades := f.facades[name]
		description := descriptionFromVersions(name, facades)
		if len(description.Versions) > 0 {
			descriptions = append(descriptions, description)
		}
	}
	return descriptions
}

// Discard gets rid of a registration that has already been done. Calling
// discard on an entry that is not present is not considered an error.
func (f *FacadeRegistry) Discard(name string, version int) {
	if versions, ok := f.facades[name]; ok {
		delete(versions, version)
		if len(versions) == 0 {
			delete(f.facades, name)
		}
	}
}
