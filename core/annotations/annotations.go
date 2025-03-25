// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
	"fmt"

	"github.com/juju/names/v6"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// Kind identifies different kinds of entities that'll get associated with
// annotations.
type Kind int

const (
	KindApplication Kind = 1
	KindMachine     Kind = 2
	KindUnit        Kind = 3
	KindModel       Kind = 4
	KindStorage     Kind = 5
)

// ID reifies annotatable GlobalEntities into an internal representation using
// annotations.Kind.
type ID struct {
	Kind Kind
	Name string
}

// Validate checks if the ID is valid or not.
func (i ID) Validate() error {
	if i.Name == "" {
		return errors.Errorf("name cannot be empty %w", coreerrors.NotValid)
	}

	switch i.Kind {
	case KindApplication, KindMachine, KindUnit, KindModel, KindStorage:
		return nil
	default:
		return errors.Errorf("unknown kind %d %w", i.Kind, coreerrors.NotValid)
	}
}

// ConvertTagToID converts the names.Tag into an ID for different names.Kinds
// of entities, registering them as annotations.Kinds of entities.
func ConvertTagToID(n names.Tag) (ID, error) {
	switch n.Kind() {
	case names.MachineTagKind:
		return ID{
			Kind: KindMachine,
			Name: n.Id(),
		}, nil
	case names.UnitTagKind:
		return ID{
			Kind: KindUnit,
			Name: n.Id(),
		}, nil
	case names.ModelTagKind:
		return ID{
			Kind: KindModel,
			Name: n.Id(),
		}, nil
	case names.StorageTagKind:
		return ID{
			Kind: KindStorage,
			Name: n.Id(),
		}, nil
	case names.ApplicationTagKind:
		return ID{
			Kind: KindApplication,
			Name: n.Id(),
		}, nil
	default:
		return ID{}, errors.Errorf("unknown kind %q", n.Kind())
	}
}

func (i ID) String() string {
	return fmt.Sprintf("%d/%s", i.Kind, i.Name)
}

// Annotation extends k8s annotation map.
type Annotation map[string]string

// New contructs an annotation.
func New(as map[string]string) Annotation {
	newA := Annotation{}
	if as == nil {
		return newA
	}
	for k, v := range as {
		newA.Add(k, v)
	}
	return newA
}

// Has checks if the provided key value pair exists in this annotation or not.
func (a Annotation) Has(key, expectedValue string) bool {
	v, ok := a.getVal(key)
	return ok && v == expectedValue
}

// HasAll checks if all the provided key value pairs exist in this annotation
// or not.
func (a Annotation) HasAll(expected map[string]string) bool {
	for k, v := range expected {
		if !a.Has(k, v) {
			return false
		}
	}
	return true
}

// HasAny checks if any provided key value pairs exists in this annotation or
// not.
func (a Annotation) HasAny(expected map[string]string) bool {
	for k, v := range expected {
		if a.Has(k, v) {
			return true
		}
	}
	return false
}

// Add inserts a new key value pair.
func (a Annotation) Add(key, value string) Annotation {
	a.setVal(key, value)
	return a
}

// Remove deletes the key and its value from the annotation.
func (a Annotation) Remove(key string) Annotation {
	delete(a, key)
	return a
}

// Merge merges an annotation with current one.
func (a Annotation) Merge(as Annotation) Annotation {
	for k, v := range as {
		a.Add(k, v)
	}
	return a
}

// ToMap returns the map format of the annotation.
func (a Annotation) ToMap() map[string]string {
	out := make(map[string]string)
	for k, v := range a {
		out[k] = v
	}
	return out
}

// Copy returns a copy of current annotation.
func (a Annotation) Copy() Annotation {
	return New(nil).Merge(a)
}

// CheckKeysNonEmpty checks if the provided keys are all set to non empty value.
func (a Annotation) CheckKeysNonEmpty(keys ...string) error {
	for _, k := range keys {
		v, ok := a.getVal(k)
		if !ok {
			return errors.Errorf("annotation key %q %w", k, coreerrors.NotFound)
		}
		if v == "" {
			return errors.Errorf("annotation key %q has empty value %w", k, coreerrors.NotValid)
		}
	}
	return nil
}

// getVal returns the value for the specified key and also indicates if it
// exists.
func (a Annotation) getVal(key string) (string, bool) {
	v, ok := a[key]
	return v, ok
}

func (a Annotation) setVal(key, val string) {
	oldVal, existing := a.getVal(key)
	if existing && oldVal == val {
		return
	}
	a[key] = val
}
