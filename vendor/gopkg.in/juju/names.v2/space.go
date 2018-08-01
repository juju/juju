// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"regexp"
)

const (
	SpaceTagKind = "space"
	SpaceSnippet = "(?:[a-z0-9]+(?:-[a-z0-9]+)*)"
)

var validSpace = regexp.MustCompile("^" + SpaceSnippet + "$")

// IsValidSpace reports whether name is a valid space name.
func IsValidSpace(name string) bool {
	return validSpace.MatchString(name)
}

type SpaceTag struct {
	name string
}

func (t SpaceTag) String() string { return t.Kind() + "-" + t.Id() }
func (t SpaceTag) Kind() string   { return SpaceTagKind }
func (t SpaceTag) Id() string     { return t.name }

// NewSpaceTag returns the tag of a space with the given name.
func NewSpaceTag(name string) SpaceTag {
	if !IsValidSpace(name) {
		panic(fmt.Sprintf("%q is not a valid space name", name))
	}
	return SpaceTag{name: name}
}

// ParseSpaceTag parses a space tag string.
func ParseSpaceTag(spaceTag string) (SpaceTag, error) {
	tag, err := ParseTag(spaceTag)
	if err != nil {
		return SpaceTag{}, err
	}
	nt, ok := tag.(SpaceTag)
	if !ok {
		return SpaceTag{}, invalidTagError(spaceTag, SpaceTagKind)
	}
	return nt, nil
}
