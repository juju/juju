// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package set

import (
	"sort"
)

// Strings represents the classic "set" data structure, and contains strings.
type Strings map[string]bool

// NewStrings creates and initializes a Strings and populates it with
// initial values as specified in the parameters.
func NewStrings(initial ...string) Strings {
	result := make(Strings)
	for _, value := range initial {
		result.Add(value)
	}
	return result
}

// Size returns the number of elements in the set.
func (s Strings) Size() int {
	return len(s)
}

// IsEmpty is true for empty or uninitialized sets.
func (s Strings) IsEmpty() bool {
	return len(s) == 0
}

// Add puts a value into the set.
func (s Strings) Add(value string) {
	if s == nil {
		panic("uninitalised set")
	}
	s[value] = true
}

// Remove takes a value out of the set. If value wasn't in the set to start
// with, this method silently succeeds.
func (s Strings) Remove(value string) {
	delete(s, value)
}

// Contains returns true if the value is in the set, and false otherwise.
func (s Strings) Contains(value string) bool {
	_, exists := s[value]
	return exists
}

// Values returns an unordered slice containing all the values in the set.
func (s Strings) Values() []string {
	result := make([]string, len(s))
	i := 0
	for key := range s {
		result[i] = key
		i++
	}
	return result
}

// SortedValues returns an ordered slice containing all the values in the set.
func (s Strings) SortedValues() []string {
	values := s.Values()
	sort.Strings(values)
	return values
}

// Union returns a new Strings representing a union of the elments in the
// method target and the parameter.
func (s Strings) Union(other Strings) Strings {
	result := make(Strings)
	// Use the internal map rather than going through the friendlier functions
	// to avoid extra allocation of slices.
	for value := range s {
		result[value] = true
	}
	for value := range other {
		result[value] = true
	}
	return result
}

// Intersection returns a new Strings representing a intersection of the elments in the
// method target and the parameter.
func (s Strings) Intersection(other Strings) Strings {
	result := make(Strings)
	// Use the internal map rather than going through the friendlier functions
	// to avoid extra allocation of slices.
	for value := range s {
		if other.Contains(value) {
			result[value] = true
		}
	}
	return result
}

// Difference returns a new Strings representing all the values in the
// target that are not in the parameter.
func (s Strings) Difference(other Strings) Strings {
	result := make(Strings)
	// Use the internal map rather than going through the friendlier functions
	// to avoid extra allocation of slices.
	for value := range s {
		if !other.Contains(value) {
			result[value] = true
		}
	}
	return result
}
