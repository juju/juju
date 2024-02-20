// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facade

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	"github.com/juju/errors"
)

// record represents an entry in a Registry.
type record struct {
	factory    ModelFactory
	facadeType reflect.Type
}

// versions is our internal structure for tracking specific versions of a
// single facade. We use a map to be able to quickly lookup a version.
type versions map[int]record

// FacadeRegistry describes the API facades exposed by some API server.
type FacadeRegistry interface {
	// MustRegister adds a single named facade at a given version to the
	// registry.
	// Factory will be called when someone wants to instantiate an object of
	// this facade, and facadeType defines the concrete type that the returned
	// object will be.
	// The Type information is used to define what methods will be exported in
	// the API, and it must exactly match the actual object returned by the
	// factory.
	MustRegister(string, int, Factory, reflect.Type)

	// MustRegisterForModel adds a single named facade for a model at a given
	// version to the registry. This allows the facade to be registered with
	// a factory that takes a ModelContext instead of a Context.
	// ModelFactory will be called when someone wants to instantiate an object
	// of this facade, and facadeType defines the concrete type that the
	// returned object will be.
	// The Type information is used to define what methods will be exported in
	// the API, and it must exactly match the actual object returned by the
	// factory.
	MustRegisterForModel(string, int, ModelFactory, reflect.Type)
}

// Registry describes the API facades exposed by some API server.
type Registry struct {
	facades map[string]versions
}

// Register adds a single named facade at a given version to the registry.
// Factory will be called when someone wants to instantiate an object of
// this facade, and facadeType defines the concrete type that the returned
// object will be.
// The Type information is used to define what methods will be exported in the
// API, and it must exactly match the actual object returned by the factory.
func (f *Registry) Register(name string, version int, factory Factory, facadeType reflect.Type) error {
	return f.RegisterForModel(name, version, callFactory(factory), facadeType)
}

// RegisterForModel adds a single named facade at a given version to the
// registry.
func (f *Registry) RegisterForModel(name string, version int, factory ModelFactory, facadeType reflect.Type) error {
	if f.facades == nil {
		f.facades = make(map[string]versions, 1)
	}
	record := record{
		factory:    factory,
		facadeType: facadeType,
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

// MustRegister adds a single named facade at a given version to the registry
// and panics if it fails.
// See: Register.
func (f *Registry) MustRegister(name string, version int, factory Factory, facadeType reflect.Type) {
	if err := f.Register(name, version, factory, facadeType); err != nil {
		panic(err)
	}
}

// MustRegisterForModel adds a single named facade for a model at a given
// version to the registry and panics if it fails.
func (f *Registry) MustRegisterForModel(name string, version int, factory ModelFactory, facadeType reflect.Type) {
	if err := f.RegisterForModel(name, version, factory, facadeType); err != nil {
		panic(err)
	}
}

// lookup translates a facade name and version into a record.
func (f *Registry) lookup(name string, version int) (record, error) {
	if versions, ok := f.facades[name]; ok {
		if record, ok := versions[version]; ok {
			return record, nil
		}
	}
	return record{}, errors.NotFoundf("%s(%d)", name, version)
}

// GetFactory returns just the Factory for a given Facade name and version.
// See also GetType for getting the type information instead of the creation factory.
func (f *Registry) GetFactory(name string, version int) (ModelFactory, error) {
	record, err := f.lookup(name, version)
	if err != nil {
		return nil, err
	}
	return record.factory, nil
}

// GetType returns the type information for a given Facade name and version.
// This can be used for introspection purposes (to determine what methods are
// available, etc).
func (f *Registry) GetType(name string, version int) (reflect.Type, error) {
	record, err := f.lookup(name, version)
	if err != nil {
		return nil, err
	}
	return record.facadeType, nil
}

// Description describes the name and what versions of a facade have been
// registered.
type Description struct {
	Name     string
	Versions []int
}

// descriptionFromVersions aggregates the information in a versions map into a
// more friendly form for List().
func descriptionFromVersions(name string, vers versions) Description {
	intVersions := make([]int, 0, len(vers))
	for version := range vers {
		intVersions = append(intVersions, version)
	}
	sort.Ints(intVersions)
	return Description{
		Name:     name,
		Versions: intVersions,
	}
}

// List returns a slice describing each of the registered Facades.
func (f *Registry) List() []Description {
	names := make([]string, 0, len(f.facades))
	for name := range f.facades {
		names = append(names, name)
	}
	sort.Strings(names)
	descriptions := make([]Description, 0, len(f.facades))
	for _, name := range names {
		facades := f.facades[name]
		description := descriptionFromVersions(name, facades)
		if len(description.Versions) > 0 {
			descriptions = append(descriptions, description)
		}
	}
	return descriptions
}

// Details holds information about a facade.
type Details struct {
	// Name is the name of the facade.
	Name string
	// Version holds the version of the facade.
	Version int
	// Factory holds the factory function for making
	// instances of the facade.
	Factory ModelFactory
	// Type holds the type of object that the Factory
	// will return. This can be used to find out
	// details of the facade without actually creating
	// a facade instance (see rpcreflect.ObjTypeOf).
	Type reflect.Type
}

// ListDetails returns information about all the facades
// registered in f, ordered lexically by name.
func (f *Registry) ListDetails() []Details {
	names := make([]string, 0, len(f.facades))
	for name := range f.facades {
		names = append(names, name)
	}
	sort.Strings(names)
	var details []Details
	for _, name := range names {
		for v, info := range f.facades[name] {
			details = append(details, Details{
				Name:    name,
				Version: v,
				Factory: info.factory,
				Type:    info.facadeType,
			})
		}
	}
	return details
}

// Discard gets rid of a registration that has already been done. Calling
// discard on an entry that is not present is not considered an error.
func (f *Registry) Discard(name string, version int) {
	if versions, ok := f.facades[name]; ok {
		delete(versions, version)
		if len(versions) == 0 {
			delete(f.facades, name)
		}
	}
}

func callFactory(factory Factory) ModelFactory {
	return func(stdCtx context.Context, facadeCtx ModelContext) (Facade, error) {
		return factory(stdCtx, facadeCtx)
	}
}
