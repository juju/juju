// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"math/rand"
	"sort"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.core.network")

// macAddressTemplate is suitable for generating virtual MAC addresses,
// particularly for use by container devices.
// The last 3 segments are randomised.
// TODO (manadart 2018-06-21) Depending on where this is utilised,
// ensuring MAC address uniqueness within a model might be prudent.
const macAddressTemplate = "00:16:3e:%02x:%02x:%02x"

// GenerateVirtualMACAddress creates a random MAC address within the address
// space implied by macAddressTemplate above.
var GenerateVirtualMACAddress = func() string {
	digits := make([]interface{}, 3)
	for i := range digits {
		digits[i] = rand.Intn(256)
	}
	return fmt.Sprintf(macAddressTemplate, digits...)
}

// Id defines a provider-specific network ID.
type Id string

// String returns the underlying string representation of the Id.
// This method helps with formatting and type inference.
func (id Id) String() string {
	return string(id)
}

// IDSet represents the classic "set" data structure, and contains Id.
// IDSet is used as a typed version to prevent string -> Id -> string
// conversion when using set.Strings
type IDSet map[Id]struct{}

// MakeIDSet creates and initializes a IDSet and populates it with
// initial values as specified in the parameters.
func MakeIDSet(values ...Id) IDSet {
	set := make(map[Id]struct{}, len(values))
	for _, id := range values {
		set[id] = struct{}{}
	}
	return set
}

// Add puts a value into the set.
func (s IDSet) Add(value Id) {
	s[value] = struct{}{}
}

// Size returns the number of elements in the set.
func (s IDSet) Size() int {
	return len(s)
}

// IsEmpty is true for empty or uninitialized sets.
func (s IDSet) IsEmpty() bool {
	return len(s) == 0
}

// Contains returns true if the value is in the set, and false otherwise.
func (s IDSet) Contains(id Id) bool {
	_, exists := s[id]
	return exists
}

// Values returns an unordered slice containing all the values in the set.
func (s IDSet) Values() []Id {
	result := make([]Id, len(s))
	i := 0
	for key := range s {
		result[i] = key
		i++
	}
	return result
}

// SortedValues returns an ordered slice containing all the values in the set.
func (s IDSet) SortedValues() []Id {
	values := s.Values()
	sort.Slice(values, func(i, j int) bool {
		return values[i] < values[j]
	})
	return values
}
