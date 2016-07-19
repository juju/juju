// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facade

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/utils/featureflag"
)

// record represents an entry in a Registry.
type record struct {
	factory    Factory
	facadeType reflect.Type
	// If the feature is not the empty string, then this facade
	// is only returned when that feature flag is set.
	//
	// It is not a good thing that we depend on yet another flavour
	// of global in the implementation of the Registry that itself
	// only meaningfully exists as a global.
	feature string
}

// versions is our internal structure for tracking specific versions of a
// single facade. We use a map to be able to quickly lookup a version.
type versions map[int]record

// Registry describes Facades the facades exposed by some API server.
//
// It's only actually used as a global -- `apiserver/common.Facades` --
// but if we were smarter we could just create a Registry directly and
// pass it into the apiserver.
type Registry struct {
	facades map[string]versions
}

// Register adds a single named facade at a given version to the registry.
// Factory will be called when someone wants to instantiate an object of
// this facade, and facadeType defines the concrete type that the returned object will be.
// The Type information is used to define what methods will be exported in the
// API, and it must exactly match the actual object returned by the factory.
func (f *Registry) Register(name string, version int, factory Factory, facadeType reflect.Type, feature string) error {
	if f.facades == nil {
		f.facades = make(map[string]versions, 1)
	}
	record := record{
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

// lookup translates a facade name and version into a record.
func (f *Registry) lookup(name string, version int) (record, error) {
	if versions, ok := f.facades[name]; ok {
		if record, ok := versions[version]; ok {
			if featureflag.Enabled(record.feature) {
				return record, nil
			}
		}
	}
	return record{}, errors.NotFoundf("%s(%d)", name, version)
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
	for version, record := range vers {
		if featureflag.Enabled(record.feature) {
			intVersions = append(intVersions, version)
		}
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
