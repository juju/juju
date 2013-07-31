// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names

import (
	"fmt"
	"regexp"
	"strings"
)

var validUnit = regexp.MustCompile("^" + ServiceSnippet + "/" + NumberSnippet + "$")

// UnitTag returns the tag for the unit with the given name.
func UnitTag(unitName string) string {
	return makeTag(UnitTagKind, strings.Replace(unitName, "/", "-", -1))
}

// UnitFromTag returns the unit name that was used to create the tag,
// or an error if the tag is not of a unit.
func UnitFromTag(tag string) (string, error) {
	kind, name, err := splitTag(tag)
	if kind != UnitTagKind || err != nil {
		return "", fmt.Errorf("%q is not a valid unit tag", tag)
	}
	// Put the slashes back.
	name = strings.Replace(name, "-", "/", -1)
	if !IsUnit(name) {
		return "", fmt.Errorf("%q is not a valid unit tag", tag)
	}
	return name, nil
}

// IsUnit returns whether name is a valid unit name.
func IsUnit(name string) bool {
	return validUnit.MatchString(name)
}
