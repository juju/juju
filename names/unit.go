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
	return UnitTagPrefix + strings.Replace(unitName, "/", "-", -1)
}

// UnitNameFromTag returns the unit name that was used to create the tag.
func UnitNameFromTag(tag string) (string, error) {
	if !strings.HasPrefix(tag, UnitTagPrefix) {
		return "", fmt.Errorf("invalid unit tag format: %v", tag)
	}
	// Strip off the "unit-" prefix.
	name := tag[len(UnitTagPrefix):]
	// Put the slashes back.
	name = strings.Replace(name, "-", "/", -1)
	return name, nil
}

// IsUnitName returns whether name is a valid unit name.
func IsUnitName(name string) bool {
	return validUnit.MatchString(name)
}
