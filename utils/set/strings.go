// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package set

import (
	"sort"
)

// Strings represents the classic "set" data structure, and contains
// strings.
type Strings struct {
	values map[string]bool
}

// NewStrings creates and initializes a Strings and populates it with
// initial values as specified in the parameters.
func NewStrings(initial ...string) Strings {
	result := Strings{values: make(map[string]bool)}
	for _, value := range initial {
		result.Add(value)
	}
	return result
}

// Size returns the number of elements in the set.
func (s Strings) Size() int {
	return len(s.values)
}

// IsEmpty is true for empty or uninitialized sets.
func (s Strings) IsEmpty() bool {
	return len(s.values) == 0
}

// Add puts a value into the set.
func (s *Strings) Add(value string) {
	if s.values == nil {
		s.values = make(map[string]bool)
	}
	s.values[value] = true
}

// Remove takes a value out of the set.  If value wasn't in the set to start
// with, this method silently succeeds.
func (s *Strings) Remove(value string) {
	delete(s.values, value)
}

// Contains returns true if the value is in the set, and false otherwise.
func (s Strings) Contains(value string) bool {
	_, exists := s.values[value]
	return exists
}

// Values returns an unordered slice containing all the values in the set.
func (s Strings) Values() []string {
	result := make([]string, len(s.values))
	i := 0
	for key := range s.values {
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
	result := NewStrings()
	// Use the internal map rather than going through the friendlier functions
	// to avoid extra allocation of slices.
	for value := range s.values {
		result.values[value] = true
	}
	for value := range other.values {
		result.values[value] = true
	}
	return result
}

// Intersection returns a new Strings representing a intersection of the elments in the
// method target and the parameter.
func (s Strings) Intersection(other Strings) Strings {
	result := NewStrings()
	// Use the internal map rather than going through the friendlier functions
	// to avoid extra allocation of slices.
	for value := range s.values {
		if other.Contains(value) {
			result.values[value] = true
		}
	}
	return result
}

// Difference returns a new Strings representing all the values in the
// target that are not in the parameter.
func (s Strings) Difference(other Strings) Strings {
	result := NewStrings()
	// Use the internal map rather than going through the friendlier functions
	// to avoid extra allocation of slices.
	for value := range s.values {
		if !other.Contains(value) {
			result.values[value] = true
		}
	}
	return result
}
