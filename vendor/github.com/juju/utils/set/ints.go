// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package set

import (
	"sort"
)

// Ints represents the classic "set" data structure, and contains ints.
type Ints map[int]bool

// NewInts creates and initializes an Ints and populates it with
// initial values as specified in the parameters.
func NewInts(initial ...int) Ints {
	result := make(Ints)
	for _, value := range initial {
		result.Add(value)
	}
	return result
}

// Size returns the number of elements in the set.
func (is Ints) Size() int {
	return len(is)
}

// IsEmpty is true for empty or uninitialized sets.
func (is Ints) IsEmpty() bool {
	return len(is) == 0
}

// Add puts a value into the set.
func (is Ints) Add(value int) {
	if is == nil {
		panic("uninitalised set")
	}
	is[value] = true
}

// Remove takes a value out of the set. If value wasn't in the set to start
// with, this method silently succeeds.
func (is Ints) Remove(value int) {
	delete(is, value)
}

// Contains returns true if the value is in the set, and false otherwise.
func (is Ints) Contains(value int) bool {
	_, exists := is[value]
	return exists
}

// Values returns an unordered slice containing all the values in the set.
func (is Ints) Values() []int {
	result := make([]int, len(is))
	i := 0
	for key := range is {
		result[i] = key
		i++
	}
	return result
}

// SortedValues returns an ordered slice containing all the values in the set.
func (is Ints) SortedValues() []int {
	values := is.Values()
	sort.Ints(values)
	return values
}

// Union returns a new Ints representing a union of the elments in the
// method target and the parameter.
func (is Ints) Union(other Ints) Ints {
	result := make(Ints)
	// Use the internal map rather than going through the friendlier functions
	// to avoid extra allocation of slices.
	for value := range is {
		result[value] = true
	}
	for value := range other {
		result[value] = true
	}
	return result
}

// Intersection returns a new Ints representing a intersection of the elments in the
// method target and the parameter.
func (is Ints) Intersection(other Ints) Ints {
	result := make(Ints)
	// Use the internal map rather than going through the friendlier functions
	// to avoid extra allocation of slices.
	for value := range is {
		if other.Contains(value) {
			result[value] = true
		}
	}
	return result
}

// Difference returns a new Ints representing all the values in the
// target that are not in the parameter.
func (is Ints) Difference(other Ints) Ints {
	result := make(Ints)
	// Use the internal map rather than going through the friendlier functions
	// to avoid extra allocation of slices.
	for value := range is {
		if !other.Contains(value) {
			result[value] = true
		}
	}
	return result
}
