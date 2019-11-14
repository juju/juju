// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"strings"
)

const (
	// AlphaSpaceId is the ID of the alpha network space.
	// Application endpoints are bound to this space by default
	// if no explicit binding is specified.
	AlphaSpaceId = "0"

	// AlphaSpaceName is the name of the alpha network space.
	AlphaSpaceName = "alpha"
)

// SpaceLookup describes methods for acquiring SpaceInfos
// to translate space IDs to space names and vice versa.
type SpaceLookup interface {
	AllSpaceInfos() (SpaceInfos, error)
}

// SpaceName is the name of a network space.
type SpaceName string

// SpaceInfo defines a network space.
type SpaceInfo struct {
	// ID is the unique identifier for the space.
	ID string

	// Name is the name of the space.
	// It is used by operators for identifying a space and should be unique.
	Name SpaceName

	// ProviderId is the provider's unique identifier for the space,
	// such as used by MAAS.
	ProviderId Id

	// Subnets are the subnets that have been grouped into this network space.
	Subnets []SubnetInfo
}

// SpaceInfos is a collection of spaces.
type SpaceInfos []SpaceInfo

// String returns returns a quoted, comma-delimited names of the spaces in the
// collection, or <none> if the collection is empty.
func (s SpaceInfos) String() string {
	if len(s) == 0 {
		return "<none>"
	}
	names := make([]string, len(s))
	for i, v := range s {
		names[i] = fmt.Sprintf("%q", string(v.Name))
	}
	return strings.Join(names, ", ")
}

// Names returns a string slice with each of the space names in the collection.
func (s SpaceInfos) Names() []string {
	names := make([]string, len(s))
	for i, v := range s {
		names[i] = string(v.Name)
	}
	return names
}

// IDs returns a string slice with each of the space ids in the collection.
func (s SpaceInfos) IDs() []string {
	ids := make([]string, len(s))
	for i, v := range s {
		ids[i] = v.ID
	}
	return ids
}

// GetByID returns a reference to the space with the input ID
// if it exists in the collection. Otherwise nil is returned.
func (s SpaceInfos) GetByID(id string) *SpaceInfo {
	for _, space := range s {
		if space.ID == id {
			return &space
		}
	}
	return nil
}

// GetByName returns a reference to the space with the input name
// if it exists in the collection. Otherwise nil is returned.
func (s SpaceInfos) GetByName(name string) *SpaceInfo {
	for _, space := range s {
		if string(space.Name) == name {
			return &space
		}
	}
	return nil
}

// ContainsID returns true if the collection contains a
// space with the given ID.
func (s SpaceInfos) ContainsID(id string) bool {
	return s.GetByID(id) != nil
}

// ContainsName returns true if the collection contains a
// space with the given name.
func (s SpaceInfos) ContainsName(name string) bool {
	return s.GetByName(name) != nil
}

// Minus returns a new SpaceInfos representing all the
// values in the target that are not in the parameter. Value
// matching is done by ID.
func (s SpaceInfos) Minus(other SpaceInfos) SpaceInfos {
	result := make(SpaceInfos, 0)
	for _, value := range s {
		if !other.ContainsID(value.ID) {
			result = append(result, value)
		}
	}
	return result
}
