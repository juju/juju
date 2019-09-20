// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import "strings"

const (
	// DefaultSpaceId is the ID of the default network space.
	// Application endpoints are bound to this space by default
	// if no explicit binding is specified.
	DefaultSpaceId = "0"

	// DefaultSpaceName is the name of the default network space.
	DefaultSpaceName = ""
)

// SpaceLookup describes methods for acquiring lookups that
// will translate space IDs to space names and vice versa.
type SpaceLookup interface {
	SpaceIDsByName() (map[string]string, error)
	SpaceInfosByID() (map[string]SpaceInfo, error)
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

// String returns the comma-delimited names of the spaces in the collection.
func (s SpaceInfos) String() string {
	namesString := make([]string, len(s))
	for i, v := range s {
		namesString[i] = string(v.Name)
	}
	return strings.Join(namesString, ", ")
}

// HasSpaceWithName returns true if there is a space in the collection,
// with the input name.
func (s SpaceInfos) HasSpaceWithName(name SpaceName) bool {
	for _, space := range s {
		if space.Name == name {
			return true
		}
	}
	return false
}

// Space returns a reference to the space with the input ID if it exists in the
// collection. Otherwise nil is returned.
func (s SpaceInfos) Space(id string) *SpaceInfo {
	for _, space := range s {
		if space.ID == id {
			return &space
		}
	}
	return nil
}
