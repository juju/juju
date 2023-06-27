// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facade

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/juju/errors"
)

// record represents an entry in a Registry.
type record struct {
	allower    Allower
	factory    Factory
	facadeType reflect.Type
}

// versions is our internal structure for tracking specific versions of a
// single facade. We use a map to be able to quickly lookup a version.
type versions map[int]record

// Allower is a function that can be used to check if a connection is allowed to
// access a given facade.
type Allower func(Authorizer) error

// AlwaysAllowed is an Allower that always returns nil, which means it's always
// allowed.
func AlwaysAllowed(a Authorizer) error {
	return nil
}

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

	MustRegisterWithAuth(string, int, Allower, Factory, reflect.Type)
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
func (f *Registry) Register(name string, version int, allower Allower, factory Factory, facadeType reflect.Type) error {
	if f.facades == nil {
		f.facades = make(map[string]versions, 1)
	}
	record := record{
		allower:    allower,
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
	if err := f.Register(name, version, nil, factory, facadeType); err != nil {
		panic(err)
	}
}

// MustRegister adds a single named facade at a given version to the registry
// and panics if it fails.
// See: Register.
func (f *Registry) MustRegisterWithAuth(name string, version int, allower Allower, factory Factory, facadeType reflect.Type) {
	if err := f.Register(name, version, allower, factory, facadeType); err != nil {
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

// GetAllower returns just the Allower for a given Facade name and version.
// See also GetType for getting the type information instead of the creation factory.
func (f *Registry) GetAllower(name string, version int) (Allower, error) {
	record, err := f.lookup(name, version)
	if err != nil {
		return nil, err
	}
	return record.allower, nil
}

// GetFactory returns just the Factory for a given Facade name and version.
// See also GetType for getting the type information instead of the creation factory.
func (f *Registry) GetFactory(name string, version int) (Factory, error) {
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
	Factory Factory
	// Allower holds the function that can be used to
	// check if a connection is allowed to access the
	// facade.
	Allower Allower
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
				Allower: info.allower,
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
