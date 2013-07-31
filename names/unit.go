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
// It will panic if the given unit name is not valid.
func UnitTag(unitName string) string {
	// Replace only the last "/" with "-".
	i := strings.LastIndex(unitName, "/")
	if i <= 0 || !IsUnit(unitName) {
		panic(fmt.Sprintf("%q is not a valid unit name", unitName))
	}
	unitName = unitName[:i] + "-" + unitName[i+1:]
	return makeTag(UnitTagKind, unitName)
}

// UnitFromTag returns the unit name that was used to create the tag,
// or an error if the tag is not of a unit.
func UnitFromTag(tag string) (string, error) {
	kind, name, err := splitTag(tag)
	if kind != UnitTagKind || err != nil {
		return "", fmt.Errorf("%q is not a valid unit tag", tag)
	}
	// Replace only the last "-" with "/".
	if i := strings.LastIndex(name, "-"); i > 0 {
		name = name[:i] + "/" + name[i+1:]
	}
	if !IsUnit(name) {
		return "", fmt.Errorf("%q is not a valid unit tag", tag)
	}
	return name, nil
}

// IsUnit returns whether name is a valid unit name.
func IsUnit(name string) bool {
	return validUnit.MatchString(name)
}
