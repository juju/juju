// Copyright 2014-2018 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"sort"

	"github.com/juju/errors"
)

// Set represents the Set data structure, and contains Tags.
type Set map[Tag]bool

// NewSet creates and initializes a Set and populates it with
// inital values as specified in the parameters.
func NewSet(initial ...Tag) Set {
	result := make(Set)
	for _, value := range initial {
		result.Add(value)
	}
	return result
}

// NewSetFromStrings creates and initializes a Set and populates it
// by using names.ParseTag on the initial values specified in the parameters.
func NewSetFromStrings(initial ...string) (Set, error) {
	result := make(Set)
	for _, value := range initial {
		tag, err := ParseTag(value)
		if err != nil {
			return result, errors.Trace(err)
		}
		result.Add(tag)
	}
	return result, nil
}

// Size returns the number of elements in the set.
func (t Set) Size() int {
	return len(t)
}

// IsEmpty is true for empty or uninitialized sets.
func (t Set) IsEmpty() bool {
	return len(t) == 0
}

// Add puts a value into the set.
func (t Set) Add(value Tag) {
	if t == nil {
		panic("uninitalised set")
	}
	t[value] = true
}

// Remove takes a value out of the set.  If value wasn't in the set to start
// with, this method silently succeeds.
func (t Set) Remove(value Tag) {
	delete(t, value)
}

// Contains returns true if the value is in the set, and false otherwise.
func (t Set) Contains(value Tag) bool {
	_, exists := t[value]
	return exists
}

// Values returns an unordered slice containing all the values in the set.
func (t Set) Values() []Tag {
	result := make([]Tag, len(t))
	i := 0
	for key := range t {
		result[i] = key
		i++
	}
	return result
}

// stringValues returns a list of strings that represent a Tag.
// Used internally by the SortedValues method.
func (t Set) stringValues() []string {
	result := make([]string, t.Size())
	i := 0
	for key := range t {
		result[i] = key.String()
		i++
	}
	return result
}

// SortedValues returns an ordered slice containing all the values in the set.
func (t Set) SortedValues() []Tag {
	values := t.stringValues()
	sort.Strings(values)

	result := make([]Tag, len(values))
	for i, value := range values {
		// We already know only good strings can live in the Tags set
		// so we can safely ignore the error here.
		tag, _ := ParseTag(value)
		result[i] = tag
	}
	return result
}

// Union returns a new Set representing a union of the elments in the
// method target and the parameter.
func (t Set) Union(other Set) Set {
	result := make(Set)
	// Use the internal map rather than going through the friendlier functions
	// to avoid extra allocation of slices.
	for value := range t {
		result[value] = true
	}
	for value := range other {
		result[value] = true
	}
	return result
}

// Intersection returns a new Set representing a intersection of the elments in the
// method target and the parameter.
func (t Set) Intersection(other Set) Set {
	result := make(Set)
	// Use the internal map rather than going through the friendlier functions
	// to avoid extra allocation of slices.
	for value := range t {
		if other.Contains(value) {
			result[value] = true
		}
	}
	return result
}

// Difference returns a new Tags representing all the values in the
// target that are not in the parameter.
func (t Set) Difference(other Set) Set {
	result := make(Set)
	// Use the internal map rather than going through the friendlier functions
	// to avoid extra allocation of slices.
	for value := range t {
		if !other.Contains(value) {
			result[value] = true
		}
	}
	return result
}
