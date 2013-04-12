package set

import (
	"sort"
)

// StringSet represents the classic "set" data structure, and contains
// strings.
type StringSet struct {
	values map[string]bool
}

// MakeStringSet creates and initializes a StringSet and populates it with
// initial values as specified in the parameters.
func MakeStringSet(initial ...string) StringSet {
	result := StringSet{values: make(map[string]bool)}
	for _, value := range initial {
		result.Add(value)
	}
	return result
}

// Size returns the number of elements in the set.
func (s *StringSet) Size() int {
	return len(s.values)
}

// Add puts a value into the set.
func (s *StringSet) Add(value string) {
	if s.values == nil {
		s.values = make(map[string]bool)
	}
	s.values[value] = true
}

// Remove takes a value out of the set.  If value wasn't in the set to start
// with, this method silently succeeds.
func (s *StringSet) Remove(value string) {
	if s.values == nil {
		return
	}
	delete(s.values, value)
}

// Contains returns true if the value is in the set, and false otherwise.
func (s *StringSet) Contains(value string) bool {
	if s.values == nil {
		return false
	}
	_, exists := s.values[value]
	return exists
}

// Values returns an unordered slice containing all the values in the set.
func (s *StringSet) Values() []string {
	result := make([]string, len(s.values))
	i := 0
	for key, _ := range s.values {
		result[i] = key
		i++
	}
	return result
}

// SortedValues returns an ordered slice containing all the values in the set.
func (s *StringSet) SortedValues() []string {
	values := s.Values()
	sort.Strings(values)
	return values
}

// Union returns a new StringSet representing a union of the elments in the
// method target and the parameter.
func (s *StringSet) Union(other StringSet) StringSet {
	result := MakeStringSet()
	// Use the internal map rather than going through the friendlier functions
	// to avoid extra allocation of slices.
	for value, _ := range s.values {
		result.values[value] = true
	}
	for value, _ := range other.values {
		result.values[value] = true
	}
	return result
}

// Intersection returns a new StringSet representing a intersection of the elments in the
// method target and the parameter.
func (s *StringSet) Intersection(other StringSet) StringSet {
	result := MakeStringSet()
	// Use the internal map rather than going through the friendlier functions
	// to avoid extra allocation of slices.
	for value, _ := range s.values {
		if other.Contains(value) {
			result.values[value] = true
		}
	}
	return result
}

// Difference returns a new StringSet representing all the values in the
// target that are not in the parameter.
func (s *StringSet) Difference(other StringSet) StringSet {
	result := MakeStringSet()
	// Use the internal map rather than going through the friendlier functions
	// to avoid extra allocation of slices.
	for value, _ := range s.values {
		if !other.Contains(value) {
			result.values[value] = true
		}
	}
	return result
}
