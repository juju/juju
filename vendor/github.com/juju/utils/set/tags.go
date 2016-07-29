// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package set

import (
	"sort"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
)

// Tags represents the Set data structure, it implements tagSet
// and contains names.Tags.
type Tags map[names.Tag]bool

// NewTags creates and initializes a Tags and populates it with
// inital values as specified in the parameters.
func NewTags(initial ...names.Tag) Tags {
	result := make(Tags)
	for _, value := range initial {
		result.Add(value)
	}
	return result
}

// NewTagsFromStrings creates and initializes a Tags and populates it
// by using names.ParseTag on the initial values specified in the parameters.
func NewTagsFromStrings(initial ...string) (Tags, error) {
	result := make(Tags)
	for _, value := range initial {
		tag, err := names.ParseTag(value)
		if err != nil {
			return result, errors.Trace(err)
		}
		result.Add(tag)
	}
	return result, nil
}

// Size returns the number of elements in the set.
func (t Tags) Size() int {
	return len(t)
}

// IsEmpty is true for empty or uninitialized sets.
func (t Tags) IsEmpty() bool {
	return len(t) == 0
}

// Add puts a value into the set.
func (t Tags) Add(value names.Tag) {
	if t == nil {
		panic("uninitalised set")
	}
	t[value] = true
}

// Remove takes a value out of the set.  If value wasn't in the set to start
// with, this method silently succeeds.
func (t Tags) Remove(value names.Tag) {
	delete(t, value)
}

// Contains returns true if the value is in the set, and false otherwise.
func (t Tags) Contains(value names.Tag) bool {
	_, exists := t[value]
	return exists
}

// Values returns an unordered slice containing all the values in the set.
func (t Tags) Values() []names.Tag {
	result := make([]names.Tag, len(t))
	i := 0
	for key := range t {
		result[i] = key
		i++
	}
	return result
}

// stringValues returns a list of strings that represent a names.Tag
// Used internally by the SortedValues method.
func (t Tags) stringValues() []string {
	result := make([]string, t.Size())
	i := 0
	for key := range t {
		result[i] = key.String()
		i++
	}
	return result
}

// SortedValues returns an ordered slice containing all the values in the set.
func (t Tags) SortedValues() []names.Tag {
	values := t.stringValues()
	sort.Strings(values)

	result := make([]names.Tag, len(values))
	for i, value := range values {
		// We already know only good strings can live in the Tags set
		// so we can safely ignore the error here.
		tag, _ := names.ParseTag(value)
		result[i] = tag
	}
	return result
}

// Union returns a new Tags representing a union of the elments in the
// method target and the parameter.
func (t Tags) Union(other Tags) Tags {
	result := make(Tags)
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

// Intersection returns a new Tags representing a intersection of the elments in the
// method target and the parameter.
func (t Tags) Intersection(other Tags) Tags {
	result := make(Tags)
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
func (t Tags) Difference(other Tags) Tags {
	result := make(Tags)
	// Use the internal map rather than going through the friendlier functions
	// to avoid extra allocation of slices.
	for value := range t {
		if !other.Contains(value) {
			result[value] = true
		}
	}
	return result
}
