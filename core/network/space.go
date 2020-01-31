// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/collections/set"
)

const (
	// AlphaSpaceId is the ID of the alpha network space.
	// Application endpoints are bound to this space by default
	// if no explicit binding is specified.
	AlphaSpaceId = "0"

	// AlphaSpaceName is the name of the alpha network space.
	AlphaSpaceName = "alpha"
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
	return strings.Join(s.Names(), ", ")
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

var (
	invalidSpaceNameChars = regexp.MustCompile("[^0-9a-z-]")
	dashPrefix            = regexp.MustCompile("^-*")
	dashSuffix            = regexp.MustCompile("-*$")
	multipleDashes        = regexp.MustCompile("--+")
)

// ConvertSpaceName is used to massage provider-sourced (i.e. MAAS)
// space names so that they conform to Juju's space name rules.
func ConvertSpaceName(name string, existing set.Strings) string {
	// Lower case and replace spaces with dashes.
	name = strings.Replace(name, " ", "-", -1)
	name = strings.ToLower(name)

	// Remove any character not in the set "-", "a-z", "0-9".
	name = invalidSpaceNameChars.ReplaceAllString(name, "")

	// Remove any dashes at the beginning and end.
	name = dashPrefix.ReplaceAllString(name, "")
	name = dashSuffix.ReplaceAllString(name, "")

	// Replace multiple dashes with a single dash.
	name = multipleDashes.ReplaceAllString(name, "-")

	// If the name had only invalid characters, give it a new name.
	if name == "" {
		name = "empty"
	}

	// If this name is in use add a numerical suffix.
	if existing.Contains(name) {
		counter := 2
		for existing.Contains(name + fmt.Sprintf("-%d", counter)) {
			counter += 1
		}
		name = name + fmt.Sprintf("-%d", counter)
	}

	return name
}
