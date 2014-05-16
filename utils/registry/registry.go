// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/juju/errors"
)

// TypedNameVersion is a registry that will allow you to register objects based
// on a name and version pair. The objects must be convertible to the Type
// defined when the registry was created. It will be cast during Register so
// you can be sure all objects returned from Get() are safe to TypeAssert to
// that type.
type TypedNameVersion struct {
	requiredType reflect.Type
	versions     map[string]Versions
}

// NewTypedNameVersion creates a place to register your objects
func NewTypedNameVersion(requiredType reflect.Type) *TypedNameVersion {
	return &TypedNameVersion{
		requiredType: requiredType,
		versions:     make(map[string]Versions),
	}
}

// Description gives the name and available versions in a registry.
type Description struct {
	Name     string
	Versions []int
}

// Versions maps concrete versions of the objects.
type Versions map[int]interface{}

// Register records the factory that can be used to produce an instance of the
// facade at the supplied version.
// If the object being registered doesn't Implement the required Type, then an
// error is returned.
// An error is also returned if an object is already registered with the given
// name and version.
func (r *TypedNameVersion) Register(name string, version int, obj interface{}) error {
	if !reflect.TypeOf(obj).ConvertibleTo(r.requiredType) {
		return fmt.Errorf("object of type %T cannot be converted to type %s.%s", obj, r.requiredType.PkgPath(), r.requiredType.Name())
	}
	obj = reflect.ValueOf(obj).Convert(r.requiredType).Interface()
	if r.versions == nil {
		r.versions = make(map[string]Versions, 1)
	}
	if versions, ok := r.versions[name]; ok {
		if _, ok := versions[version]; ok {
			fullname := fmt.Sprintf("%s(%d)", name, version)
			return fmt.Errorf("object %q already registered", fullname)
		}
		versions[version] = obj
	} else {
		r.versions[name] = Versions{version: obj}
	}
	return nil
}

// descriptionFromVersions aggregates the information in a Versions map into a
// more friendly form for List()
func descriptionFromVersions(name string, versions Versions) Description {
	intVersions := make([]int, 0, len(versions))
	for version := range versions {
		intVersions = append(intVersions, version)
	}
	sort.Ints(intVersions)
	return Description{
		Name:     name,
		Versions: intVersions,
	}
}

// List returns a slice describing each of the registered Facades.
func (r *TypedNameVersion) List() []Description {
	names := make([]string, 0, len(r.versions))
	for name := range r.versions {
		names = append(names, name)
	}
	sort.Strings(names)
	descriptions := make([]Description, len(r.versions))
	for i, name := range names {
		versions := r.versions[name]
		descriptions[i] = descriptionFromVersions(name, versions)
	}
	return descriptions
}

// Get returns the object for a single name and version. If the requested
// facade is not found, it returns error.NotFound
func (r *TypedNameVersion) Get(name string, version int) (interface{}, error) {
	if versions, ok := r.versions[name]; ok {
		if factory, ok := versions[version]; ok {
			return factory, nil
		}
	}
	return nil, errors.NotFoundf("%s(%d)", name, version)
}
